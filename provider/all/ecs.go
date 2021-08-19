// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build !minimal || provider_ecs
// +build !minimal provider_ecs

package all

import (
	// Register the provider.
	_ "github.com/juju/juju/caas/ecs"
)
