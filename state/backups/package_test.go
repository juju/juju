// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	stdtesting "testing"

	"github.com/juju/juju/v3/core/os"
	"github.com/juju/juju/v3/testing"
)

func Test(t *stdtesting.T) {
	if os.HostOS() == os.Ubuntu {
		testing.MgoTestPackage(t)
	}
}
