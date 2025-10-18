package tracing

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel/propagation"
)

// MetadataSupplier is the supplier for the metadata from context.
type MetadataSupplier struct {
	metadata map[string][]string
}

// assert that MetadataSupplier implements the TextMapCarrier interface.
var _ propagation.TextMapCarrier = &MetadataSupplier{}

// NewMetadataSupplier creates a new MetadataSupplier.
func NewMetadataSupplier(md map[string][]string) *MetadataSupplier {
	return &MetadataSupplier{metadata: md}
}

// Get returns the value associated with the passed key.
func (s *MetadataSupplier) Get(key string) string {
	values := s.metadata[strings.ToLower(key)]
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// Set stores the key-value pair.
func (s *MetadataSupplier) Set(key string, value string) {
	s.metadata[strings.ToLower(key)] = []string{value}
}

// Keys lists the keys stored in this carrier.
func (s *MetadataSupplier) Keys() []string {
	keys := make([]string, 0, len(s.metadata))
	for k := range s.metadata {
		keys = append(keys, k)
	}
	return keys
}

// Inject injects the trace context into the metadata.
func Inject(ctx context.Context, metadata map[string][]string, propagators propagation.TextMapPropagator) {
	propagators.Inject(ctx, NewMetadataSupplier(metadata))
}

// Extract extracts the trace context from the metadata.
func Extract(ctx context.Context, metadata map[string][]string, propagators propagation.TextMapPropagator) context.Context {
	return propagators.Extract(ctx, NewMetadataSupplier(metadata))
}
