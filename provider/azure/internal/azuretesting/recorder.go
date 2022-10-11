// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azuretesting

import (
	"bytes"
	"io"
	"net/http"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/go-autorest/autorest"
)

type RequestRecorderPolicy struct {
	mu       sync.Mutex
	Requests *[]*http.Request
}

func (p *RequestRecorderPolicy) Do(req *policy.Request) (*http.Response, error) {
	resp, err := req.Next()
	// Save the request body, since it will be consumed.
	reqCopy := *req.Raw()
	if req.Raw().Body != nil {
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(req.Raw().Body); err != nil {
			return nil, err
		}
		if err := req.Raw().Body.Close(); err != nil {
			return nil, err
		}
		reqCopy.Body = io.NopCloser(&buf)
		if err := req.RewindBody(); err != nil {
			return nil, err
		}
	}
	p.mu.Lock()
	*p.Requests = append(*p.Requests, &reqCopy)
	p.mu.Unlock()

	return resp, err
}

// RequestRecorder returns an autorest.PrepareDecorator that records requests
// to ghe given slice.
func RequestRecorder(requests *[]*http.Request) autorest.PrepareDecorator {
	if requests == nil {
		return nil
	}
	var mu sync.Mutex
	return func(p autorest.Preparer) autorest.Preparer {
		return autorest.PreparerFunc(func(req *http.Request) (*http.Request, error) {
			// Save the request body, since it will be consumed.
			reqCopy := *req
			if req.Body != nil {
				var buf bytes.Buffer
				if _, err := buf.ReadFrom(req.Body); err != nil {
					return nil, err
				}
				if err := req.Body.Close(); err != nil {
					return nil, err
				}
				reqCopy.Body = io.NopCloser(&buf)
				req.Body = io.NopCloser(bytes.NewReader(buf.Bytes()))
			}
			mu.Lock()
			*requests = append(*requests, &reqCopy)
			mu.Unlock()
			return req, nil
		})
	}
}
