package runtime

import (
	"context"
	"fmt"
	"time"
)

// FlowExecutor 流程编排器：执行流程在配置的 flows 表中声明（流程名 → 节点名列表），
// 按序把上一个节点的输出作为下一个节点的输入（流水线模式），
// 调整执行顺序/增删节点只需改配置，代码不变。
type FlowExecutor struct {
	runtime *Runtime            // 多 Agent 运行时
	flows   map[string][]string // 流程名 → 节点名列表
	timeout time.Duration       // 单节点调用超时
}

// NewFlowExecutor 创建流程编排器。
func NewFlowExecutor(runtime *Runtime, flows map[string][]string, timeout time.Duration) *FlowExecutor {
	return &FlowExecutor{runtime: runtime, flows: flows, timeout: timeout}
}

// Has 流程是否存在。
func (f *FlowExecutor) Has(name string) bool {
	_, ok := f.flows[name]
	return ok
}

// Run 按配置顺序执行流程，返回最后一个节点的输出。
// 任一节点失败即中断并返回错误（含节点名与 TraceID 便于定位）。
func (f *FlowExecutor) Run(ctx context.Context, flowName, input string) (string, error) {
	steps, ok := f.flows[flowName]
	if !ok || len(steps) == 0 {
		return "", fmt.Errorf("流程 %q 未配置", flowName)
	}
	current := input
	sink := SinkFromContext(ctx) // 事件透传：流程节点的进度事件回传给调用方（如 SSE）
	for _, node := range steps {
		reply, err := f.runtime.CallWithSink(ctx, "flow:"+flowName, node, MsgTypeTask, current, f.timeout, sink)
		if err != nil {
			return "", fmt.Errorf("流程 %q 节点 %q 执行失败: %w", flowName, node, err)
		}
		current = reply.Content
	}
	return current, nil
}
