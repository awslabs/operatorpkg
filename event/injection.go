package event

import "context"

type contextKey struct{}

func FromContext(ctx context.Context) Recorder {
	if v := ctx.Value(contextKey{}); v != nil {
		return v.(Recorder)
	}
	return &noopRecorder{}
}

func IntoContext(ctx context.Context, recorder Recorder) context.Context {
	return context.WithValue(ctx, contextKey{}, recorder)
}
