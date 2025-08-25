// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"cloud.google.com/go/compute/apiv1/computepb"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/gce"
)

type environUpgradeSuite struct {
	gce.BaseSuite
}

var _ = gc.Suite(&environUpgradeSuite{})

func (s *environUpgradeSuite) TestEnvironImplementsUpgrader(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)
	c.Assert(env, gc.Implements, new(environs.Upgrader))
}

func (s *environUpgradeSuite) TestEnvironUpgradeOperationsInvalidCredentialError(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)
	c.Assert(s.InvalidatedCredentials, jc.IsFalse)

	s.MockService.EXPECT().Disks(gomock.Any()).Return(nil, gce.InvalidCredentialError)

	ops := env.UpgradeOperations(s.CallCtx, environs.UpgradeOperationsParams{})
	err := ops[0].Steps[0].Run(s.CallCtx)
	c.Assert(err, gc.NotNil)
	c.Assert(s.InvalidatedCredentials, jc.IsTrue)
}

func (s *environUpgradeSuite) TestEnvironUpgradeOperations(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	ops := env.UpgradeOperations(s.CallCtx, environs.UpgradeOperationsParams{})
	c.Assert(ops, gc.HasLen, 1)
	c.Assert(ops[0].TargetVersion, gc.Equals, 1)
	c.Assert(ops[0].Steps, gc.HasLen, 1)
	c.Assert(ops[0].Steps[0].Description(), gc.Equals, "Set disk labels")
}

func (s *environUpgradeSuite) TestEnvironUpgradeOperationSetDiskLabels(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().Disks(gomock.Any()).Return([]*computepb.Disk{{
		Name:             ptr("zone--566fe7b2-c026-4a86-a2cc-84cb7f9a4868"),
		Status:           ptr("READY"),
		Zone:             ptr("zone"),
		LabelFingerprint: ptr("fingerprint"),
	}}, nil)
	s.MockService.EXPECT().SetDiskLabels(
		gomock.Any(), "zone", "zone--566fe7b2-c026-4a86-a2cc-84cb7f9a4868", "fingerprint",
		map[string]string{
			"juju-controller-uuid": s.ControllerUUID,
			"juju-model-uuid":      s.ModelUUID,
		})

	op0 := env.UpgradeOperations(s.CallCtx, environs.UpgradeOperationsParams{
		ControllerUUID: s.ControllerUUID,
	})[0]
	c.Assert(op0.Steps[0].Run(s.CallCtx), jc.ErrorIsNil)
}

func (s *environUpgradeSuite) TestEnvironUpgradeOperationSetDiskLabelsNoDescription(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().Disks(gomock.Any()).Return([]*computepb.Disk{{
		Name:   ptr("zone--566fe7b2-c026-4a86-a2cc-84cb7f9a4868"),
		Status: ptr("READY"),
		Labels: map[string]string{
			"juju-model-uuid": s.ModelUUID,
		},
	}}, nil)

	op0 := env.UpgradeOperations(s.CallCtx, environs.UpgradeOperationsParams{
		ControllerUUID: s.ControllerUUID,
	})[0]
	c.Assert(op0.Steps[0].Run(s.CallCtx), jc.ErrorIsNil)
}

func (s *environUpgradeSuite) TestEnvironUpgradeOperationSetDiskLabelsIdempotent(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().Disks(gomock.Any()).Return([]*computepb.Disk{{
		Name:             ptr("zone--566fe7b2-c026-4a86-a2cc-84cb7f9a4868"),
		Status:           ptr("READY"),
		Zone:             ptr("zone"),
		LabelFingerprint: ptr("fingerprint"),
		Labels: map[string]string{
			"juju-controller-uuid": s.ControllerUUID,
			"juju-model-uuid":      s.ModelUUID,
		},
	}}, nil)

	op0 := env.UpgradeOperations(s.CallCtx, environs.UpgradeOperationsParams{
		ControllerUUID: s.ControllerUUID,
	})[0]
	c.Assert(op0.Steps[0].Run(s.CallCtx), jc.ErrorIsNil)
}
