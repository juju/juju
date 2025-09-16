// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"encoding/json"

	"github.com/juju/juju/domain/application/charm"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
)

func decodeActions(actions charm.Actions) (internalcharm.Actions, error) {
	if len(actions.Actions) == 0 {
		return internalcharm.Actions{}, nil
	}

	result := make(map[string]internalcharm.ActionSpec)
	for name, action := range actions.Actions {
		params, err := decodeActionParams(action.Params)
		if err != nil {
			return internalcharm.Actions{}, errors.Errorf("decode action params: %w", err)
		}

		result[name] = internalcharm.ActionSpec{
			Description:    action.Description,
			Parallel:       action.Parallel,
			ExecutionGroup: action.ExecutionGroup,
			Params:         params,
		}
	}
	return internalcharm.Actions{
		ActionSpecs: result,
	}, nil
}

func decodeActionParams(params []byte) (map[string]any, error) {
	if len(params) == 0 {
		return nil, nil
	}

	var result map[string]any
	if err := json.Unmarshal(params, &result); err != nil {
		return nil, errors.Errorf("unmarshal: %w", err)
	}
	return result, nil
}

func encodeActions(actions *internalcharm.Actions) (charm.Actions, error) {
	if actions == nil || len(actions.ActionSpecs) == 0 {
		return charm.Actions{}, nil
	}

	result := make(map[string]charm.Action)
	for name, action := range actions.ActionSpecs {
		params, err := encodeActionParams(action.Params)
		if err != nil {
			return charm.Actions{}, errors.Errorf("encode action params: %w", err)
		}

		result[name] = charm.Action{
			Description:    action.Description,
			Parallel:       action.Parallel,
			ExecutionGroup: action.ExecutionGroup,
			Params:         params,
		}
	}
	return charm.Actions{
		Actions: result,
	}, nil
}

func encodeActionParams(params map[string]any) ([]byte, error) {
	if len(params) == 0 {
		return nil, nil
	}

	result, err := json.Marshal(params)
	if err != nil {
		return nil, errors.Errorf("marshal: %w", err)
	}

	return result, nil
}
