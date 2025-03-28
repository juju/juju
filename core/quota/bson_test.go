// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package quota_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/quota"
)

var _ = gc.Suite(&BSONTotalSizeCheckerSuite{})

type BSONTotalSizeCheckerSuite struct {
}

func (s *BSONTotalSizeCheckerSuite) TestSuccessfulCheck(c *gc.C) {
	chk := quota.NewBSONTotalSizeChecker(256)
	chk.Check(map[string]string{
		"a long key": "bar",
		"key":        "val",
	})
	chk.Check("some string")

	err := chk.Outcome()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *BSONTotalSizeCheckerSuite) TestExceedMaxSize(c *gc.C) {
	chk := quota.NewBSONTotalSizeChecker(24)
	chk.Check(map[string]string{
		"a long key": "bar",
		"key":        "val",
	})
	chk.Check("some string")

	err := chk.Outcome()
	c.Assert(err, jc.ErrorIs, coreerrors.QuotaLimitExceeded)
	c.Assert(err, gc.ErrorMatches, "max allowed size.*", gc.Commentf("expected error about exceeding max size"))
}

func (s *BSONTotalSizeCheckerSuite) TestQuotaBypass(c *gc.C) {
	chk := quota.NewBSONTotalSizeChecker(0)
	chk.Check(map[string]string{
		"a long key": "bar",
		"key":        "val",
	})
	chk.Check("some string")

	err := chk.Outcome()
	c.Assert(err, jc.ErrorIsNil)
}
