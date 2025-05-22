// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	"os"
	"testing"

	"github.com/juju/juju/internal/testhelpers"
)

func TestMain(m *testing.M) {
	testhelpers.ExecHelperProcess()
	os.Exit(m.Run())
}
