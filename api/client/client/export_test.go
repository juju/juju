// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"context"
	"github.com/golang/mock/gomock"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/proxy"
	"github.com/juju/names/v4"
	"github.com/juju/version/v2"
	"gopkg.in/macaroon.v2"
	"net/url"
)

// PatchClientFacadeCall changes the internal FacadeCaller to one that lets
// you mock out the FacadeCall method. The function returned by
// PatchClientFacadeCall is a cleanup function that returns the client to its
// original state.
func PatchClientFacadeCall(c *Client, mockCall func(request string, params interface{}, response interface{}) error) func() {
	orig := c.facade
	c.facade = &resultCaller{mockCall}
	return func() {
		c.facade = orig
	}
}

type resultCaller struct {
	mockCall func(request string, params interface{}, response interface{}) error
}

func (f *resultCaller) FacadeCall(request string, params, response interface{}) error {
	return f.mockCall(request, params, response)
}

func (f *resultCaller) Name() string {
	return ""
}

func (f *resultCaller) BestAPIVersion() int {
	return 0
}

func (f *resultCaller) RawAPICaller() base.APICaller {
	return &rawAPICaller{}
}

type FakeAPICaller struct {
	*mocks.MockAPICaller
}

// NewFakeAPICaller creates a new fake instance on top of Mock Instance.
func NewFakeAPICaller(ctrl *gomock.Controller) FakeAPICaller {
	mock := mocks.NewMockAPICaller(ctrl)

	return FakeAPICaller{mock}
}

func (c FakeAPICaller) Close() error {
	panic("not implemented")
}

func (c FakeAPICaller) Addr() string {
	panic("not implemented")
}

func (c FakeAPICaller) IPAddr() string {
	panic("not implemented")
}

func (c FakeAPICaller) APIHostPorts() []network.MachineHostPorts {
	panic("not implemented")
}

func (c FakeAPICaller) Broken() <-chan struct{} {
	panic("not implemented")
}

func (c FakeAPICaller) IsBroken() bool {
	panic("not implemented")
}

func (c FakeAPICaller) IsProxied() bool {
	panic("not implemented")
}

func (c FakeAPICaller) Proxy() proxy.Proxier {
	panic("not implemented")
}

func (c FakeAPICaller) PublicDNSName() string {
	panic("not implemented")
}

func (c FakeAPICaller) Login(name names.Tag, password, nonce string, ms []macaroon.Slice) error {
	panic("not implemented")
}

func (c FakeAPICaller) ServerVersion() (version.Number, bool) {
	panic("not implemented")
}

func (c FakeAPICaller) ControllerTag() names.ControllerTag {
	panic("not implemented")
}

func (c FakeAPICaller) AuthTag() names.Tag {
	panic("not implemented")
}

func (c FakeAPICaller) ControllerAccess() string {
	panic("not implemented")
}

func (c FakeAPICaller) CookieURL() *url.URL {
	panic("not implemented")
}

type rawAPICaller struct {
	base.APICaller
}

func (r *rawAPICaller) Context() context.Context {
	return context.Background()
}

// SetServerAddress allows changing the URL to the internal API server
// that AddLocalCharm uses in order to test NotImplementedError.
func SetServerAddress(c *Client, scheme, addr string) {
	api.SetServerAddressForTesting(c.conn, scheme, addr)
}
