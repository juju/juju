// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package introspect_test

import (
	"runtime"
	"testing"

	"github.com/juju/tc"
)

func TestPackage(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("introspection socket only runs on Linux")
	}
	tc.TestingT(t)
}
