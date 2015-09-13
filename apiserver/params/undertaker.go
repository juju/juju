// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"
)

// UndertakerEnvironInfo returns information on an environment needed by the undertaker worker.
type UndertakerEnvironInfo struct {
	UUID        string
	Name        string
	GlobalName  string
	IsSystem    bool
	Life        Life
	TimeOfDeath *time.Time
}

// UndertakerEnvironInfoResult holds the result of an API call that returns an
// UndertakerEnvironInfoResult or an error.
type UndertakerEnvironInfoResult struct {
	Error  *Error
	Result UndertakerEnvironInfo
}
