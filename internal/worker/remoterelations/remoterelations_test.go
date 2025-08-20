// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

func TestRemoteRelationsSuite(t *stdtesting.T) {
	tc.Run(t, &remoteRelationsSuite{})
}

type remoteRelationsSuite struct {
	testhelpers.IsolationSuite
}
