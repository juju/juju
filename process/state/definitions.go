// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/process/persistence"
)

// The persistence methods needed for workload processes in state.
type definitionsPersistence interface {
	EnsureDefinitions(definitions ...charm.Process) ([]string, []string, error)
}

// Definitions provides the definition-related functionality
// needed by state.
type Definitions struct {
	// Persist is the persistence layer that will be used.
	Persist definitionsPersistence
}

// NewDefinitions builds a new Definitions for the charm.
func NewDefinitions(st persistence.PersistenceBase, charm names.CharmTag) *Definitions {
	persist := persistence.NewPersistence(st, &charm, nil)
	return &Definitions{
		Persist: persist,
	}
}

// EnsureDefined makes sure that all the provided definitions exist in
// state. So either they are there already or they get added.
func (pd Definitions) EnsureDefined(definitions ...charm.Process) error {
	for _, definition := range definitions {
		if err := definition.Validate(); err != nil {
			return errors.NewNotValid(err, "bad definition")
		}
	}
	_, mismatched, err := pd.Persist.EnsureDefinitions(definitions...)
	if err != nil {
		return errors.Trace(err)
	}
	if len(mismatched) > 0 {
		return errors.NotValidf("mismatched definitions for %v", mismatched)
	}
	return nil
}
