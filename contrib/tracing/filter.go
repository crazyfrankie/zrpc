package tracing

import (
	"context"
	"strings"

	"github.com/crazyfrankie/zrpc/stats"
)

// Filter is a predicate used to determine whether a given request in
// zRPC should be traced. A Filter must be concurrent safe.
type Filter func(ctx context.Context, info *stats.RPCTagInfo) bool

// AcceptAll returns a Filter that accepts all requests.
func AcceptAll() Filter {
	return func(context.Context, *stats.RPCTagInfo) bool {
		return true
	}
}

// RejectAll returns a Filter that rejects all requests.
func RejectAll() Filter {
	return func(context.Context, *stats.RPCTagInfo) bool {
		return false
	}
}

// MethodFilter returns a Filter that accepts only requests with the given method names.
func MethodFilter(methods ...string) Filter {
	methodSet := make(map[string]struct{}, len(methods))
	for _, method := range methods {
		methodSet[method] = struct{}{}
	}
	return func(_ context.Context, info *stats.RPCTagInfo) bool {
		_, ok := methodSet[info.FullMethodName]
		return ok
	}
}

// MethodPrefixFilter returns a Filter that accepts requests with method names
// that have any of the given prefixes.
func MethodPrefixFilter(prefixes ...string) Filter {
	return func(_ context.Context, info *stats.RPCTagInfo) bool {
		for _, prefix := range prefixes {
			if strings.HasPrefix(info.FullMethodName, prefix) {
				return true
			}
		}
		return false
	}
}

// HealthCheckFilter returns a Filter that rejects health check requests.
func HealthCheckFilter() Filter {
	return func(_ context.Context, info *stats.RPCTagInfo) bool {
		return !strings.HasPrefix(info.FullMethodName, "/grpc.health.v1.Health/")
	}
}

// Any returns a Filter that accepts requests that are accepted by any of the given filters.
func Any(filters ...Filter) Filter {
	return func(ctx context.Context, info *stats.RPCTagInfo) bool {
		for _, filter := range filters {
			if filter(ctx, info) {
				return true
			}
		}
		return false
	}
}

// All returns a Filter that accepts requests that are accepted by all of the given filters.
func All(filters ...Filter) Filter {
	return func(ctx context.Context, info *stats.RPCTagInfo) bool {
		for _, filter := range filters {
			if !filter(ctx, info) {
				return false
			}
		}
		return true
	}
}

// Not returns a Filter that accepts requests that are rejected by the given filter.
func Not(filter Filter) Filter {
	return func(ctx context.Context, info *stats.RPCTagInfo) bool {
		return !filter(ctx, info)
	}
}
