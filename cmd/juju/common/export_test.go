// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/cmd"

	"github.com/juju/juju/cmd/envcmd"
)

// NewGetConstraintsCommand returns a GetCommand with the api provided as specified.
func NewGetConstraintsCommandWithAPI(api ConstraintsAPI) cmd.Command {
	return envcmd.Wrap(&GetConstraintsCommand{
		api: api,
	})
}

// NewGetConstraintsCommand returns a GetCommand with the api provided as specified.
func NewSetConstraintsCommandWithAPI(api ConstraintsAPI) cmd.Command {
	return envcmd.Wrap(&SetConstraintsCommand{
		api: api,
	})
}
