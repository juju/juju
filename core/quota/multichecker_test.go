// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package quota_test

import (
	"testing"

	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/quota"
)

func TestMultiCheckerSuite(t *testing.T) {
	tc.Run(t, &MultiCheckerSuite{})
}

type MultiCheckerSuite struct {
}

func (s *MultiCheckerSuite) TestSuccessfulCheck(c *tc.C) {
	chk := quota.NewMultiChecker(
		quota.NewMapKeyValueSizeChecker(5, 10),
		newStringMapTotalSizeChecker(50),
	)
	chk.Check(map[string]string{
		"key0": "0123456789",
		"key":  "0123456789",
	})

	err := chk.Outcome()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *MultiCheckerSuite) TestExceedMaxSize(c *tc.C) {
	chk := quota.NewMultiChecker(
		quota.NewMapKeyValueSizeChecker(5, 10),
		newStringMapTotalSizeChecker(30),
	)
	chk.Check(map[string]string{
		"key0": "0123456789",
		"key":  "0123456789",
		"moar": "data", // passes key/value check but fails total size check
	})

	err := chk.Outcome()
	c.Assert(err, tc.ErrorIs, coreerrors.QuotaLimitExceeded)
}

type stringMapTotalSizeChecker struct {
	maxSize int
	lastErr error
}

func newStringMapTotalSizeChecker(maxSize int) *stringMapTotalSizeChecker {
	return &stringMapTotalSizeChecker{
		maxSize: maxSize,
	}
}

func (c *stringMapTotalSizeChecker) Check(v any) {
	settings, ok := v.(map[string]string)
	if !ok {
		return
	}
	c.lastErr = quota.CheckStringMapTotalSize(settings, c.maxSize)
}

func (c *stringMapTotalSizeChecker) Outcome() error {
	return c.lastErr
}
