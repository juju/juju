// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"

	"github.com/juju/names"

	"github.com/juju/juju/state/api/base"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/api/watcher"
)

// Watch starts a NotifyWatcher for the entity with the specified tag.
func Watch(caller base.Caller, facadeName string, tag names.Tag) (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag.String()}},
	}
	err := caller.Call(facadeName, "", "Watch", args, &results)
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
	return watcher.NewNotifyWatcher(caller, result), nil
}
