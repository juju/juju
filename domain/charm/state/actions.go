// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"github.com/juju/juju/domain/charm"
)

func decodeActions(actions []charmAction) charm.Actions {
	result := charm.Actions{
		Actions: make(map[string]charm.Action),
	}
	for _, action := range actions {
		result.Actions[action.Key] = charm.Action{
			Description:    action.Description,
			Parallel:       action.Parallel,
			ExecutionGroup: action.ExecutionGroup,
			Params:         action.Params,
		}
	}
	return result
}
