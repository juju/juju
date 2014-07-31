// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"github.com/juju/juju/network"
)

var (
	NewWebsocketDialer  = newWebsocketDialer
	WebsocketDialConfig = &websocketDialConfig
	SetUpWebsocket      = setUpWebsocket
	SlideAddressToFront = slideAddressToFront
	BestVersion         = bestVersion
	FacadeVersions      = &facadeVersions
)

// SetServerRoot allows changing the URL to the internal API server
// that AddLocalCharm uses in order to test NotImplementedError.
func SetServerRoot(c *Client, root string) {
	c.st.serverRoot = root
}

// SetEnvironTag patches the value of the environment tag.
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
	ServerRoot     string
}

// NewTestingState creates an api.State object that can be used for testing. It
// isn't backed onto an actual API server, so actual RPC methods can't be
// called on it. But it can be used for testing general behavior.
func NewTestingState(params TestingStateParams) *State {
	st := &State{
		addr:           params.Address,
		environTag:     params.EnvironTag,
		hostPorts:      params.APIHostPorts,
		facadeVersions: params.FacadeVersions,
		serverRoot:     params.ServerRoot,
	}
	return st
}
