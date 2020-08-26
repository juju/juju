// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build tools

package juju

import (
	// github.com/golang/mock/mockgen for generating mocks
	_ "github.com/golang/mock/mockgen"

	// github.com/hpidcock/juju-fake-init for testing new k8s
	_ "github.com/hpidcock/juju-fake-init"
)
