// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package quota_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/quota"
)

func TestBSONTotalSizeCheckerSuite(t *stdtesting.T) {
	tc.Run(t, &BSONTotalSizeCheckerSuite{})
}

type BSONTotalSizeCheckerSuite struct {
}

func (s *BSONTotalSizeCheckerSuite) TestSuccessfulCheck(c *tc.C) {
	chk := quota.NewBSONTotalSizeChecker(256)
	chk.Check(map[string]string{
		"a long key": "bar",
		"key":        "val",
	})
	chk.Check("some string")

	err := chk.Outcome()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *BSONTotalSizeCheckerSuite) TestExceedMaxSize(c *tc.C) {
	chk := quota.NewBSONTotalSizeChecker(24)
	chk.Check(map[string]string{
		"a long key": "bar",
		"key":        "val",
	})
	chk.Check("some string")

	err := chk.Outcome()
	c.Assert(err, tc.ErrorIs, coreerrors.QuotaLimitExceeded)
	c.Assert(err, tc.ErrorMatches, "max allowed size.*", tc.Commentf("expected error about exceeding max size"))
}

func (s *BSONTotalSizeCheckerSuite) TestQuotaBypass(c *tc.C) {
	chk := quota.NewBSONTotalSizeChecker(0)
	chk.Check(map[string]string{
		"a long key": "bar",
		"key":        "val",
	})
	chk.Check("some string")

	err := chk.Outcome()
	c.Assert(err, tc.ErrorIsNil)
}
