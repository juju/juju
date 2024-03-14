// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import "github.com/juju/juju/core/modelmigration"

// IgnoredSetupOperation is a helper function to test the operation within a
// coordinator.
// This just ignores the setup call of the coordinator. It is expected that
// the operation will have all the information up front.
func IgnoredSetupOperation(op modelmigration.Operation) modelmigration.Operation {
	return &ignoredSetupOperation{Operation: op}
}

type ignoredSetupOperation struct {
	modelmigration.Operation
}

func (i *ignoredSetupOperation) Setup(modelmigration.Scope) error {
	return nil
}
