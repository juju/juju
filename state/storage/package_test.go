// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	"testing"

	gitjujutesting "github.com/juju/testing"
)

func Test(t *testing.T) {
	gitjujutesting.MgoTestPackage(t, nil)
}
