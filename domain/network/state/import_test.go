// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/tc"

	modelerrors "github.com/juju/juju/domain/model/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type cloudTypeSuite struct {
	linkLayerBaseSuite
}

func TestCloudTypeSuite(t *testing.T) {
	tc.Run(t, &cloudTypeSuite{})
}

func (s *cloudTypeSuite) TestGetModelCloudType(c *tc.C) {
	runner := s.TxnRunnerFactory()
	state := NewState(runner, loggertesting.WrapCheckLog(c))

	// Populate the model table in the model database
	s.query(c, `
INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type) 
VALUES (?, ?, 'test-model', 'admin', 'iaas', 'test-cloud', 'ec2')
		`, tc.Must(c, uuid.NewUUID).String(), "controller-uuid")

	modelCloudType, err := state.GetModelCloudType(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(modelCloudType, tc.DeepEquals, "ec2")
}

func (s *cloudTypeSuite) TestGetModelCloudTypeNotFound(c *tc.C) {
	runner := s.TxnRunnerFactory()
	state := NewState(runner, loggertesting.WrapCheckLog(c))

	_, err := state.GetModelCloudType(c.Context())
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}
