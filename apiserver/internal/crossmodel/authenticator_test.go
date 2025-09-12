// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodel

import (
	"testing"

	"github.com/juju/tc"
)

type authenticatorSuite struct{}

func TestAuthenticatorSuite(t *testing.T) {
	tc.Run(t, &authenticatorSuite{})
}
