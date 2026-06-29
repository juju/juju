// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelimport

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/export"
)

type modelimportSuite struct{}

func TestModelimport(t *testing.T) {
	tc.Run(t, &modelimportSuite{})
}

func (s *modelimportSuite) TestNewTransformerTargetsLatestSupportedPayloadVersion(c *tc.C) {
	tr, err := NewTransformer()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(tr.Target(), tc.Equals, export.LatestSupportedPayloadVersion())
}
