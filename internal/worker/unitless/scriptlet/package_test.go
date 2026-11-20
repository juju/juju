// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package scriptlet

import (
	"testing"

	"github.com/juju/tc"
)

type scriptletSuite struct{}

func TestScriptletSuite(t *testing.T) {
	tc.Run(t, &scriptletSuite{})
}
