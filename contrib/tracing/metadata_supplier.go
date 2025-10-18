package tracing

import (
	"context"

	"go.opentelemetry.io/otel/propagation"

	"github.com/crazyfrankie/zrpc/metadata"
)

// MetadataSupplier is the supplier for the metadata from context.
type MetadataSupplier struct {
	metadata metadata.MD
}

// assert that MetadataSupplier implements the TextMapCarrier interface.
var _ propagation.TextMapCarrier = &MetadataSupplier{}

// NewMetadataSupplier creates a new MetadataSupplier.
func NewMetadataSupplier(md metadata.MD) *MetadataSupplier {
	return &MetadataSupplier{metadata: md}
}

// Get returns the value associated with the passed key.
func (s *MetadataSupplier) Get(key string) string {
	values := s.metadata.Get(key)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// Set stores the key-value pair.
func (s *MetadataSupplier) Set(key string, value string) {
	s.metadata.Set(key, value)
}

// Keys lists the keys stored in this carrier.
func (s *MetadataSupplier) Keys() []string {
	keys := make([]string, 0, len(s.metadata))
	for k := range s.metadata {
		keys = append(keys, k)
	}
	return keys
}

// inject injects the trace context into outgoing metadata
func inject(ctx context.Context, propagators propagation.TextMapPropagator) context.Context {
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		md = metadata.MD{}
	}

	propagators.Inject(ctx, NewMetadataSupplier(md))
	return metadata.NewOutgoingContext(ctx, md)
}

// extract extracts the trace context from incoming metadata
func extract(ctx context.Context, propagators propagation.TextMapPropagator) context.Context {
	md, ok := metadata.FromInComingContext(ctx)
	if !ok {
		md = metadata.MD{}
	}

	return propagators.Extract(ctx, NewMetadataSupplier(md))
}
