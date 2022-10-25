// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/http"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

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
