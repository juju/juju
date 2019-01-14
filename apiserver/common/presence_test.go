// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/presence"
)

func agentAlive(agent string) common.ModelPresence {
	return &fakeModelPresence{status: presence.Alive, agent: agent}
}

func agentDown(agent string) common.ModelPresence {
	return &fakeModelPresence{status: presence.Missing, agent: agent}
}

func presenceError(agent string) common.ModelPresence {
	return &fakeModelPresence{err: errors.New("boom"), agent: agent}
}

type fakeModelPresence struct {
	agent  string
	status presence.Status
	err    error
}

func (f *fakeModelPresence) AgentStatus(agent string) (presence.Status, error) {
	if agent != f.agent {
		return f.status, fmt.Errorf("unexpected agent %v, expected %v", agent, f.agent)
	}
	return f.status, f.err
}
