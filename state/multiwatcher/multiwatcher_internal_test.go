// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package multiwatcher

import (
	"testing"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

var (
	_ EntityInfo = (*MachineInfo)(nil)
	_ EntityInfo = (*ApplicationInfo)(nil)
	_ EntityInfo = (*RemoteApplicationInfo)(nil)
	_ EntityInfo = (*ApplicationOfferInfo)(nil)
	_ EntityInfo = (*UnitInfo)(nil)
	_ EntityInfo = (*RelationInfo)(nil)
	_ EntityInfo = (*AnnotationInfo)(nil)
	_ EntityInfo = (*BlockInfo)(nil)
	_ EntityInfo = (*ActionInfo)(nil)
	_ EntityInfo = (*ModelInfo)(nil)
)

type ConstantsSuite struct{}

var _ = gc.Suite(&ConstantsSuite{})

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

func (s *ConstantsSuite) TestAnyJobNeedsState(c *gc.C) {
	c.Assert(AnyJobNeedsState(), jc.IsFalse)
	c.Assert(AnyJobNeedsState(JobHostUnits), jc.IsFalse)
	c.Assert(AnyJobNeedsState(JobManageModel), jc.IsTrue)
	c.Assert(AnyJobNeedsState(JobHostUnits, JobManageModel), jc.IsTrue)
}
