// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/gce"
	"github.com/juju/juju/provider/gce/google"
)

type environUpgradeSuite struct {
	gce.BaseSuite
}

var _ = gc.Suite(&environUpgradeSuite{})

func (s *environUpgradeSuite) TestEnvironImplementsUpgrader(c *gc.C) {
	c.Assert(s.Env, gc.Implements, new(environs.Upgrader))
}

func (s *environUpgradeSuite) TestEnvironUpgradeOperations(c *gc.C) {
	ops := s.Env.UpgradeOperations(s.CallCtx, environs.UpgradeOperationsParams{})
	c.Assert(ops, gc.HasLen, 1)
	c.Assert(ops[0].TargetVersion, gc.Equals, 1)
	c.Assert(ops[0].Steps, gc.HasLen, 1)
	c.Assert(ops[0].Steps[0].Description(), gc.Equals, "Set disk labels")
}

func (s *environUpgradeSuite) TestEnvironUpgradeOperationSetDiskLabels(c *gc.C) {
	delete(s.BaseDisk.Labels, "juju-model-uuid")
	delete(s.BaseDisk.Labels, "juju-controller-uuid")
	s.FakeConn.GoogleDisks = []*google.Disk{s.BaseDisk}

	op0 := s.Env.UpgradeOperations(s.CallCtx, environs.UpgradeOperationsParams{
		ControllerUUID: "yup",
	})[0]
	c.Assert(op0.Steps[0].Run(s.CallCtx), jc.ErrorIsNil)

	setDiskLabelsCalled, calls := s.FakeConn.WasCalled("SetDiskLabels")
	c.Assert(setDiskLabelsCalled, jc.IsTrue)
	c.Check(calls, gc.HasLen, 1)
	c.Check(calls[0].ID, gc.Equals, s.BaseDisk.Name)
	c.Check(calls[0].ZoneName, gc.Equals, "home-zone")
	c.Check(calls[0].LabelFingerprint, gc.Equals, "foo")
	c.Check(calls[0].Labels, jc.DeepEquals, map[string]string{
		"juju-controller-uuid": "yup",
		"juju-model-uuid":      s.Env.Config().UUID(),
		"yodel":                "eh",
	})
}

func (s *environUpgradeSuite) TestEnvironUpgradeOperationSetDiskLabelsNoDescription(c *gc.C) {
	delete(s.BaseDisk.Labels, "juju-model-uuid")
	delete(s.BaseDisk.Labels, "juju-controller-uuid")
	s.BaseDisk.Description = ""
	s.FakeConn.GoogleDisks = []*google.Disk{s.BaseDisk}

	op0 := s.Env.UpgradeOperations(s.CallCtx, environs.UpgradeOperationsParams{
		ControllerUUID: "yup",
	})[0]
	c.Assert(op0.Steps[0].Run(s.CallCtx), jc.ErrorIsNil)

	setDiskLabelsCalled, _ := s.FakeConn.WasCalled("SetDiskLabels")
	c.Assert(setDiskLabelsCalled, jc.IsTrue)
}

func (s *environUpgradeSuite) TestEnvironUpgradeOperationSetDiskLabelsIdempotent(c *gc.C) {
	// s.BaseDisk is already labelled appropriately,
	// so we should not see a call to SetDiskLabels.
	s.FakeConn.GoogleDisks = []*google.Disk{s.BaseDisk}

	op0 := s.Env.UpgradeOperations(s.CallCtx, environs.UpgradeOperationsParams{
		ControllerUUID: "yup",
	})[0]
	c.Assert(op0.Steps[0].Run(s.CallCtx), jc.ErrorIsNil)

	setDiskLabelsCalled, _ := s.FakeConn.WasCalled("SetDiskLabels")
	c.Assert(setDiskLabelsCalled, jc.IsFalse)
}
