// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"
)

// ReaperEnvironInfo returns information on an environment needed by the undertaker worker.
type ReaperEnvironInfo struct {
	UUID        string
	Name        string
	GlobalName  string
	IsSystem    bool
	Life        Life
	TimeOfDeath *time.Time
}

// ReaperEnvironInfoResult holds the result of an API call that returns an
// ReaperEnvironInfoResult or an error.
type ReaperEnvironInfoResult struct {
	Error  *Error
	Result ReaperEnvironInfo
}
