// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// UndertakerModelInfo returns information on an model needed by the undertaker worker.
type UndertakerModelInfo struct {
	UUID       string `json:"uuid"`
	Name       string `json:"name"`
	GlobalName string `json:"global-name"`
	IsSystem   bool   `json:"is-system"`
	Life       Life   `json:"life"`
}

// UndertakerModelInfoResult holds the result of an API call that returns an
// UndertakerModelInfoResult or an error.
type UndertakerModelInfoResult struct {
	Error  *Error              `json:"error,omitempty"`
	Result UndertakerModelInfo `json:"result"`
}
