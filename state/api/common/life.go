// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"

	"launchpad.net/juju-core/state/api/base"
	"launchpad.net/juju-core/state/api/params"
)

// Life requests the life cycle of the given entity from the given
// server-side API facade via the given caller.
func Life(caller base.Caller, facadeName, tag string) (params.Life, error) {
	var result params.LifeResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag}},
	}
	err := caller.Call(facadeName, "", "Life", args, &result)
	if err != nil {
		return "", err
	}
	if len(result.Results) != 1 {
		return "", fmt.Errorf("expected 1 result, got %d", len(result.Results))
	}
	if err := result.Results[0].Error; err != nil {
		return "", err
	}
	return result.Results[0].Life, nil
}
