// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package syslog_test

import (
	"runtime"
	"testing"

	gc "gopkg.in/check.v1"
)

func TestPackage(t *testing.T) {
	//TODO(bogdanteleaga): Fix this on windows
	if runtime.GOOS == "windows" {
		t.Skip("bug 1403084: Skipping rsyslog tests on windows")
	}
	gc.TestingT(t)
}
