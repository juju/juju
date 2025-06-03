// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coremodel "github.com/juju/juju/core/model"
	clouderrors "github.com/juju/juju/domain/cloud/errors"
)

// modelTypeSuite is a test suite for asserting the logic around determining a
// model's type.
type modelTypeSuite struct {
	state *MockModelTypeState
}

// TestModelTypeSuite runs all of the tests in the [modelTypeSuite].
func TestModelTypeSuite(t *testing.T) {
	tc.Run(t, &modelTypeSuite{})
}

func (m *modelTypeSuite) TearDownTest(c *tc.C) {
	m.state = nil
}

func (m *modelTypeSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	m.state = NewMockModelTypeState(ctrl)
	return ctrl
}

// TestDetermineModelTypeForCAASCloud tests the case where if the type of a
// cloud is considered a CAAS cloud the model type returned is [coremodel.CAAS].
func (m *modelTypeSuite) TestDetermineModelTypeForCAASCloud(c *tc.C) {
	defer m.setupMocks(c).Finish()

	cloudName := "test-cloud"
	cloudType := "kubernetes"

	m.state.EXPECT().GetCloudType(
		gomock.Any(), cloudName,
	).Return(cloudType, nil)

	modelType, err := DetermineModelTypeForCloud(
		c.Context(), m.state, cloudName,
	)
	c.Check(err, tc.IsNil)
	c.Check(modelType, tc.Equals, coremodel.CAAS)
}

// TestDetermineModelTypeForIAASCloud tests the case where if the type of a
// clouid is considered a IAAS cloud the model type returned is [coremodel.IAAS].
func (m *modelTypeSuite) TestDetermineModelTypeForIAASCloud(c *tc.C) {
	defer m.setupMocks(c).Finish()

	cloudName := "test-cloud"
	cloudType := "ec2"

	m.state.EXPECT().GetCloudType(
		gomock.Any(), cloudName,
	).Return(cloudType, nil)

	modelType, err := DetermineModelTypeForCloud(
		c.Context(), m.state, cloudName,
	)
	c.Check(err, tc.IsNil)
	c.Check(modelType, tc.Equals, coremodel.IAAS)
}

// TestDetermineModelTypeForCloudNotFound tests the case where if the model
// type of a cloud is asked for but the cloud does not exist, a
// [clouderrors.NotFound] error is returned.
func (m *modelTypeSuite) TestDetermineModelTypeForCloudNotFound(c *tc.C) {
	defer m.setupMocks(c).Finish()

	cloudName := "test-cloud"

	m.state.EXPECT().GetCloudType(
		gomock.Any(), cloudName,
	).Return("", clouderrors.NotFound)

	_, err := DetermineModelTypeForCloud(
		c.Context(), m.state, cloudName,
	)
	c.Check(err, tc.ErrorIs, clouderrors.NotFound)
}
