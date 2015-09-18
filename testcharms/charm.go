// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package testcharms holds a corpus of charms
// for testing.
package testcharms

import (
	"gopkg.in/juju/charm.v5/testing"
)

// Repo provides access to the test charm repository.
var Repo = testing.NewRepo("charm-repo", "quantal")
