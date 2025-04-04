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
	return false, errors.NotImplementedf("cross model relations are disabled until " +
		"backend functionality is moved to domain")
}
