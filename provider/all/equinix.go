// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build !minimal || provider_equinix
// +build !minimal provider_equinix

package all

import (
	// Register the provider.
	_ "github.com/juju/juju/v2/provider/equinix"
)
