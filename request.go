package httpx

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/avast/retry-go"
)

const (
	FixedDelay        = "FixedDelay"
	BackOffDelay      = "BackOffDelay"
	DefaultRetryDelay = time.Millisecond * 100
)

type Request struct {
	Ctx            context.Context
	Timeout        time.Duration
	Method         string
	Url            string
	ContentType    string
	Header         http.Header
	Body           []byte
	ExpectCode     []int
	RetryAttempts  uint
	RetryDelay     time.Duration
	RetryDelayType string
	LastErrorOnly  bool
	OnRetry        func(n uint, err error)
}

func (c *Client) DoRequest(req *Request) ([]byte, error) {
	var (
		resp      []byte
		err       error
		delayType retry.DelayTypeFunc
	)

	if req == nil {
		req = &Request{}
	}

	if req.Ctx == nil {
		req.Ctx = context.Background()
	}

	if req.RetryAttempts == 0 {
		req.RetryAttempts = 1
	}

	if req.RetryDelay == 0 {
		req.RetryDelay = DefaultRetryDelay
	}

	if req.RetryDelayType == BackOffDelay {
		delayType = retry.BackOffDelay
	} else {
		delayType = retry.FixedDelay
	}

	opts := []retry.Option{
		retry.Context(req.Ctx),
		retry.Delay(req.RetryDelay),
		retry.Attempts(req.RetryAttempts),
		retry.DelayType(delayType),
		retry.LastErrorOnly(req.LastErrorOnly),
	}

	if req.OnRetry != nil {
		opts = append(opts, retry.OnRetry(req.OnRetry))
	}

	err = retry.Do(
		func() error {
			resp, err = c.doRequest(req)
			if err != nil {
				return err
			}
			return nil
		},
		opts...,
	)

	return resp, err
}

func (c *Client) doRequest(req *Request) ([]byte, error) {
	var (
		allow  bool
		resp   []byte
		ctx    context.Context
		cancel context.CancelFunc
	)

	if req == nil {
		req = &Request{}
	}

	if req.Ctx != nil {
		ctx = req.Ctx
	}

	if req.Timeout > 0 {
		ctx, cancel = context.WithTimeout(req.Ctx, req.Timeout)
		defer cancel()
	}

	request, err := http.NewRequestWithContext(ctx, req.Method, req.Url, bytes.NewBuffer(req.Body))
	if err != nil {
		return nil, err
	}

	if req.ContentType != "" {
		req.Header.Set("Content-Type", req.ContentType)
	}

	for k, v := range req.Header {
		for _, vv := range v {
			req.Header.Add(k, vv)
		}
	}

	response, err := c.Client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if req.ExpectCode == nil {
		req.ExpectCode = append(req.ExpectCode, http.StatusOK)
	}

	for _, code := range req.ExpectCode {
		if response.StatusCode == code {
			allow = true
			break
		}
	}

	resp, err = io.ReadAll(response.Body)
	if err != nil {
		return resp, fmt.Errorf("http status:%v, error:%v", response.Status, err)
	}

	if !allow {
		return resp, fmt.Errorf("http status:%d, expectCode:%v", response.StatusCode, req.ExpectCode)
	}

	return resp, nil
}
