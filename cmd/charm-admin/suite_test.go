// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"testing"

	gitjujutesting "github.com/juju/testing"
)

func Test(t *testing.T) {
	gitjujutesting.MgoTestPackage(t, nil)
}
