// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type rulesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&rulesSuite{})
