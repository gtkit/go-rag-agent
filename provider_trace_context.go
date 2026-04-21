package ragagent

import "context"

type providerTraceObserverKey struct{}

func withProviderTraceObserver(ctx context.Context, fn func(ProviderCallTrace)) context.Context {
	if fn == nil {
		return ctx
	}
	return context.WithValue(ctx, providerTraceObserverKey{}, fn)
}

func emitProviderTrace(ctx context.Context, call ProviderCallTrace) {
	if fn, ok := ctx.Value(providerTraceObserverKey{}).(func(ProviderCallTrace)); ok && fn != nil {
		fn(call)
	}
}
