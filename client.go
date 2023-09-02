package httpx

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

const (
	TraceName                  = "httpx"
	DefaultMaxIdleConnsPerHost = 1000
	DefaultIdleConnTimeout     = 60 * time.Second
)

const (
	PostJsonContentType          = "application/json"
	PostFormContentType          = "application/x-www-form-urlencoded"
	PostMultipartFormContentType = "multipart/form-data"
	PostStreamContentType        = "application/octet-stream"
)

var DefaultClient = Client{&http.Client{
	Transport: &http.Transport{
		MaxIdleConnsPerHost: DefaultMaxIdleConnsPerHost,
		IdleConnTimeout:     DefaultIdleConnTimeout,
	},
}}

type Client struct {
	*http.Client
}

type ClientOption struct {
	MaxIdleConnsPerHost int
	IdleConnTimeout     time.Duration
}

type ClientOptionFunc func(*ClientOption)

func NewClient(opts ...ClientOptionFunc) *Client {
	opt := ClientOption{
		MaxIdleConnsPerHost: DefaultMaxIdleConnsPerHost,
		IdleConnTimeout:     DefaultIdleConnTimeout,
	}

	for _, o := range opts {
		o(&opt)
	}

	return &Client{&http.Client{
		Transport: &http.Transport{
			MaxIdleConnsPerHost: opt.MaxIdleConnsPerHost,
			IdleConnTimeout:     opt.IdleConnTimeout,
		},
	}}
}

func WithMaxIdleConnsPerHost(n int) ClientOptionFunc {
	return func(opt *ClientOption) {
		opt.MaxIdleConnsPerHost = n
	}
}

func WithIdleConnTimeout(t time.Duration) ClientOptionFunc {
	return func(opt *ClientOption) {
		opt.IdleConnTimeout = t
	}
}

func (c *Client) Do(req *http.Request) (resp *http.Response, err error) {
	return c.do(req)
}

func (c *Client) Head(url string) (resp *http.Response, err error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

func (c *Client) Get(url string) (resp *http.Response, err error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

func (c *Client) Post(url, contentType string, body io.Reader) (resp *http.Response, err error) {
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)
	return c.Do(req)
}

func (c *Client) PostForm(url string, data url.Values) (resp *http.Response, err error) {
	return c.Post(url, PostFormContentType, strings.NewReader(data.Encode()))
}

func (c *Client) do(req *http.Request) (resp *http.Response, err error) {
	ctx := req.Context()
	tracer := tracerFromContext(ctx)
	propagator := otel.GetTextMapPropagator()

	spanName := req.URL.Path
	ctx, span := tracer.Start(
		ctx,
		spanName,
		oteltrace.WithSpanKind(oteltrace.SpanKindClient),
		oteltrace.WithAttributes(semconv.HTTPClientAttributesFromHTTPRequest(req)...),
	)
	defer span.End()

	req = req.WithContext(ctx)
	propagator.Inject(ctx, propagation.HeaderCarrier(req.Header))

	if resp, err = c.Client.Do(req); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}
	span.SetAttributes(semconv.HTTPAttributesFromHTTPStatusCode(resp.StatusCode)...)
	span.SetStatus(semconv.SpanStatusFromHTTPStatusCodeAndSpanKind(resp.StatusCode, oteltrace.SpanKindClient))

	return resp, nil
}

func tracerFromContext(ctx context.Context) (tracer trace.Tracer) {
	if span := trace.SpanFromContext(ctx); span.SpanContext().IsValid() {
		tracer = span.TracerProvider().Tracer(TraceName)
	} else {
		tracer = otel.Tracer(TraceName)
	}

	return
}
