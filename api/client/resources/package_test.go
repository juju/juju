// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/http"
)

func NewClientForTest(caller base.FacadeCaller, httpClient http.HTTPDoer) *Client {
	return &Client{
		ClientFacade: noopCloser{caller},
		facade:       caller,
		httpClient:   httpClient,
	}
}

type noopCloser struct {
	base.FacadeCaller
}

func (noopCloser) Close() error {
	return nil
}

func NewClientFromCaller(caller base.FacadeCaller) *Client {
	return &Client{
		facade: caller,
	}
}
