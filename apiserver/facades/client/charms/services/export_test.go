// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package services

import (
	"github.com/juju/utils/v3"
)

func (s *CharmStorage) SetUUIDGenerator(f func() (utils.UUID, error)) {
	s.uuidGenerator = f
}
