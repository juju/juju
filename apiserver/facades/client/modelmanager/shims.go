// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/space"
)

type spaceStateShim struct {
	common.ModelManagerBackend
}

func (s spaceStateShim) AllSpaces() ([]space.Space, error) {
	spaces, err := s.ModelManagerBackend.AllSpaces()
	if err != nil {
		return nil, errors.Trace(err)
	}

	results := make([]space.Space, len(spaces))
	for i, space := range spaces {
		results[i] = space
	}
	return results, nil
}

func (s spaceStateShim) AddSpace(name string, providerId network.Id, subnetIds []string, public bool) (space.Space, error) {
	result, err := s.ModelManagerBackend.AddSpace(name, providerId, subnetIds, public)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return result, nil
}

func (s spaceStateShim) ConstraintsBySpaceName(name string) ([]space.Constraints, error) {
	constraints, err := s.ModelManagerBackend.ConstraintsBySpaceName(name)
	if err != nil {
		return nil, errors.Trace(err)
	}

	results := make([]space.Constraints, len(constraints))
	for i, constraint := range constraints {
		results[i] = constraint
	}
	return results, nil
}
