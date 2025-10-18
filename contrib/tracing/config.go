package tracing

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const (
	// ScopeName is the instrumentation scope name.
	ScopeName = "go.opentelemetry.io/contrib/instrumentation/github.com/crazyfrankie/zrpc"
)

// config contains configuration for the instrumentation.
type config struct {
	Filter          Filter
	Propagators     propagation.TextMapPropagator
	TracerProvider  trace.TracerProvider
	MeterProvider   metric.MeterProvider
	SpanStartOptions []trace.SpanStartOption

	ReceivedEvent bool
	SentEvent     bool
}

// Option applies an option value for a config.
type Option func(*config)

// newConfig returns a config configured with all the passed Options.
func newConfig(opts []Option) *config {
	c := &config{
		Propagators:    otel.GetTextMapPropagator(),
		TracerProvider: otel.GetTracerProvider(),
		MeterProvider:  otel.GetMeterProvider(),
		ReceivedEvent:  true,
		SentEvent:     true,
	}
	for _, o := range opts {
		o(c)
	}

	if c.Filter == nil {
		c.Filter = AcceptAll()
	}

	return c
}

// WithPropagators returns an Option to use the Propagators when extracting
// and injecting trace context from requests.
func WithPropagators(p propagation.TextMapPropagator) Option {
	return func(c *config) {
		if p != nil {
			c.Propagators = p
		}
	}
}

// WithTracerProvider returns an Option to use the TracerProvider when
// creating a Tracer.
func WithTracerProvider(tp trace.TracerProvider) Option {
	return func(c *config) {
		if tp != nil {
			c.TracerProvider = tp
		}
	}
}

// WithMeterProvider returns an Option to use the MeterProvider when
// creating a Meter.
func WithMeterProvider(mp metric.MeterProvider) Option {
	return func(c *config) {
		if mp != nil {
			c.MeterProvider = mp
		}
	}
}

// WithFilter returns an Option to use the provided Filter to
// determine whether a given request in zRPC should be traced.
func WithFilter(f Filter) Option {
	return func(c *config) {
		c.Filter = f
	}
}

// WithMessageEvents configures the Handler to record the message events
// (InPayload, OutPayload) on spans.
func WithMessageEvents(events bool) Option {
	return func(c *config) {
		c.ReceivedEvent = events
		c.SentEvent = events
	}
}

// WithReceivedMessageEvents configures the Handler to record the received message events
// (InPayload) on spans.
func WithReceivedMessageEvents(events bool) Option {
	return func(c *config) {
		c.ReceivedEvent = events
	}
}

// WithSentMessageEvents configures the Handler to record the sent message events
// (OutPayload) on spans.
func WithSentMessageEvents(events bool) Option {
	return func(c *config) {
		c.SentEvent = events
	}
}
