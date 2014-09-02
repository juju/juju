// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"

	"github.com/juju/names"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
)

// Watch starts a NotifyWatcher for the entity with the specified tag.
func Watch(facade base.FacadeCaller, tag names.Tag) (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag.String()}},
	}
	err := facade.FacadeCall("Watch", args, &results)
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
	return watcher.NewNotifyWatcher(facade.RawAPICaller(), result), nil
}
