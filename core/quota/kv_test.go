// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package quota_test

import (
	"time"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/quota"
)

var _ = gc.Suite(&MapKeyValueCheckerSuite{})

type MapKeyValueCheckerSuite struct {
}

func (s *MapKeyValueCheckerSuite) TestNonMapValue(c *gc.C) {
	chk := quota.NewMapKeyValueSizeChecker(24, 42)
	chk.Check("not-a-map")

	err := chk.Outcome()
	c.Assert(err, jc.Satisfies, errors.IsNotImplemented)
}

func (s *MapKeyValueCheckerSuite) TestMapWithMixedValueTypes(c *gc.C) {
	chk := quota.NewMapKeyValueSizeChecker(10, 30)
	chk.Check(map[string]interface{}{
		"key": map[string]string{
			"hello": "world",
		},
		"got-time?": time.Now(),
	})

	err := chk.Outcome()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MapKeyValueCheckerSuite) TestMapWithStringKeyValues(c *gc.C) {
	chk := quota.NewMapKeyValueSizeChecker(5, 3)
	chk.Check(map[string]string{
		"key":  "val",
		"foof": "bar",
	})

	err := chk.Outcome()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MapKeyValueCheckerSuite) TestQuotaBypass(c *gc.C) {
	chk := quota.NewMapKeyValueSizeChecker(0, 0)
	chk.Check(map[string]string{
		"key":  "val",
		"foof": "bar",
	})

	err := chk.Outcome()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *MapKeyValueCheckerSuite) TestExceedMaxKeySize(c *gc.C) {
	chk := quota.NewMapKeyValueSizeChecker(5, 3)
	chk.Check(map[string]string{
		"a long key": "bar",
		"key":        "val",
	})

	err := chk.Outcome()
	c.Assert(err, jc.Satisfies, errors.IsQuotaLimitExceeded)
	c.Assert(err, gc.ErrorMatches, "max allowed key length.*", gc.Commentf("expected error about exceeding max key length"))
}

func (s *MapKeyValueCheckerSuite) TestExceedMaxValueSize(c *gc.C) {
	chk := quota.NewMapKeyValueSizeChecker(5, 3)
	chk.Check(map[string]string{
		"key1": "val",
		"key2": "a long value",
	})

	err := chk.Outcome()
	c.Assert(err, jc.Satisfies, errors.IsQuotaLimitExceeded)
	c.Assert(err, gc.ErrorMatches, "max allowed value length.*", gc.Commentf("expected error about exceeding max value length"))
}
