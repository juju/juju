// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmanager

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs/space"
	"github.com/juju/juju/state"
)

type spaceStateShim struct {
	common.ModelManagerBackend
}

func (s spaceStateShim) AllSpaces() ([]network.SpaceInfo, error) {
	spaces, err := s.ModelManagerBackend.AllSpaces()
	if err != nil {
		return nil, errors.Trace(err)
	}

	results := make([]network.SpaceInfo, len(spaces))
	for i, space := range spaces {
		spaceInfo, err := space.NetworkSpace()
		if err != nil {
			return nil, errors.Trace(err)
		}
		results[i] = spaceInfo
	}
	return results, nil
}

func (s spaceStateShim) AddSpace(name string, providerId network.Id, subnetIds []string, public bool) (network.SpaceInfo, error) {
	result, err := s.ModelManagerBackend.AddSpace(name, providerId, subnetIds, public)
	if err != nil {
		return network.SpaceInfo{}, errors.Trace(err)
	}
	spaceInfo, err := result.NetworkSpace()
	if err != nil {
		return network.SpaceInfo{}, errors.Trace(err)
	}
	return spaceInfo, nil
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

// TODO(nvinuesa): This method is needed temporarily and must be removed when
// the new spaces domain has been finished and the mongodb state removed.
func (s spaceStateShim) Life(spaceID string) (state.Life, error) {
	space, err := s.ModelManagerBackend.Space(spaceID)
	if err != nil {
		return state.Dead, errors.Trace(err)
	}
	return space.Life(), nil
}

// TODO(nvinuesa): This method is needed temporarily and must be removed when
// the new spaces domain has been finished and the mongodb state removed.
func (s spaceStateShim) EnsureDead(spaceID string) error {
	space, err := s.ModelManagerBackend.Space(spaceID)
	if err != nil {
		return errors.Trace(err)
	}
	return space.EnsureDead()
}

// TODO(nvinuesa): This method is needed temporarily and must be removed when
// the new spaces domain has been finished and the mongodb state removed.
func (s spaceStateShim) Remove(spaceID string) error {
	space, err := s.ModelManagerBackend.Space(spaceID)
	if err != nil {
		return errors.Trace(err)
	}
	return space.Remove()
}

type credentialStateShim struct {
	StateBackend
}

func (s credentialStateShim) CloudCredentialTag() (names.CloudCredentialTag, bool, error) {
	m, err := s.StateBackend.Model()
	if err != nil {
		return names.CloudCredentialTag{}, false, errors.Trace(err)
	}
	credTag, exists := m.CloudCredentialTag()
	return credTag, exists, nil
}
