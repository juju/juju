// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"fmt"

	"github.com/juju/names"

	"github.com/juju/juju/state/api/common"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/api/watcher"
)

// Service represents the state of a service.
type Service struct {
	st   *State
	tag  names.ServiceTag
	life params.Life
}

// Name returns the service name.
func (s *Service) Name() string {
	return s.tag.Id()
}

// Watch returns a watcher for observing changes to a service.
func (s *Service) Watch() (watcher.NotifyWatcher, error) {
	return common.Watch(s.st.caller, firewallerFacade, s.tag)
}

// Life returns the service's current life state.
func (s *Service) Life() params.Life {
	return s.life
}

// Refresh refreshes the contents of the Service from the underlying
// state.
func (s *Service) Refresh() error {
	life, err := s.st.life(s.tag)
	if err != nil {
		return err
	}
	s.life = life
	return nil
}

// IsExposed returns whether this service is exposed. The explicitly
// open ports (with open-port) for exposed services may be accessed
// from machines outside of the local deployment network.
//
// NOTE: This differs from state.Service.IsExposed() by returning
// an error as well, because it needs to make an API call.
func (s *Service) IsExposed() (bool, error) {
	var results params.BoolResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: s.tag.String()}},
	}
	err := s.st.call("GetExposed", args, &results)
	if err != nil {
		return false, err
	}
	if len(results.Results) != 1 {
		return false, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return false, result.Error
	}
	return result.Result, nil
}
