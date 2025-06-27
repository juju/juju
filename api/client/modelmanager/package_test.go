// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	"github.com/juju/juju/api/base"
)

func NewClientFromCaller(caller base.FacadeCaller) *Client {
	return &Client{
		facade:       caller,
		ClientFacade: &mockClient{},
	}
}

type mockClient struct {
}

func (m *mockClient) BestAPIVersion() int {
	return 11
}

func (*mockClient) Close() error {
	return nil
}
