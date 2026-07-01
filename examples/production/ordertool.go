package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	ragagent "github.com/gtkit/go-rag-agent"
)

// 订单工具的错误分类：参数错误与未命中错误必须可区分，便于上层决定是追问用户还是直接拒答。
var (
	// ErrOrderArgInvalid 表示调用参数非法（缺失或为空的 order_id）。
	ErrOrderArgInvalid = errors.New("order tool: invalid argument")
	// ErrOrderNotFound 表示订单号不存在。
	ErrOrderNotFound = errors.New("order tool: order not found")
)

// orderRecord 是只读订单数据。真实接入时换成对订单服务的只读查询，
// 但务必保持"只读"边界：本工具不暴露任何下单、改单、退款等写操作。
type orderRecord struct {
	ID       string
	Status   string
	Address  string
	Items    []string
	RefundAt string
}

// orderTool 是一个只读订单查询工具示例。
//
// 何时不该调用本工具（交给模型的使用边界，写在 Description 与注释里）：
//   - 问题与具体订单无关（如配送政策、退货规则），应由知识库 RAG 回答，而不是查订单。
//   - 用户没有提供订单号，应先向用户追问订单号，而不是凭空调用。
//   - 涉及下单、改地址、发起退款等写操作，本工具一律不支持，必须走人工或带审批的写通道。
type orderTool struct {
	timeout time.Duration
	orders  map[string]orderRecord
}

// newOrderTool 构造一个带内存数据源的只读订单工具。timeout <= 0 时使用 2s 默认值。
func newOrderTool(timeout time.Duration) *orderTool {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &orderTool{
		timeout: timeout,
		orders: map[string]orderRecord{
			"NO-1001": {ID: "NO-1001", Status: "运输中，预计明天送达", Address: "上海市浦东新区xx路1号", Items: []string{"无线耳机", "充电器"}, RefundAt: ""},
			"NO-1002": {ID: "NO-1002", Status: "已签收", Address: "北京市海淀区yy路2号", Items: []string{"机械键盘"}, RefundAt: ""},
			"NO-1003": {ID: "NO-1003", Status: "退款处理中", Address: "广州市天河区zz路3号", Items: []string{"蓝牙音箱"}, RefundAt: "预计 2 个工作日内到账"},
		},
	}
}

func (t *orderTool) Name() string { return "query_order" }

func (t *orderTool) Description() string {
	return "按订单号查询订单的只读信息（状态、地址、商品、退款进度）。" +
		"仅在用户明确提供订单号且问题是关于该订单时调用；" +
		"配送/退货/保修等通用政策问题不要调用本工具；本工具不支持任何写操作。"
}

func (t *orderTool) Schema() ragagent.ToolSchema {
	return ragagent.ToolSchema{
		Description: "只读订单查询参数",
		Properties: map[string]ragagent.ToolParameterSchema{
			"order_id": {
				Type:        ragagent.ToolParameterString,
				Description: "订单号，形如 NO-1001",
				MinLength:   1,
				MaxLength:   64,
			},
		},
		Required: []string{"order_id"},
	}
}

// RunStructured 执行只读查询，并对所有外部边界设超时。
func (t *orderTool) RunStructured(ctx context.Context, args map[string]any) (ragagent.ToolResult, error) {
	ctx, cancel := context.WithTimeout(ctx, t.timeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return ragagent.ToolResult{}, err
	}

	raw, ok := args["order_id"]
	if !ok {
		return ragagent.ToolResult{}, fmt.Errorf("%w: missing order_id", ErrOrderArgInvalid)
	}
	id, ok := raw.(string)
	if !ok {
		return ragagent.ToolResult{}, fmt.Errorf("%w: order_id must be string", ErrOrderArgInvalid)
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return ragagent.ToolResult{}, fmt.Errorf("%w: empty order_id", ErrOrderArgInvalid)
	}

	order, ok := t.orders[id]
	if !ok {
		return ragagent.ToolResult{Retryable: false}, fmt.Errorf("%w: %s", ErrOrderNotFound, id)
	}

	text := fmt.Sprintf("订单 %s 状态：%s；收货地址：%s；商品：%s", order.ID, order.Status, order.Address, strings.Join(order.Items, "、"))
	if order.RefundAt != "" {
		text += "；退款：" + order.RefundAt
	}
	return ragagent.ToolResult{
		Text:     text,
		JSON:     order,
		Metadata: map[string]string{"order_id": order.ID, "access": "read_only"},
	}, nil
}

// registerOrderTool 把只读订单工具注册到 ToolRegistry（经结构化适配）。
func registerOrderTool(registry *ragagent.ToolRegistry, tool *orderTool) error {
	return registry.Register(ragagent.NewStructuredToolAdapter(tool))
}
