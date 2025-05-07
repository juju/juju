// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"context"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/provider/gce"
	"github.com/juju/juju/internal/provider/gce/google"
)

type environUpgradeSuite struct {
	gce.BaseSuite
}

var _ = tc.Suite(&environUpgradeSuite{})

func (s *environUpgradeSuite) TestEnvironImplementsUpgrader(c *tc.C) {
	c.Assert(s.Env, tc.Implements, new(environs.Upgrader))
}

func (s *environUpgradeSuite) TestEnvironUpgradeOperationsInvalidCredentialError(c *tc.C) {
	s.FakeConn.Err = gce.InvalidCredentialError
	c.Assert(s.InvalidatedCredentials, jc.IsFalse)
	ops := s.Env.UpgradeOperations(context.Background(), environs.UpgradeOperationsParams{})
	err := ops[0].Steps[0].Run(context.Background())
	c.Assert(err, tc.NotNil)
	c.Assert(s.InvalidatedCredentials, jc.IsTrue)
}

func (s *environUpgradeSuite) TestEnvironUpgradeOperations(c *tc.C) {
	ops := s.Env.UpgradeOperations(context.Background(), environs.UpgradeOperationsParams{})
	c.Assert(ops, tc.HasLen, 1)
	c.Assert(ops[0].TargetVersion, tc.Equals, 1)
	c.Assert(ops[0].Steps, tc.HasLen, 1)
	c.Assert(ops[0].Steps[0].Description(), tc.Equals, "Set disk labels")
}

func (s *environUpgradeSuite) TestEnvironUpgradeOperationSetDiskLabels(c *tc.C) {
	delete(s.BaseDisk.Labels, "juju-model-uuid")
	delete(s.BaseDisk.Labels, "juju-controller-uuid")
	s.FakeConn.GoogleDisks = []*google.Disk{s.BaseDisk}

	op0 := s.Env.UpgradeOperations(context.Background(), environs.UpgradeOperationsParams{
		ControllerUUID: "yup",
	})[0]
	c.Assert(op0.Steps[0].Run(context.Background()), jc.ErrorIsNil)

	setDiskLabelsCalled, calls := s.FakeConn.WasCalled("SetDiskLabels")
	c.Assert(setDiskLabelsCalled, jc.IsTrue)
	c.Check(calls, tc.HasLen, 1)
	c.Check(calls[0].ID, tc.Equals, s.BaseDisk.Name)
	c.Check(calls[0].ZoneName, tc.Equals, "home-zone")
	c.Check(calls[0].LabelFingerprint, tc.Equals, "foo")
	c.Check(calls[0].Labels, jc.DeepEquals, map[string]string{
		"juju-controller-uuid": "yup",
		"juju-model-uuid":      s.Env.Config().UUID(),
		"yodel":                "eh",
	})
}

func (s *environUpgradeSuite) TestEnvironUpgradeOperationSetDiskLabelsNoDescription(c *tc.C) {
	delete(s.BaseDisk.Labels, "juju-model-uuid")
	delete(s.BaseDisk.Labels, "juju-controller-uuid")
	s.BaseDisk.Description = ""
	s.FakeConn.GoogleDisks = []*google.Disk{s.BaseDisk}

	op0 := s.Env.UpgradeOperations(context.Background(), environs.UpgradeOperationsParams{
		ControllerUUID: "yup",
	})[0]
	c.Assert(op0.Steps[0].Run(context.Background()), jc.ErrorIsNil)

	setDiskLabelsCalled, _ := s.FakeConn.WasCalled("SetDiskLabels")
	c.Assert(setDiskLabelsCalled, jc.IsTrue)
}

func (s *environUpgradeSuite) TestEnvironUpgradeOperationSetDiskLabelsIdempotent(c *tc.C) {
	// s.BaseDisk is already labelled appropriately,
	// so we should not see a call to SetDiskLabels.
	s.FakeConn.GoogleDisks = []*google.Disk{s.BaseDisk}

	op0 := s.Env.UpgradeOperations(context.Background(), environs.UpgradeOperationsParams{
		ControllerUUID: "yup",
	})[0]
	c.Assert(op0.Steps[0].Run(context.Background()), jc.ErrorIsNil)

	setDiskLabelsCalled, _ := s.FakeConn.WasCalled("SetDiskLabels")
	c.Assert(setDiskLabelsCalled, jc.IsFalse)
}
