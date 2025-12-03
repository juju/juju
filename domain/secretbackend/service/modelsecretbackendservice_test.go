// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/secretbackend"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/internal/testhelpers"
)

type modelSecretBackendServiceSuite struct {
	testhelpers.IsolationSuite

	mockState *MockState
}

func (s *modelSecretBackendServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.mockState = NewMockState(ctrl)
	return ctrl
}

func TestModelSecretBackendServiceSuite(t *testing.T) {
	tc.Run(t, &modelSecretBackendServiceSuite{})
}

func (s *modelSecretBackendServiceSuite) TestGetModelSecretBackendFailedModelNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState)

	s.mockState.EXPECT().GetModelSecretBackendDetails(gomock.Any(), modelUUID).Return(secretbackend.ModelSecretBackend{}, modelerrors.NotFound)

	_, err := svc.GetModelSecretBackend(c.Context())
	c.Assert(err, tc.ErrorMatches, `getting model secret backend detail for "`+modelUUID.String()+`": model not found`)
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *modelSecretBackendServiceSuite) TestGetModelSecretBackendCAAS(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState)

	s.mockState.EXPECT().GetModelSecretBackendDetails(gomock.Any(), modelUUID).Return(secretbackend.ModelSecretBackend{
		SecretBackendName: "backend-name",
		ModelType:         coremodel.CAAS,
	}, nil)

	backendID, err := svc.GetModelSecretBackend(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(backendID, tc.Equals, "backend-name")
}

func (s *modelSecretBackendServiceSuite) TestGetModelSecretBackendIAAS(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState)

	s.mockState.EXPECT().GetModelSecretBackendDetails(gomock.Any(), modelUUID).Return(secretbackend.ModelSecretBackend{
		SecretBackendName: "backend-name",
		ModelType:         coremodel.IAAS,
	}, nil)

	backendID, err := svc.GetModelSecretBackend(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(backendID, tc.Equals, "backend-name")
}

func (s *modelSecretBackendServiceSuite) TestGetModelSecretBackendCAASAuto(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState)

	s.mockState.EXPECT().GetModelSecretBackendDetails(gomock.Any(), modelUUID).Return(secretbackend.ModelSecretBackend{
		SecretBackendName: "kubernetes",
		ModelType:         coremodel.CAAS,
	}, nil)

	backendID, err := svc.GetModelSecretBackend(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(backendID, tc.Equals, "auto")
}

func (s *modelSecretBackendServiceSuite) TestGetModelSecretBackendIAASAuto(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState)

	s.mockState.EXPECT().GetModelSecretBackendDetails(gomock.Any(), modelUUID).Return(secretbackend.ModelSecretBackend{
		SecretBackendName: "internal",
		ModelType:         coremodel.IAAS,
	}, nil)

	backendID, err := svc.GetModelSecretBackend(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(backendID, tc.Equals, "auto")
}

func (s *modelSecretBackendServiceSuite) TestSetModelSecretBackendFailedEmptyBackendName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState)

	err := svc.SetModelSecretBackend(c.Context(), "")
	c.Assert(err, tc.ErrorMatches, `missing backend name`)
}

func (s *modelSecretBackendServiceSuite) TestSetModelSecretBackendFailedReservedNameKubernetes(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState)

	err := svc.SetModelSecretBackend(c.Context(), "kubernetes")
	c.Assert(err, tc.ErrorMatches, `secret backend name "kubernetes" not valid`)
	c.Assert(err, tc.ErrorIs, secretbackenderrors.NotValid)
}

func (s *modelSecretBackendServiceSuite) TestSetModelSecretBackendFailedReservedNameInternal(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState)

	err := svc.SetModelSecretBackend(c.Context(), "internal")
	c.Assert(err, tc.ErrorMatches, `secret backend name "internal" not valid`)
	c.Assert(err, tc.ErrorIs, secretbackenderrors.NotValid)
}

func (s *modelSecretBackendServiceSuite) TestSetModelSecretBackendFailedUnkownModelType(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState)

	s.mockState.EXPECT().GetModelType(gomock.Any(), modelUUID).Return("bad-type", nil)

	err := svc.SetModelSecretBackend(c.Context(), "auto")
	c.Assert(err, tc.ErrorMatches, `setting model secret backend for unsupported model type "bad-type" for model "`+modelUUID.String()+`"`)
}

func (s *modelSecretBackendServiceSuite) TestSetModelSecretBackendFailedModelNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState)

	s.mockState.EXPECT().GetModelType(gomock.Any(), modelUUID).Return("", modelerrors.NotFound)

	err := svc.SetModelSecretBackend(c.Context(), "auto")
	c.Assert(err, tc.ErrorMatches, `getting model type for "`+modelUUID.String()+`": model not found`)
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *modelSecretBackendServiceSuite) TestSetModelSecretBackendFailedSecretBackendNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState)

	s.mockState.EXPECT().GetModelType(gomock.Any(), modelUUID).Return(coremodel.CAAS, nil)
	s.mockState.EXPECT().SetModelSecretBackend(gomock.Any(), modelUUID, "kubernetes").Return(secretbackenderrors.NotFound)

	err := svc.SetModelSecretBackend(c.Context(), "auto")
	c.Assert(err, tc.ErrorMatches, `setting model secret backend for "`+modelUUID.String()+`": secret backend not found`)
	c.Assert(err, tc.ErrorIs, secretbackenderrors.NotFound)
}

func (s *modelSecretBackendServiceSuite) TestSetModelSecretBackend(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState)

	s.mockState.EXPECT().SetModelSecretBackend(gomock.Any(), modelUUID, "backend-name").Return(nil)

	err := svc.SetModelSecretBackend(c.Context(), "backend-name")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelSecretBackendServiceSuite) TestSetModelSecretBackendCAASAuto(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState)

	s.mockState.EXPECT().GetModelType(gomock.Any(), modelUUID).Return(coremodel.CAAS, nil)
	s.mockState.EXPECT().SetModelSecretBackend(gomock.Any(), modelUUID, "kubernetes").Return(nil)

	err := svc.SetModelSecretBackend(c.Context(), "auto")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelSecretBackendServiceSuite) TestSetModelSecretBackendIAASAuto(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState)

	s.mockState.EXPECT().GetModelType(gomock.Any(), modelUUID).Return(coremodel.IAAS, nil)
	s.mockState.EXPECT().SetModelSecretBackend(gomock.Any(), modelUUID, "internal").Return(nil)

	err := svc.SetModelSecretBackend(c.Context(), "auto")
	c.Assert(err, tc.ErrorIsNil)
}
