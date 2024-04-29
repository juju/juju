// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsdrain

import (
	"context"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

// ModelConfig provides the subset of state.Model that is required by the secrets drain apis.
type ModelConfig interface {
	ModelConfig(context.Context) (*config.Config, error)
	WatchForModelConfigChanges() state.NotifyWatcher
}
