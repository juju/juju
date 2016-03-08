// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"
)

// UndertakerModelInfo returns information on an model needed by the undertaker worker.
type UndertakerModelInfo struct {
	UUID        string
	Name        string
	GlobalName  string
	IsSystem    bool
	Life        Life
	TimeOfDeath *time.Time
}

// UndertakerModelInfoResult holds the result of an API call that returns an
// UndertakerModelInfoResult or an error.
type UndertakerModelInfoResult struct {
	Error  *Error
	Result UndertakerModelInfo
}
