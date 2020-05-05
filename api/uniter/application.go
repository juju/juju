// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"fmt"

	"github.com/juju/charm/v7"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
)

// This module implements a subset of the interface provided by
// state.Application, as needed by the uniter API.

// Application represents the state of an application.
type Application struct {
	st   *State
	tag  names.ApplicationTag
	life life.Value
}

// Tag returns the application's tag.
func (s *Application) Tag() names.ApplicationTag {
	return s.tag
}

// Name returns the application name.
func (s *Application) Name() string {
	return s.tag.Id()
}

// String returns the application as a string.
func (s *Application) String() string {
	return s.Name()
}

// Watch returns a watcher for observing changes to an application.
func (s *Application) Watch() (watcher.NotifyWatcher, error) {
	return common.Watch(s.st.facade, "Watch", s.tag)
}

// Life returns the application's current life state.
func (s *Application) Life() life.Value {
	return s.life
}

// Refresh refreshes the contents of the application from the underlying
// state.
func (s *Application) Refresh() error {
	life, err := s.st.life(s.tag)
	if err != nil {
		return err
	}
	s.life = life
	return nil
}

// CharmModifiedVersion increments every time the charm, or any part of it, is
// changed in some way.
func (s *Application) CharmModifiedVersion() (int, error) {
	var results params.IntResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: s.tag.String()}},
	}
	err := s.st.facade.FacadeCall("CharmModifiedVersion", args, &results)
	if err != nil {
		return -1, err
	}

	if len(results.Results) != 1 {
		return -1, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return -1, result.Error
	}

	return result.Result, nil
}

// CharmURL returns the application's charm URL, and whether units should
// upgrade to the charm with that URL even if they are in an error
// state (force flag).
//
// NOTE: This differs from state.Application.CharmURL() by returning
// an error instead as well, because it needs to make an API call.
func (s *Application) CharmURL() (*charm.URL, bool, error) {
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

// SetStatus sets the status of the application if the passed unitName,
// corresponding to the calling unit, is of the leader.
func (s *Application) SetStatus(unitName string, appStatus status.Status, info string, data map[string]interface{}) error {
	tag := names.NewUnitTag(unitName)
	var result params.ErrorResults
	args := params.SetStatus{
		Entities: []params.EntityStatusArgs{
			{
				Tag:    tag.String(),
				Status: appStatus.String(),
				Info:   info,
				Data:   data,
			},
		},
	}
	err := s.st.facade.FacadeCall("SetApplicationStatus", args, &result)
	if err != nil {
		return errors.Trace(err)
	}
	return result.OneError()
}

// Status returns the status of the application if the passed unitName,
// corresponding to the calling unit, is of the leader.
func (s *Application) Status(unitName string) (params.ApplicationStatusResult, error) {
	tag := names.NewUnitTag(unitName)
	var results params.ApplicationStatusResults
	args := params.Entities{
		Entities: []params.Entity{
			{
				Tag: tag.String(),
			},
		},
	}
	err := s.st.facade.FacadeCall("ApplicationStatus", args, &results)
	if err != nil {
		return params.ApplicationStatusResult{}, errors.Trace(err)
	}
	result := results.Results[0]
	if result.Error != nil {
		return params.ApplicationStatusResult{}, result.Error
	}
	return result, nil
}

// WatchLeadershipSettings returns a watcher which can be used to wait
// for leadership settings changes to be made for the application.
func (s *Application) WatchLeadershipSettings() (watcher.NotifyWatcher, error) {
	return s.st.LeadershipSettings.WatchLeadershipSettings(s.tag.Id())
}
