// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"time"

	"github.com/hashicorp/raft"

	"github.com/juju/errors"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/raftlease"
)

type Logger interface {
	Debugf(string, ...interface{})
	Tracef(string, ...interface{})
}

// raftMediator encapsulates raft related capabilities to the facades.
type raftMediator struct {
	raft         Raft
	notifyTarget raftlease.NotifyTarget
	logger       Logger
}

// ApplyLease attempts to apply the command on to the raft FSM.
func (ctx *raftMediator) ApplyLease(cmd []byte, timeout time.Duration) error {
	if state := ctx.raft.State(); state != raft.Leader {
		leaderAddress := ctx.raft.Leader()

		ctx.logger.Debugf("Attempt to apply the lease failed, we're not the leader. State: %v, Leader: %v", state, leaderAddress)

		// If the leaderAddress is empty, this implies that either we don't
		// have a leader or there is no raft cluster setup.
		if leaderAddress == "" {
			// Return back we don't have a leader and then it's up to the client
			// to work out what to do next.
			return apiservererrors.NewNotLeaderError("", "")
		}

		// If we have a leader address, we hope that we can use that leader
		// address to locate the associate server ID. The server ID can be used
		// as a mapping for the machine ID.
		future := ctx.raft.GetConfiguration()
		if err := future.Error(); err != nil {
			return errors.Trace(err)
		}

		config := future.Configuration()

		// If no leader ID is located that could imply that the leader has gone
		// away during the raft.State call and the GetConfiguration call. If
		// this is the case, return the leader address and no leader ID and get
		// the client to figure out the best approach.
		var leaderID string
		for _, server := range config.Servers {
			if server.Address == leaderAddress {
				leaderID = string(server.ID)
				break
			}
		}

		return apiservererrors.NewNotLeaderError(string(leaderAddress), leaderID)
	}

	ctx.logger.Tracef("Applying command %v", string(cmd))

	future := ctx.raft.Apply(cmd, timeout)
	if err := future.Error(); err != nil {
		return errors.Trace(err)
	}

	response := future.Response()
	fsmResponse, ok := response.(raftlease.FSMResponse)
	if !ok {
		// This should never happen.
		panic(errors.Errorf("programming error: expected an FSMResponse, got %T: %#v", response, response))
	}

	fsmResponse.Notify(ctx.notifyTarget)

	return nil
}
