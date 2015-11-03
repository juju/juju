// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package all

// Register all the available providers that require Go 1.3
import (
	_ "github.com/juju/juju/provider/lxd"
)
