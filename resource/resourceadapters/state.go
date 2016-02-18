// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"github.com/juju/names"
	"gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/state"
)

type service struct {
	*state.Service
}

func (s *service) ID() names.ServiceTag {
	return names.NewServiceTag(s.Name())
}

// CharmURL implements resource/workers.Service.
func (s *service) CharmURL() *charm.URL {
	cURL, _ := s.Service.CharmURL()
	return cURL
}
