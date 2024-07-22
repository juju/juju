// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/domain/application/charm"
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

func encodeActions(id corecharm.ID, actions charm.Actions) []setCharmAction {
	result := make([]setCharmAction, 0, len(actions.Actions))
	for key, action := range actions.Actions {
		if action.Params == nil {
			action.Params = make([]byte, 0)
		}
		result = append(result, setCharmAction{
			CharmUUID:      id.String(),
			Key:            key,
			Description:    action.Description,
			Parallel:       action.Parallel,
			ExecutionGroup: action.ExecutionGroup,
			Params:         action.Params,
		})
	}
	return result
}
