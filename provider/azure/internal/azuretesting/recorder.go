// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azuretesting

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"sync"

	"github.com/Azure/go-autorest/autorest"
)

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
				reqCopy.Body = ioutil.NopCloser(&buf)
				req.Body = ioutil.NopCloser(bytes.NewReader(buf.Bytes()))
			}
			mu.Lock()
			*requests = append(*requests, &reqCopy)
			mu.Unlock()
			return req, nil
		})
	}
}
