// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/state/presence"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/state/workers"
	"github.com/juju/juju/worker/lease"
)

type workersFactory struct {
	st    *State
	clock clock.Clock
}

func (wf workersFactory) NewTxnLogWorker() (workers.TxnLogWorker, error) {
	coll := wf.st.getTxnLogCollection()
	worker := watcher.New(coll)
	return worker, nil
}

func (wf workersFactory) NewPresenceWorker() (workers.PresenceWorker, error) {
	coll := wf.st.getPresenceCollection()
	worker := presence.NewWatcher(coll, wf.st.ModelTag())
	return worker, nil
}

func (wf workersFactory) NewLeadershipWorker() (workers.LeaseWorker, error) {
	client, err := wf.st.getLeadershipLeaseClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	manager, err := lease.NewManager(lease.ManagerConfig{
		Secretary: leadershipSecretary{},
		Client:    client,
		Clock:     wf.clock,
		MaxSleep:  time.Minute,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return manager, nil
}

func (wf workersFactory) NewSingularWorker() (workers.LeaseWorker, error) {
	client, err := wf.st.getSingularLeaseClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	manager, err := lease.NewManager(lease.ManagerConfig{
		Secretary: singularSecretary{wf.st.ModelUUID()},
		Client:    client,
		Clock:     wf.clock,
		MaxSleep:  time.Minute,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return manager, nil
}
