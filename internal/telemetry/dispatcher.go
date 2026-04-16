package telemetry

import "context"

// Callback defines lifecycle hooks used by the internal telemetry dispatcher.
type Callback interface {
	OnRetrieveStart(ctx context.Context, query string)
	OnRetrieveEnd(ctx context.Context, resultCount int, err error)
	OnToolStart(ctx context.Context, tool string)
	OnToolEnd(ctx context.Context, tool string, err error)
	OnModelStart(ctx context.Context, model string)
	OnModelEnd(ctx context.Context, model string, err error)
}

// Dispatcher fans telemetry events out to all registered callbacks.
type Dispatcher struct {
	callbacks []Callback
}

// NewDispatcher creates a dispatcher with a defensive callback copy.
func NewDispatcher(callbacks []Callback) Dispatcher {
	return Dispatcher{
		callbacks: append([]Callback(nil), callbacks...),
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
