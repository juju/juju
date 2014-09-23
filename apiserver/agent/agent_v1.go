// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agent

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/state"
)

// AgentAPIV1 implements the version 1 of the API provided to an agent.
type AgentAPIV1 struct {
	*AgentAPIV0
}

// NewAgentAPIV1 returns an object implementing version 1 of the Agent API
// with the given authorizer representing the currently logged in client.
// The functionality is like V0, except that it also knows about the additional
// JobManageNetworking.
func NewAgentAPIV1(st *state.State, resources *common.Resources, auth common.Authorizer) (*AgentAPIV1, error) {
	apiV0, err := NewAgentAPIV0(st, resources, auth)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &AgentAPIV1{apiV0}, nil
}
