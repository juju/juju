// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"github.com/juju/tc"

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
	c.Assert(s.InvalidatedCredentials, tc.IsFalse)
	ops := s.Env.UpgradeOperations(c.Context(), environs.UpgradeOperationsParams{})
	err := ops[0].Steps[0].Run(c.Context())
	c.Assert(err, tc.NotNil)
	c.Assert(s.InvalidatedCredentials, tc.IsTrue)
}

func (s *environUpgradeSuite) TestEnvironUpgradeOperations(c *tc.C) {
	ops := s.Env.UpgradeOperations(c.Context(), environs.UpgradeOperationsParams{})
	c.Assert(ops, tc.HasLen, 1)
	c.Assert(ops[0].TargetVersion, tc.Equals, 1)
	c.Assert(ops[0].Steps, tc.HasLen, 1)
	c.Assert(ops[0].Steps[0].Description(), tc.Equals, "Set disk labels")
}

func (s *environUpgradeSuite) TestEnvironUpgradeOperationSetDiskLabels(c *tc.C) {
	delete(s.BaseDisk.Labels, "juju-model-uuid")
	delete(s.BaseDisk.Labels, "juju-controller-uuid")
	s.FakeConn.GoogleDisks = []*google.Disk{s.BaseDisk}

	op0 := s.Env.UpgradeOperations(c.Context(), environs.UpgradeOperationsParams{
		ControllerUUID: "yup",
	})[0]
	c.Assert(op0.Steps[0].Run(c.Context()), tc.ErrorIsNil)

	setDiskLabelsCalled, calls := s.FakeConn.WasCalled("SetDiskLabels")
	c.Assert(setDiskLabelsCalled, tc.IsTrue)
	c.Check(calls, tc.HasLen, 1)
	c.Check(calls[0].ID, tc.Equals, s.BaseDisk.Name)
	c.Check(calls[0].ZoneName, tc.Equals, "home-zone")
	c.Check(calls[0].LabelFingerprint, tc.Equals, "foo")
	c.Check(calls[0].Labels, tc.DeepEquals, map[string]string{
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

	op0 := s.Env.UpgradeOperations(c.Context(), environs.UpgradeOperationsParams{
		ControllerUUID: "yup",
	})[0]
	c.Assert(op0.Steps[0].Run(c.Context()), tc.ErrorIsNil)

	setDiskLabelsCalled, _ := s.FakeConn.WasCalled("SetDiskLabels")
	c.Assert(setDiskLabelsCalled, tc.IsTrue)
}

func (s *environUpgradeSuite) TestEnvironUpgradeOperationSetDiskLabelsIdempotent(c *tc.C) {
	// s.BaseDisk is already labelled appropriately,
	// so we should not see a call to SetDiskLabels.
	s.FakeConn.GoogleDisks = []*google.Disk{s.BaseDisk}

	op0 := s.Env.UpgradeOperations(c.Context(), environs.UpgradeOperationsParams{
		ControllerUUID: "yup",
	})[0]
	c.Assert(op0.Steps[0].Run(c.Context()), tc.ErrorIsNil)

	setDiskLabelsCalled, _ := s.FakeConn.WasCalled("SetDiskLabels")
	c.Assert(setDiskLabelsCalled, tc.IsFalse)
}
