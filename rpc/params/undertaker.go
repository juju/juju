// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"

	"github.com/juju/juju/core/life"
)

// UndertakerModelInfo returns information on an model needed by the undertaker worker.
type UndertakerModelInfo struct {
	UUID           string         `json:"uuid"`
	Name           string         `json:"name"`
	IsSystem       bool           `json:"is-system"`
	Life           life.Value     `json:"life"`
	ForceDestroyed bool           `json:"force-destroyed,omitempty"`
	DestroyTimeout *time.Duration `json:"destroy-timeout,omitempty"`
}

// UndertakerModelInfoResult holds the result of an API call that returns an
// UndertakerModelInfoResult or an error.
type UndertakerModelInfoResult struct {
	Error  *Error              `json:"error,omitempty"`
	Result UndertakerModelInfo `json:"result"`
}
