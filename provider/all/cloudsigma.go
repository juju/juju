// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build !minimal || provider_cloudsigma
// +build !minimal provider_cloudsigma

package all

import (
	// Register the provider.
	_ "github.com/juju/juju/provider/cloudsigma"
)
