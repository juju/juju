// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"

	"github.com/juju/names"
	"gopkg.in/juju/charm.v4"

	"github.com/juju/juju/api/common"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
)

// This module implements a subset of the interface provided by
// state.Service, as needed by the uniter API.

// Service represents the state of a service.
type Service struct {
	st   *State
	tag  names.ServiceTag
	life params.Life
}

// Tag returns the service's tag.
func (s *Service) Tag() names.ServiceTag {
	return s.tag
}

// Name returns the service name.
func (s *Service) Name() string {
	return s.tag.Id()
}

// String returns the service as a string.
func (s *Service) String() string {
	return s.Name()
}

// Watch returns a watcher for observing changes to a service.
func (s *Service) Watch() (watcher.NotifyWatcher, error) {
	return common.Watch(s.st.facade, s.tag)
}

// WatchRelations returns a StringsWatcher that notifies of changes to
// the lifecycles of relations involving s.
func (s *Service) WatchRelations() (watcher.StringsWatcher, error) {
	var results params.StringsWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: s.tag.String()}},
	}
	err := s.st.facade.FacadeCall("WatchServiceRelations", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := watcher.NewStringsWatcher(s.st.facade.RawAPICaller(), result)
	return w, nil
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

// CharmURL returns the service's charm URL, and whether units should
// upgrade to the charm with that URL even if they are in an error
// state (force flag).
//
// NOTE: This differs from state.Service.CharmURL() by returning
// an error instead as well, because it needs to make an API call.
func (s *Service) CharmURL() (*charm.URL, bool, error) {
	var results params.StringBoolResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: s.tag.String()}},
	}
	err := s.st.facade.FacadeCall("CharmURL", args, &results)
	if err != nil {
		return nil, false, err
	}
	if len(results.Results) != 1 {
		return nil, false, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, false, result.Error
	}
	if result.Result != "" {
		curl, err := charm.ParseURL(result.Result)
		if err != nil {
			return nil, false, err
		}
		return curl, result.Ok, nil
	}
	return nil, false, fmt.Errorf("%q has no charm url set", s.tag)
}

// OwnerTag returns the service's owner user tag.
func (s *Service) OwnerTag() (names.UserTag, error) {
	if s.st.BestAPIVersion() > 0 {
		return s.serviceOwnerTag()
	}
	return s.ownerTag()
}

func (s *Service) serviceOwnerTag() (names.UserTag, error) {
	var invalidTag names.UserTag
	var results params.StringResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: s.tag.String()}},
	}
	err := s.st.facade.FacadeCall("ServiceOwner", args, &results)
	if err != nil {
		return invalidTag, err
	}
	if len(results.Results) != 1 {
		return invalidTag, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return invalidTag, result.Error
	}
	return names.ParseUserTag(result.Result)
}

func (s *Service) ownerTag() (names.UserTag, error) {
	var invalidTag names.UserTag
	var result params.StringResult
	args := params.Entities{
		Entities: []params.Entity{{Tag: s.tag.String()}},
	}
	err := s.st.facade.FacadeCall("GetOwnerTag", args, &result)
	if err != nil {
		return invalidTag, err
	}
	if result.Error != nil {
		return invalidTag, result.Error
	}
	return names.ParseUserTag(result.Result)
}
