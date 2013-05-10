// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store

import (
	"time"
)

func TimeToStamp(t time.Time) int32 {
	return timeToStamp(t)
}
