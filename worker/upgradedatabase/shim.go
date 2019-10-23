// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradedatabase

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/state"
)

//go:generate mockgen -package mocks -destination mocks/package.go github.com/juju/juju/worker/upgradedatabase Logger,Pool
//go:generate mockgen -package mocks -destination mocks/lock.go github.com/juju/juju/worker/gate Lock
//go:generate mockgen -package mocks -destination mocks/agent.go github.com/juju/juju/agent Agent,Config,ConfigSetter

// Logger represents the methods required to emit log messages.
type Logger interface {
	Debugf(message string, args ...interface{})
	Infof(message string, args ...interface{})
	Errorf(message string, args ...interface{})
}

// State describes methods required by the upgradeDB worker
// that use access to a state pool.
type Pool interface {
	// IsPrimary returns true if the Mongo primary is
	// running on the machine with the input ID.
	IsPrimary(string) (bool, error)

	// SetStatus updates the status of the machine with the input ID.
	SetStatus(string, status.Status, string) error

	// Close closes the state pool.
	Close() error
}

type pool struct {
	*state.StatePool
}

// IsPrimary (Pool) returns true if the Mongo primary is
// running on the machine with the input ID.
func (p *pool) IsPrimary(nodeID string) (bool, error) {
	st := p.SystemState()

	// For IAAS models, controllers are machines.
	// For CAAS models, until we support HA, there's
	// only one mongo and it's the primary.
	model, err := st.Model()
	if err != nil {
		return false, errors.Trace(err)
	}
	// TODO(CAAS) - bug 1849030 support HA
	if model.Type() == state.ModelTypeCAAS {
		return true, nil
	}

	machine, err := st.Machine(nodeID)
	if err != nil {
		return false, errors.Trace(err)
	}
	isPrimary, err := mongo.IsMaster(st.MongoSession(), machine)
	return isPrimary, errors.Trace(err)
}

// SetStatus (Pool) updates the status of the machine with the input ID.
func (p *pool) SetStatus(machineID string, sts status.Status, msg string) error {
	machine, err := p.SystemState().Machine(machineID)
	if err != nil {
		return errors.Trace(err)
	}

	now := time.Now()
	return errors.Trace(machine.SetStatus(status.StatusInfo{
		Status:  sts,
		Message: msg,
		Since:   &now,
	}))
}
