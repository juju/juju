// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package handlers

import (
	"context"
)

// ConfigHandler amends model config as it is being saved and loaded from state.
type ConfigHandler interface {
	// Name is the handler name.
	Name() string
	// OnSave is run when saving model config; some attributes
	// from rawCfg might be removed as a result.
	OnSave(ctx context.Context, rawCfg map[string]any, removeAttrs []string) error
	// OnLoad is run when loading model config. The result value is a map
	// containing any extra attributes to be added to the final config.
	OnLoad(ctx context.Context) (map[string]string, error)
}
