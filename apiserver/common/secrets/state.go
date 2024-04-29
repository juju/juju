// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"context"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

// ModelConfig defines a subset of state model methods
// for getting config.
type ModelConfig interface {
	ModelConfig(context.Context) (*config.Config, error)
	WatchForModelConfigChanges() state.NotifyWatcher
}
