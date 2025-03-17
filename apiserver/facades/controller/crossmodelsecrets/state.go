// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelsecrets

import (
	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/state"
)

// The following interfaces are used to access backend state.

type CrossModelState interface {
	GetRemoteApplicationTag(string) (names.Tag, error)
	GetToken(entity names.Tag) (string, error)
}

type StateBackend interface {
	HasEndpoint(key string, app string) (bool, error)
}

type stateBackendShim struct {
	*state.State
}

func (s *stateBackendShim) HasEndpoint(key string, app string) (bool, error) {
	rel, err := s.State.KeyRelation(key)
	if err != nil {
		return false, errors.Trace(err)
	}
	if rel.Suspended() {
		return false, nil
	}
	_, err = rel.Endpoint(app)
	return err == nil, nil
}
