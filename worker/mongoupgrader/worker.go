// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongoupgrader

import (
	"net"
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker"
	"github.com/juju/loggo"
	"github.com/juju/replicaset"
)

var logger = loggo.GetLogger("juju.worker.mongoupgrader")

// StopMongo represents a function that can issue a stop
// to a running mongo service.
type StopMongo func(mongo.Version, bool) error

// New returns a worker or err in case of failure.
// this worker takes care of watching the state of machine's upgrade
// mongo information and change agent conf accordingly.
func New(st *state.State, machineID string, maybeStopMongo StopMongo) (worker.Worker, error) {
	upgradeWorker := func(stopch <-chan struct{}) error {
		return upgradeMongoWatcher(st, stopch, machineID, maybeStopMongo)
	}
	return worker.NewSimpleWorker(upgradeWorker), nil
}

func upgradeMongoWatcher(st *state.State, stopch <-chan struct{}, machineID string, maybeStopMongo StopMongo) error {
	m, err := st.Machine(machineID)
	if err != nil {
		return errors.Annotatef(err, "cannot start watcher for machine %q", machineID)
	}
	watch := m.Watch()
	defer func() {
		watch.Kill()
		watch.Wait()
	}()

	for {
		select {
		case <-watch.Changes():
			if err := m.Refresh(); err != nil {
				return errors.Annotate(err, "cannot refresh machine information")
			}
			if !m.IsManager() {
				continue
			}
			expectedVersion, err := m.StopMongoUntilVersion()
			if err != nil {
				return errors.Annotate(err, "cannot obtain minimum version of mongo")
			}
			if expectedVersion == mongo.Mongo24 {
				continue
			}
			var isMaster bool
			isMaster, err = mongo.IsMaster(st.MongoSession(), m)
			if err != nil {
				return errors.Annotatef(err, "cannot determine if machine %q is master", machineID)
			}

			err = maybeStopMongo(expectedVersion, isMaster)
			if err != nil {
				return errors.Annotate(err, "cannot determine if mongo must be stopped")
			}
			if !isMaster {
				addrs := make([]string, len(m.Addresses()))
				ssi, err := st.StateServingInfo()
				if err != nil {
					return errors.Annotate(err, "cannot obtain state serving info to stop mongo")
				}
				for i, addr := range m.Addresses() {
					addrs[i] = net.JoinHostPort(addr.Value, strconv.Itoa(ssi.StatePort))
				}
				if err := replicaset.Remove(st.MongoSession(), addrs...); err != nil {
					return errors.Annotatef(err, "cannot remove %q from replicaset", m.Id())
				}
				if err := m.SetStopMongoUntilVersion(mongo.Mongo24); err != nil {
					return errors.Annotate(err, "cannot reset stop mongo flag")
				}
			}
		case <-stopch:
			return nil
		}
	}
}
