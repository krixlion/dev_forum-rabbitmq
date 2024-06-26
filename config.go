package rabbitmq

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/embedded"
)

type Config struct {
	QueueSize         int           // Max number of messages internally queued for publishing.
	MaxWorkers        int           // Max number of concurrent workers per operation type.
	ReconnectInterval time.Duration // Time between reconnect attempts.

	// Settings for the internal circuit breaker.
	MaxRequests   uint32        // Number of requests allowed to half-open state.
	ClearInterval time.Duration // Time after which failed calls count is cleared.
	ClosedTimeout time.Duration // Time after which closed state becomes half-open.
}

func DefaultConfig() Config {
	return Config{
		QueueSize:         100,              // Max number of messages internally queued for publishing.
		MaxWorkers:        30,               // Max number of concurrent workers per operation type.
		ReconnectInterval: time.Second * 2,  // Time between reconnect attempts.
		MaxRequests:       10,               // Number of requests allowed to half-open state.
		ClearInterval:     time.Second * 10, // Time after which failed calls count is cleared.
		ClosedTimeout:     time.Second * 10, // Time after which closed state becomes half-open.
	}
}

type Logger interface {
	Log(ctx context.Context, msg string, keyvals ...interface{})
}

type Option interface {
	apply(*options)
}

func WithTracer(tracer trace.Tracer) Option {
	return tracerOption{tracer}
}

func WithLogger(logger Logger) Option {
	return loggerOption{logger}
}

var _ Logger = (*nullLogger)(nil)

type nullLogger struct{}

func (nullLogger) Log(context.Context, string, ...any) {}

var _ trace.Tracer = (*nullTracer)(nil)

type nullTracer struct {
	embedded.Tracer
}

func (nullTracer) Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return ctx, nullSpan{}
}

type nullSpan struct {
	embedded.Span
}

func (nullSpan) AddLink(link trace.Link)                             {}
func (nullSpan) End(options ...trace.SpanEndOption)                  {}
func (nullSpan) AddEvent(name string, options ...trace.EventOption)  {}
func (nullSpan) IsRecording() bool                                   { return false }
func (nullSpan) RecordError(err error, options ...trace.EventOption) {}
func (nullSpan) SpanContext() trace.SpanContext                      { return trace.SpanContext{} }
func (nullSpan) SetStatus(code codes.Code, description string)       {}
func (nullSpan) SetName(name string)                                 {}
func (nullSpan) SetAttributes(kv ...attribute.KeyValue)              {}
func (nullSpan) TracerProvider() trace.TracerProvider                { return nil }

func setSpanErr(span trace.Span, err error) {
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

type options struct {
	tracer trace.Tracer
	logger Logger
}

func defaultOptions() options {
	return options{
		tracer: nullTracer{},
		logger: nullLogger{},
	}
}

type tracerOption struct {
	tracer trace.Tracer
}

func (opt tracerOption) apply(opts *options) {
	opts.tracer = opt.tracer
}

type loggerOption struct {
	logger Logger
}

func (opt loggerOption) apply(opts *options) {
	opts.logger = opt.logger
}
