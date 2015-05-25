// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/network"
)

var (
	CertDir               = &certDir
	NewWebsocketDialer    = newWebsocketDialer
	NewWebsocketDialerPtr = &newWebsocketDialer
	WebsocketDialConfig   = &websocketDialConfig
	SlideAddressToFront   = slideAddressToFront
	BestVersion           = bestVersion
	FacadeVersions        = &facadeVersions
	NewHTTPClient         = &newHTTPClient
)

// SetServerAddress allows changing the URL to the internal API server
// that AddLocalCharm uses in order to test NotImplementedError.
func SetServerAddress(c *Client, scheme, addr string) {
	c.st.serverScheme = scheme
	c.st.addr = addr
}

// ServerRoot is exported so that we can test the built URL.
func ServerRoot(c *Client) string {
	return c.st.serverRoot()
}

// PatchEnvironTag patches the value of the environment tag.
// It returns a function that reverts the change.
func PatchEnvironTag(st *State, envTag string) func() {
	originalTag := st.environTag
	st.environTag = envTag
	return func() {
		st.environTag = originalTag
	}
}

// TestingStateParams is the parameters for NewTestingState, so that you can
// only set the bits that you acutally want to test.
type TestingStateParams struct {
	Address        string
	EnvironTag     string
	APIHostPorts   [][]network.HostPort
	FacadeVersions map[string][]int
	ServerScheme   string
	ServerRoot     string
}

// NewTestingState creates an api.State object that can be used for testing. It
// isn't backed onto an actual API server, so actual RPC methods can't be
// called on it. But it can be used for testing general behavior.
func NewTestingState(params TestingStateParams) *State {
	st := &State{
		addr:              params.Address,
		environTag:        params.EnvironTag,
		hostPorts:         params.APIHostPorts,
		facadeVersions:    params.FacadeVersions,
		serverScheme:      params.ServerScheme,
		serverRootAddress: params.ServerRoot,
	}
	return st
}

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
	return nil
}
