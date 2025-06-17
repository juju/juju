// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasmodeloperator

import (
	"github.com/juju/names/v6"

	"github.com/juju/juju/state"
)

// CAASModelOperatorState provides the subset of model state required by the
// model operator provisioner.
type CAASModelOperatorState interface {
	FindEntity(tag names.Tag) (state.Entity, error)
}
