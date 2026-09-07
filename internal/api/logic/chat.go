package logic

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"ai-start/internal/agent"
	"ai-start/internal/runtime"
	"ai-start/internal/store"
	"ai-start/internal/tool"
)

// ChatReply 对话回复。
type ChatReply struct {
	SessionID string `json:"session_id"` // 会话 ID
	Answer    string `json:"answer"`     // 回复内容
	Agent     string `json:"agent"`      // 实际处理的 Agent（路由结果）
	TraceID   string `json:"trace_id"`   // 链路追踪 ID
}

// ChatService 多轮对话编排（feat002/feat003）。
//
// 记忆权责划分（feat003 修正）：
//   - 编排层（本服务）：短期记忆（Redis 会话缓冲）+ 用户画像（MySQL 档案）
//   - 各 Agent：长期经验记忆（pgvector，agent 作用域隔离）自召回、自沉淀
type ChatService struct {
	runtime *runtime.Runtime // 多 Agent 运行时
	buffer  store.ChatBuffer // 短期会话缓冲
	parse   *ParseService    // 档案服务（取画像注入上下文）
	logger  *slog.Logger     // 日志器（注入）
}

// NewChatService 创建对话服务。
func NewChatService(rt *runtime.Runtime, buffer store.ChatBuffer, parse *ParseService, logger *slog.Logger) *ChatService {
	return &ChatService{runtime: rt, buffer: buffer, parse: parse, logger: logger}
}

// 节点名常量。
const (
	nodeRouter  = "router"  // 意图路由节点
	nodeAdvisor = "advisor" // 选岗参谋节点（兜底目标）
)

// clarifyReply 意图不明时的反问文案。
const clarifyReply = "我没有完全理解你的意思。你是想：1）咨询选岗/岗位推荐 2）练习笔试题目 3）模拟面试？跟我说一句就行。"

// chatCallTimeout chat 场景 Agent 调用超时。
// advise 流程嵌入 run_advise 工具执行（researcher→analyzer→strategist→responder 多次 LLM 调用），
// 需要比默认 30s 更长的预算。
const chatCallTimeout = 5 * time.Minute

// Chat 对话主流程。sink 非空时启用流式（状态/工具/逐 token 事件经 SSE 下发）。
func (s *ChatService) Chat(ctx context.Context, userID uint64, sessionID, question, mode string, sink runtime.EventSink) (*ChatReply, error) {
	if sessionID == "" {
		sessionID = newSessionID() // 新会话
	}
	// 身份注入：工具执行的用户身份来自 JWT，不接受模型传入
	ctx = tool.WithUserID(ctx, userID)
	// 事件通道注入：ReAct/节点的流式事件经此透传到 SSE
	if sink != nil {
		ctx = runtime.WithEventSink(ctx, sink)
	}
	traceID := runtime.TraceIDFromContext(ctx)

	// 1. 短期记忆：读取本会话最近消息（长期经验由各 Agent 自召回，编排层不管）
	runtime.EmitEvent(ctx, runtime.EventStatus, "正在读取会话记忆…")
	// 默认三题模拟含追问会超过十条消息，使用现有缓冲的完整二十条窗口。
	history, _ := s.buffer.Recent(ctx, sessionID, 20)

	// 2. 意图路由（LLM 判断目标 Agent）
	runtime.EmitEvent(ctx, runtime.EventStatus, "正在识别你的意图…")
	// Router 和目标 Agent 共享最近对话，短作答也能关联上一轮面试题。
	userContent := s.composeInput(history, question)
	route := s.route(ctx, userContent)
	if route.Clarify {
		s.rememberAll(ctx, sessionID, question, clarifyReply)
		return &ChatReply{SessionID: sessionID, Answer: clarifyReply, Agent: nodeRouter, TraceID: traceID}, nil
	}

	// 3. 组装上下文并调用目标 Agent（ReAct + 工具白名单）
	runtime.EmitEvent(ctx, runtime.EventStatus, fmt.Sprintf("已由「%s」接管，正在思考…", route.Target))
	vars := map[string]any{
		"Mode":    mode,
		"Profile": s.profileSummary(userID),
		// Memories 变量由目标 Agent 自召回填充（agent 作用域隔离）
	}
	reply, err := s.runtime.CallWithSink(runtime.WithPromptVars(ctx, vars), "chat", route.Target, runtime.MsgTypeTask, userContent, chatCallTimeout, sink)
	var answer string
	if err != nil {
		s.logger.Warn("Agent 调用失败，返回兜底回复", "agent", route.Target, "err", err, "trace_id", traceID)
		answer = fallbackReply(route.Target)
	} else {
		answer = reply.Content
	}

	// 4. 写入短期记忆（原始对话只进 Redis 缓冲；长期沉淀由各 Agent 自行决定）
	s.rememberAll(ctx, sessionID, question, answer)

	return &ChatReply{SessionID: sessionID, Answer: answer, Agent: route.Target, TraceID: traceID}, nil
}

// route 意图路由：调用 router 节点，失败/未知目标时兜底 advisor。
func (s *ChatService) route(ctx context.Context, question string) agent.RouteResult {
	reply, err := s.runtime.Call(ctx, "chat", nodeRouter, runtime.MsgTypeTask, question, runtime.DefaultCallTimeout)
	if err != nil {
		s.logger.Warn("意图路由失败，兜底 advisor", "err", err)
		return agent.RouteResult{Target: nodeAdvisor, Confidence: 1.0, Reason: "路由节点不可用"}
	}
	var result agent.RouteResult
	if err := json.Unmarshal([]byte(reply.Content), &result); err != nil || result.Target == "" {
		return agent.RouteResult{Target: nodeAdvisor, Confidence: 1.0, Reason: "路由结果解析失败"}
	}
	// 路由目标合法性：不存在的节点兜底 advisor（防止模型编造节点名）
	if !result.Clarify && !s.runtime.HasNode(result.Target) {
		s.logger.Warn("路由目标不存在，兜底 advisor", "target", result.Target)
		return agent.RouteResult{Target: nodeAdvisor, Confidence: result.Confidence, Reason: "目标节点未注册"}
	}
	return result
}

// composeInput 组装用户输入：短期历史 + 当前问题。
func (s *ChatService) composeInput(history []store.ChatMessage, question string) string {
	var sb strings.Builder
	if len(history) > 0 {
		sb.WriteString("【最近对话】\n")
		for _, m := range history {
			role := "用户"
			if m.Role == "assistant" {
				role = "助手"
			}
			fmt.Fprintf(&sb, "%s：%s\n", role, m.Content)
		}
	}
	sb.WriteString("【当前问题】\n" + question)
	return sb.String()
}

// profileSummary 用户档案摘要（注入 Prompt 变量）。
func (s *ChatService) profileSummary(userID uint64) string {
	profile, _, err := s.parse.GetLatestProfile(userID)
	if err != nil || profile == nil {
		return "（用户尚未建立档案）"
	}
	data, _ := json.Marshal(profile)
	return string(data)
}

// rememberAll 写入短期记忆（Redis 会话缓冲）。
// 注意：原始对话不写长期向量库（避免噪音淹没+重复存储），
// 长期记忆由各 Agent 通过 save_insight 工具自主沉淀（agent 作用域）。
func (s *ChatService) rememberAll(ctx context.Context, sessionID, question, answer string) {
	_ = s.buffer.Append(ctx, sessionID, "user", question)
	_ = s.buffer.Append(ctx, sessionID, "assistant", answer)
}

// fallbackReply Agent 调用失败时的兜底回复。
func fallbackReply(agentName string) string {
	return fmt.Sprintf("抱歉，%s 暂时开小差了。请稍后再试，或换个问法。", agentName)
}
