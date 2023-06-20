// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"time"

	"github.com/juju/version/v2"
)

type Info struct {
	PreviousVersion  version.Number
	TargetVersion    version.Number
	InitTime         time.Time
	StartTime        time.Time
	CompletionTime   time.Time
	ControllersReady []string
	ControllersDone  []string
}
