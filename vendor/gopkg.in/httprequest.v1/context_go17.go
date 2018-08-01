// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

// +build go1.7

package httprequest

import (
	"context"
	"net/http"
)

func contextFromRequest(req *http.Request) (context.Context, context.CancelFunc) {
	return req.Context(), func() {}
}

func requestWithContext(req *http.Request, ctx context.Context) *http.Request {
	return req.WithContext(ctx)
}
