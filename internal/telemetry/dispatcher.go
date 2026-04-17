package telemetry

import "context"

// Callback 定义内部 telemetry dispatcher 使用的生命周期回调。
type Callback interface {
	OnRetrieveStart(ctx context.Context, query string)
	OnRetrieveEnd(ctx context.Context, resultCount int, err error)
	OnToolStart(ctx context.Context, tool string)
	OnToolEnd(ctx context.Context, tool string, err error)
	OnModelStart(ctx context.Context, model string)
	OnModelEnd(ctx context.Context, model string, err error)
}

// Dispatcher 把 telemetry 事件扇出到所有已注册回调。
type Dispatcher struct {
	callbacks []Callback
}

// NewDispatcher 创建一个带防御性拷贝的 dispatcher。
func NewDispatcher(callbacks []Callback) Dispatcher {
	filtered := make([]Callback, 0, len(callbacks))
	for _, cb := range callbacks {
		if cb == nil {
			continue
		}
		filtered = append(filtered, cb)
	}
	return Dispatcher{
		callbacks: filtered,
	}
}

func (d Dispatcher) OnRetrieveStart(ctx context.Context, query string) {
	for _, cb := range d.callbacks {
		cb.OnRetrieveStart(ctx, query)
	}
}

func (d Dispatcher) OnRetrieveEnd(ctx context.Context, resultCount int, err error) {
	for _, cb := range d.callbacks {
		cb.OnRetrieveEnd(ctx, resultCount, err)
	}
}

func (d Dispatcher) OnToolStart(ctx context.Context, tool string) {
	for _, cb := range d.callbacks {
		cb.OnToolStart(ctx, tool)
	}
}

func (d Dispatcher) OnToolEnd(ctx context.Context, tool string, err error) {
	for _, cb := range d.callbacks {
		cb.OnToolEnd(ctx, tool, err)
	}
}

func (d Dispatcher) OnModelStart(ctx context.Context, model string) {
	for _, cb := range d.callbacks {
		cb.OnModelStart(ctx, model)
	}
}

func (d Dispatcher) OnModelEnd(ctx context.Context, model string, err error) {
	for _, cb := range d.callbacks {
		cb.OnModelEnd(ctx, model, err)
	}
}
