// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build !minimal || provider_ec2
// +build !minimal provider_ec2

package all

import (
	// Register the provider.
	_ "github.com/juju/juju/v3/provider/ec2"
)
