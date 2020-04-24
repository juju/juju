// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher"
)

// Application represents the state of an application.
type Application struct {
	st   *Client
	tag  names.ApplicationTag
	life life.Value
}

// Name returns the application name.
func (s *Application) Name() string {
	return s.tag.Id()
}

// Tag returns the application tag.
func (s *Application) Tag() names.ApplicationTag {
	return s.tag
}

// Watch returns a watcher for observing changes to an application.
func (s *Application) Watch() (watcher.NotifyWatcher, error) {
	return common.Watch(s.st.facade, "Watch", s.tag)
}

// IsExposed returns whether this application is exposed. The explicitly
// open ports (with open-port) for exposed application may be accessed
// from machines outside of the local deployment network.
//
// NOTE: This differs from state.Application.IsExposed() by returning
// an error as well, because it needs to make an API call.
func (s *Application) IsExposed() (bool, error) {
	var results params.BoolResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: s.tag.String()}},
	}
	err := s.st.facade.FacadeCall("GetExposed", args, &results)
	if err != nil {
		return false, err
	}
	if len(results.Results) != 1 {
		return false, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		if params.IsCodeNotFound(result.Error) {
			return false, errors.NewNotFound(result.Error, "")
		}
		return false, result.Error
	}
	return result.Result, nil
}
