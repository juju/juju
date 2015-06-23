// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	jujutxn "github.com/juju/txn"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/process"
)

// TODO(ericsnow) Implement persistence using a TXN abstraction (used
// in the business logic) with ops factories available from the
// persistence layer.

type processesPersistenceBase interface {
	run(transactions jujutxn.TransactionSource) error
}

type processesPersistence struct {
	st processesPersistenceBase
}

func (pp processesPersistence) ensureDefinitions(ids []string, definitions []charm.Process, unit string) error {
	// Add definition if not already added (or ensure matches).

	// TODO(ericsnow) finish!
	return errors.Errorf("not finished")
}

func (pp processesPersistence) insert(id, charm string, info process.Info) error {
	// Ensure defined.

	// Add launch info.
	// Add process info.

	// TODO(ericsnow) finish!
	return errors.Errorf("not finished")
}

func (pp processesPersistence) setStatus(id string, status process.Status) error {
	// TODO(ericsnow) finish!
	return errors.Errorf("not finished")
}

func (pp processesPersistence) list(ids ...string) ([]process.Info, error) {
	// TODO(ericsnow) finish!
	return nil, errors.Errorf("not finished")
}

func (pp processesPersistence) remove(id string) error {
	// TODO(ericsnow) finish!
	return errors.Errorf("not finished")
}
