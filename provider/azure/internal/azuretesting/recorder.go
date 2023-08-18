// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azuretesting

import (
	"bytes"
	"io"
	"net/http"
	"sync"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
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
