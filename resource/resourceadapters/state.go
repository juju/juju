// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"github.com/juju/errors"
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

// DataStore implements functionality wrapping state for resources.
type DataStore struct {
	state.Resources
	State *state.State
}

// Units returns the tags for all units in the service.
func (d DataStore) Units(serviceID string) (tags []names.UnitTag, err error) {
	svc, err := d.State.Service(serviceID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	units, err := svc.AllUnits()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for _, u := range units {
		tags = append(tags, u.UnitTag())
	}
	return tags, nil
}
