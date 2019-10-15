// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradedatabase

import (
	"github.com/juju/errors"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state"
)

//go:generate mockgen -package mocks -destination mocks/package.go github.com/juju/juju/worker/upgradedatabase Logger,Pool
//go:generate mockgen -package mocks -destination mocks/lock.go github.com/juju/juju/worker/gate Lock
//go:generate mockgen -package mocks -destination mocks/agent.go github.com/juju/juju/agent Agent,Config

// Logger represents the methods required to emit log messages.
type Logger interface {
	Debugf(message string, args ...interface{})
}

// State describes methods required by the upgradeDB worker
// that use access to a state pool.
type Pool interface {
	// IsPrimary returns true if the Mongo primary is
	// running on the machine with the input ID.
	IsPrimary(string) (bool, error)

	// Close closes the state pool.
	Close() error
}

type pool struct {
	*state.StatePool
}

// IsPrimary implements Pool.
func (p *pool) IsPrimary(machineID string) (bool, error) {
	st := p.SystemState()

	machine, err := st.Machine(machineID)
	if err != nil {
		return false, errors.Trace(err)
	}

	isPrimary, err := mongo.IsMaster(st.MongoSession(), machine)
	return isPrimary, errors.Trace(err)
}
