// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package handlers

import (
	"context"
)

// ConfigHandler amends model config as it is being saved and loaded from state.
// Only keys returned by RegisteredKeys are processed.
type ConfigHandler interface {
	// Name is the handler name.
	Name() string
	// RegisteredKeys returns the keys the handler will be asked to process.
	RegisteredKeys() []string
	// Validate returns an error if the config to be saved does not pass validation checks.
	// Any validation errors will satisfy config.ValidationError.
	Validate(ctx context.Context, cfg, old map[string]any) error
	// OnSave is run when saving model config; some attributes
	// from rawCfg might be removed as a result.
	OnSave(ctx context.Context, rawCfg map[string]any) error
	// OnLoad is run when loading model config. The result value is a map
	// containing any extra attributes to be added to the final config.
	OnLoad(ctx context.Context) (map[string]string, error)
}
