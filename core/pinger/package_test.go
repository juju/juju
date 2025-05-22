// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pinger_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package pinger -destination clock_mock_test.go github.com/juju/clock Clock,Timer


type ImportTest struct{}

func TestImportTest(t *stdtesting.T) { tc.Run(t, &ImportTest{}) }
func (*ImportTest) TestImports(c *tc.C) {
	found := coretesting.FindJujuCoreImports(c, "github.com/juju/juju/core/pinger")

	// This package brings in nothing else from juju/juju
	c.Assert(found, tc.SameContents, []string{
		"internal/errors",
	})
}
