// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	stdtesting "testing"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

func Test(t *stdtesting.T) {
	if version.Current.OS != version.Ubuntu {
		t.Skip("backups are a state-server-only feature, hence Ubuntu-only")
	}
	testing.MgoTestPackage(t)
}
