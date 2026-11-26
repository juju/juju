// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package scriptlet

import (
	"testing"

	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
)

type scriptletSuite struct {
	coretesting.BaseSuite
}

func TestScriptletSuite(t *testing.T) {
	tc.Run(t, &scriptletSuite{})
}
