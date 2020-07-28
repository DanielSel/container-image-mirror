package contextos

import (
	"context"

	"github.com/go-logr/logr"
)

type Key string

const LogrKey Key = "logr"

func Logger(ctx context.Context) logr.Logger {
	return ctx.Value(LogrKey).(logr.Logger)
}

func WithLogger(ctx context.Context, log logr.Logger) context.Context {
	return context.WithValue(ctx, LogrKey, log)
}

func WithLogName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, LogrKey, Logger(ctx).WithName(name))
}

func WithLogValues(ctx context.Context, keysAndValues ...interface{}) context.Context {
	return context.WithValue(ctx, LogrKey, Logger(ctx).WithValues(keysAndValues...))
}
