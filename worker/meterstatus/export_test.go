// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus

import (
	"github.com/juju/clock"
	"github.com/juju/loggo/v2"
)

func NewLimitedContext(unitName string) *limitedContext {
	return newLimitedContext(hookConfig{
		unitName: unitName,
		clock:    clock.WallClock,
		logger:   loggo.GetLogger("test"),
	})
}
