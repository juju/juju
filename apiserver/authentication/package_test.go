// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication_test

import (
	"testing"

	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&AgentAuthenticatorSuite{})

func TestAll(t *testing.T) {
	coretesting.MgoTestPackage(t)
}
