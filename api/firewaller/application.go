// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/common"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
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

// ExposeInfo returns a flag to indicate whether an application is exposed
// as well as any endpoint-specific expose settings (if present).
func (s *Application) ExposeInfo() (bool, map[string]params.ExposedEndpoint, error) {
	if s.st.BestAPIVersion() < 6 {
		// ExposeInfo() was introduced in FirewallerAPIV6.
		return false, nil, errors.NotImplementedf("ExposeInfo() (need V6+)")
	}

	var results params.ExposeInfoResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: s.tag.String()}},
	}
	err := s.st.facade.FacadeCall("GetExposeInfo", args, &results)
	if err != nil {
		return false, nil, err
	}
	if len(results.Results) != 1 {
		return false, nil, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		if params.IsCodeNotFound(result.Error) {
			return false, nil, errors.NewNotFound(result.Error, "")
		}
		return false, nil, result.Error
	}
	return result.Exposed, result.ExposedEndpoints, nil
}
