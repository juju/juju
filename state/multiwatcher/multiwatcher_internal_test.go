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
	_ EntityInfo = (*ServiceInfo)(nil)
	_ EntityInfo = (*UnitInfo)(nil)
	_ EntityInfo = (*RelationInfo)(nil)
	_ EntityInfo = (*AnnotationInfo)(nil)
	_ EntityInfo = (*BlockInfo)(nil)
	_ EntityInfo = (*ActionInfo)(nil)
	_ EntityInfo = (*EnvironmentInfo)(nil)
)

type ConstantsSuite struct{}

var _ = gc.Suite(&ConstantsSuite{})

func TestPackage(t *testing.T) {
	gc.TestingT(t)
}

func (s *ConstantsSuite) TestAnyJobNeedsState(c *gc.C) {
	c.Assert(AnyJobNeedsState(), jc.IsFalse)
	c.Assert(AnyJobNeedsState(JobHostUnits), jc.IsFalse)
	c.Assert(AnyJobNeedsState(JobManageNetworking), jc.IsFalse)
	c.Assert(AnyJobNeedsState(JobManageStateDeprecated), jc.IsFalse)
	c.Assert(AnyJobNeedsState(JobManageEnviron), jc.IsTrue)
	c.Assert(AnyJobNeedsState(JobHostUnits, JobManageEnviron), jc.IsTrue)
}
