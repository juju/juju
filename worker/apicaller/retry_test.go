// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apicaller_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

// RetryStrategySuite exercises the cases where we need to connect
// repeatedly, either to wait for provisioning errors or to fall
// back to other possible passwords. It covers OnlyConnect in detail,
// checking success and failure behaviour, but only checks suitable
// error paths for ScaryConnect (which does extra complex things like
// make api calls and rewrite passwords in config).
//
// Would be best of all to test all the ScaryConnect success/failure
// paths explicitly, but the combinatorial explosion makes it tricky;
// in the absence of a further decomposition of responsibilities, it
// seems best to at least decompose the testing. Which is more detailed
// than it was before, anyway.
type RetryStrategySuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&RetryStrategySuite{})
