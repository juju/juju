// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/presence"
)

func allAlive() common.ModelPresence {
	return &fakeModelPresence{status: presence.Alive}
}

func agentsDown() common.ModelPresence {
	return &fakeModelPresence{status: presence.Missing}
}

func presenceError() common.ModelPresence {
	return &fakeModelPresence{err: errors.New("boom")}
}

type fakeModelPresence struct {
	status presence.Status
	err    error
}

func (f *fakeModelPresence) AgentStatus(agent string) (presence.Status, error) {
	return f.status, f.err
}
