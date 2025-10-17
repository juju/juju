// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	"github.com/juju/juju/api/base"
)

func NewClientFromCaller(caller base.FacadeCaller) *Client {
	return &Client{
		facade:       caller,
		ClientFacade: &mockClient{bestAPIVersion: 11},
	}
}

type mockClient struct {
	bestAPIVersion int
}

func (m *mockClient) BestAPIVersion() int {
	return m.bestAPIVersion
}

func (*mockClient) Close() error {
	return nil
}

func NewLegacyClientFromCaller(caller base.FacadeCaller) *Client {
	return &Client{
		facade:       caller,
		ClientFacade: &mockClient{bestAPIVersion: 10},
	}
}
