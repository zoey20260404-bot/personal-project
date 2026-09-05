package agent

import (
	"ai-start/internal/runtime"
	"context"
	"encoding/json"

	"ai-start/internal/types"
	"fmt"
)

// 流程名常量（在配置 flows 表中声明步骤）。
const (
	FlowParse = "parse" // 条件解析流程
)

// Supervisor 调度中枢：不直接持有 Agent 实例，
// 通过流程编排器按配置的执行流程驱动各节点（层级调度是总线上的使用模式）。
type Supervisor struct {
	flows *runtime.FlowExecutor
}

// NewSupervisor 创建 Supervisor。
func NewSupervisor(flows *runtime.FlowExecutor) *Supervisor {
	return &Supervisor{flows: flows}
}

// ParseInput 条件解析入口（PRD 4.1 时序图首段）：
// 走配置中的 parse 流程，支持多源融合（文本 + 图片）。
func (s *Supervisor) ParseInput(ctx context.Context, content string, images []types.ImageInput) (*ParseResult, error) {
	task, err := json.Marshal(parseTask{Content: content, Images: images})
	if err != nil {
		return nil, fmt.Errorf("序列化解析任务失败: %w", err)
	}
	output, err := s.flows.Run(ctx, FlowParse, string(task))
	if err != nil {
		return nil, err
	}
	var result ParseResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return nil, fmt.Errorf("反序列化解析结果失败: %w", err)
	}
	return &result, nil
}
