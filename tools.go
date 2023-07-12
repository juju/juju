// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build tools

package juju

import (
	// go.uber.org/mock/mockgen for generating mocks
	_ "go.uber.org/mock/mockgen"

	// github.com/canonical/pebble/cmd/pebble for pebble binary generation
	_ "github.com/canonical/pebble/cmd/pebble"
)
