// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build tools

package juju

import (
	// github.com/canonical/pebble/cmd/pebble for pebble binary generation
	_ "github.com/canonical/pebble/cmd/pebble"
	// github.com/canonical/gomock/mockgen for generating mocks
	_ "github.com/canonical/gomock/mockgen"
)
