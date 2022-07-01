// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package quota_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v2/core/quota"
)

var _ = gc.Suite(&MultiCheckerSuite{})

type MultiCheckerSuite struct {
}

func (s *MultiCheckerSuite) TestSuccessfulCheck(c *gc.C) {
	chk := quota.NewMultiChecker(
		quota.NewMapKeyValueSizeChecker(5, 10),
		quota.NewBSONTotalSizeChecker(50),
	)
	chk.Check(map[string]string{
		"key0": "0123456789",
		"key":  "0123456789",
	})

	err := chk.Outcome()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MultiCheckerSuite) TestExceedMaxSize(c *gc.C) {
	chk := quota.NewMultiChecker(
		quota.NewMapKeyValueSizeChecker(5, 10),
		quota.NewBSONTotalSizeChecker(24),
	)
	chk.Check(map[string]string{
		"key0": "0123456789",
		"key":  "0123456789",
		"moar": "data", // passes key/value check but fails total size check
	})

	err := chk.Outcome()
	c.Assert(err, jc.Satisfies, errors.IsQuotaLimitExceeded)
}
