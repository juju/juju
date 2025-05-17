// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package factory_test

import (
	"os"
	stdtesting "testing"

	"github.com/juju/juju/internal/testing"
)

// TestPackage integrates the tests into gotest.
func TestMain(m *stdtesting.M) {
	os.Exit(func() int {
		defer testing.MgoTestMain()()
		return m.Run()
	}())
}
