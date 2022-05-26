// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

func NewClientForTest(caller base.FacadeCaller, httpClient api.HTTPDoer) *Client {
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
