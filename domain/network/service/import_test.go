// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	modelerrors "github.com/juju/juju/domain/model/errors"
)

func (s *migrationSuite) TestGetModelCloudType(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.st.EXPECT().GetModelCloudType(gomock.Any()).Return("ec2", nil)

	cloudType, err := s.service(c).GetModelCloudType(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cloudType, tc.Equals, "ec2")
}

func (s *migrationSuite) TestGetModelCloudTypeFailedModelNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.st.EXPECT().GetModelCloudType(gomock.Any()).Return("", modelerrors.NotFound)

	_, err := s.service(c).GetModelCloudType(c.Context())
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}
