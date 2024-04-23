package logger

import "context"

type contextKey string

const (
	namespaceKey contextKey = "namespace"
)

func WithNamespaceForContext(ctx context.Context, namespace string) context.Context {
	return context.WithValue(ctx, namespaceKey, namespace)
}

func NamespaceFromContext(ctx context.Context) string {
	if val, ok := ctx.Value(namespaceKey).(string); ok {
		return val
	}
	return ""
}
