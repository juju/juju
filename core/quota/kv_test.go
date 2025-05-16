// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package quota_test

import (
	stdtesting "testing"
	"time"

	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/quota"
)

func TestMapKeyValueCheckerSuite(t *stdtesting.T) { tc.Run(t, &MapKeyValueCheckerSuite{}) }

type MapKeyValueCheckerSuite struct {
}

func (s *MapKeyValueCheckerSuite) TestNonMapValue(c *tc.C) {
	chk := quota.NewMapKeyValueSizeChecker(24, 42)
	chk.Check("not-a-map")

	err := chk.Outcome()
	c.Assert(err, tc.ErrorIs, coreerrors.NotImplemented)
}

func (s *MapKeyValueCheckerSuite) TestMapWithMixedValueTypes(c *tc.C) {
	chk := quota.NewMapKeyValueSizeChecker(10, 30)
	chk.Check(map[string]interface{}{
		"key": map[string]string{
			"hello": "world",
		},
		"got-time?": time.Now(),
	})

	err := chk.Outcome()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *MapKeyValueCheckerSuite) TestMapWithStringKeyValues(c *tc.C) {
	chk := quota.NewMapKeyValueSizeChecker(5, 3)
	chk.Check(map[string]string{
		"key":  "val",
		"foof": "bar",
	})

	err := chk.Outcome()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *MapKeyValueCheckerSuite) TestQuotaBypass(c *tc.C) {
	chk := quota.NewMapKeyValueSizeChecker(0, 0)
	chk.Check(map[string]string{
		"key":  "val",
		"foof": "bar",
	})

	err := chk.Outcome()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *MapKeyValueCheckerSuite) TestExceedMaxKeySize(c *tc.C) {
	chk := quota.NewMapKeyValueSizeChecker(5, 3)
	chk.Check(map[string]string{
		"a long key": "bar",
		"key":        "val",
	})

	err := chk.Outcome()
	c.Assert(err, tc.ErrorIs, coreerrors.QuotaLimitExceeded)
	c.Assert(err, tc.ErrorMatches, "max allowed key length.*", tc.Commentf("expected error about exceeding max key length"))
}

func (s *MapKeyValueCheckerSuite) TestExceedMaxValueSize(c *tc.C) {
	chk := quota.NewMapKeyValueSizeChecker(5, 3)
	chk.Check(map[string]string{
		"key1": "val",
		"key2": "a long value",
	})

	err := chk.Outcome()
	c.Assert(err, tc.ErrorIs, coreerrors.QuotaLimitExceeded)
	c.Assert(err, tc.ErrorMatches, "max allowed value length.*", tc.Commentf("expected error about exceeding max value length"))
}
