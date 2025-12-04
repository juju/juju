// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package service

import (
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/semversion"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/secretbackend"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	"github.com/juju/juju/internal/secrets/provider/vault"
	"github.com/juju/juju/internal/testhelpers"
)

type modelSecretBackendServiceSuite struct {
	testhelpers.IsolationSuite

	mockState              *MockState
	mockAgentVersionGetter *MockAgentVersionGetter
}

func (s *modelSecretBackendServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.mockState = NewMockState(ctrl)
	s.mockAgentVersionGetter = NewMockAgentVersionGetter(ctrl)
	return ctrl
}

func TestModelSecretBackendServiceSuite(t *testing.T) {
	tc.Run(t, &modelSecretBackendServiceSuite{})
}

func (s *modelSecretBackendServiceSuite) TestGetModelSecretBackendFailedModelNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState, s.mockAgentVersionGetter)

	s.mockState.EXPECT().GetModelSecretBackendDetails(gomock.Any(), modelUUID).Return(secretbackend.ModelSecretBackend{}, modelerrors.NotFound)

	_, err := svc.GetModelSecretBackend(c.Context())
	c.Assert(err, tc.ErrorMatches, `getting model secret backend detail for "`+modelUUID.String()+`": model not found`)
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *modelSecretBackendServiceSuite) TestGetModelSecretBackendCAAS(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState, s.mockAgentVersionGetter)

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
	svc := NewModelSecretBackendService(modelUUID, s.mockState, s.mockAgentVersionGetter)

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
	svc := NewModelSecretBackendService(modelUUID, s.mockState, s.mockAgentVersionGetter)

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
	svc := NewModelSecretBackendService(modelUUID, s.mockState, s.mockAgentVersionGetter)

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
	svc := NewModelSecretBackendService(modelUUID, s.mockState, s.mockAgentVersionGetter)

	err := svc.SetModelSecretBackend(c.Context(), "")
	c.Assert(err, tc.ErrorMatches, `missing backend name`)
}

func (s *modelSecretBackendServiceSuite) TestSetModelSecretBackendFailedReservedNameKubernetes(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState, s.mockAgentVersionGetter)

	err := svc.SetModelSecretBackend(c.Context(), "kubernetes")
	c.Assert(err, tc.ErrorMatches, `secret backend name "kubernetes" not valid`)
	c.Assert(err, tc.ErrorIs, secretbackenderrors.NotValid)
}

func (s *modelSecretBackendServiceSuite) TestSetModelSecretBackendFailedReservedNameInternal(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState, s.mockAgentVersionGetter)

	err := svc.SetModelSecretBackend(c.Context(), "internal")
	c.Assert(err, tc.ErrorMatches, `secret backend name "internal" not valid`)
	c.Assert(err, tc.ErrorIs, secretbackenderrors.NotValid)
}

func (s *modelSecretBackendServiceSuite) TestSetModelSecretBackendFailedUnknownModelType(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState, s.mockAgentVersionGetter)

	s.mockState.EXPECT().GetModelType(gomock.Any(), modelUUID).Return("bad-type", nil)

	err := svc.SetModelSecretBackend(c.Context(), "auto")
	c.Assert(err, tc.ErrorMatches, `setting model secret backend for unsupported model type "bad-type" for model "`+modelUUID.String()+`"`)
}

func (s *modelSecretBackendServiceSuite) TestSetModelSecretBackendFailedModelNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState, s.mockAgentVersionGetter)

	s.mockState.EXPECT().GetModelType(gomock.Any(), modelUUID).Return("", modelerrors.NotFound)

	err := svc.SetModelSecretBackend(c.Context(), "auto")
	c.Assert(err, tc.ErrorMatches, `getting model type for "`+modelUUID.String()+`": model not found`)
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *modelSecretBackendServiceSuite) TestSetModelSecretBackendFailedSecretBackendNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState, s.mockAgentVersionGetter)

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
	svc := NewModelSecretBackendService(modelUUID, s.mockState, s.mockAgentVersionGetter)

	backendName := "backend-name"
	s.mockState.EXPECT().SetModelSecretBackend(gomock.Any(), modelUUID, backendName).Return(nil)
	// compatibility check
	s.mockState.EXPECT().GetSecretBackend(gomock.Any(), secretbackend.BackendIdentifier{Name: backendName}).Return(&secretbackend.
		SecretBackend{ /* no mountPath */ }, nil)

	err := svc.SetModelSecretBackend(c.Context(), backendName)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelSecretBackendServiceSuite) TestSetModelSecretBackendWithMountPathJuju3Error(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState, s.mockAgentVersionGetter)

	backendName := "backend-name"
	// compatibility check
	s.mockState.EXPECT().GetSecretBackend(gomock.Any(), secretbackend.BackendIdentifier{Name: backendName}).Return(&secretbackend.
		SecretBackend{BackendType: vault.BackendType, Config: map[string]any{vault.MountPathKey: "foo"}}, nil)
	s.mockAgentVersionGetter.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(semversion.MustParse("3.6.11"),
		nil)

	err := svc.SetModelSecretBackend(c.Context(), backendName)
	c.Assert(err, tc.ErrorIs, secretbackenderrors.NotSupported)
}

func (s *modelSecretBackendServiceSuite) TestSetModelSecretBackendWithMountPathJuju4Error(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState, s.mockAgentVersionGetter)

	backendName := "backend-name"
	// compatibility check
	s.mockState.EXPECT().GetSecretBackend(gomock.Any(), secretbackend.BackendIdentifier{Name: backendName}).Return(&secretbackend.
		SecretBackend{BackendType: vault.BackendType, Config: map[string]any{vault.MountPathKey: "foo"}}, nil)
	s.mockAgentVersionGetter.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(semversion.MustParse("4.0.0"),
		nil)

	err := svc.SetModelSecretBackend(c.Context(), backendName)
	c.Assert(err, tc.ErrorIs, secretbackenderrors.NotSupported)
}

func (s *modelSecretBackendServiceSuite) TestSetModelSecretBackendWithMountPathJuju3Success(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState, s.mockAgentVersionGetter)

	backendName := "backend-name"
	// compatibility check
	s.mockState.EXPECT().GetSecretBackend(gomock.Any(), secretbackend.BackendIdentifier{Name: backendName}).Return(&secretbackend.
		SecretBackend{BackendType: vault.BackendType, Config: map[string]any{vault.MountPathKey: "foo"}}, nil)
	s.mockAgentVersionGetter.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(semversion.MustParse("3.6.12"),
		nil)
	s.mockState.EXPECT().SetModelSecretBackend(gomock.Any(), modelUUID, backendName).Return(nil)

	err := svc.SetModelSecretBackend(c.Context(), backendName)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelSecretBackendServiceSuite) TestSetModelSecretBackendWithMountPathJuju4Success(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState, s.mockAgentVersionGetter)

	backendName := "backend-name"
	// compatibility check
	s.mockState.EXPECT().GetSecretBackend(gomock.Any(), secretbackend.BackendIdentifier{Name: backendName}).Return(&secretbackend.
		SecretBackend{BackendType: vault.BackendType, Config: map[string]any{vault.MountPathKey: "foo"}}, nil)
	s.mockAgentVersionGetter.EXPECT().GetModelTargetAgentVersion(gomock.Any()).Return(semversion.MustParse("4.0.1"),
		nil)
	s.mockState.EXPECT().SetModelSecretBackend(gomock.Any(), modelUUID, backendName).Return(nil)

	err := svc.SetModelSecretBackend(c.Context(), backendName)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelSecretBackendServiceSuite) TestSetModelSecretBackendCAASAuto(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState, s.mockAgentVersionGetter)

	s.mockState.EXPECT().GetModelType(gomock.Any(), modelUUID).Return(coremodel.CAAS, nil)
	s.mockState.EXPECT().SetModelSecretBackend(gomock.Any(), modelUUID, "kubernetes").Return(nil)

	err := svc.SetModelSecretBackend(c.Context(), "auto")
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelSecretBackendServiceSuite) TestSetModelSecretBackendIAASAuto(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelSecretBackendService(modelUUID, s.mockState, s.mockAgentVersionGetter)

	s.mockState.EXPECT().GetModelType(gomock.Any(), modelUUID).Return(coremodel.IAAS, nil)
	s.mockState.EXPECT().SetModelSecretBackend(gomock.Any(), modelUUID, "internal").Return(nil)

	err := svc.SetModelSecretBackend(c.Context(), "auto")
	c.Assert(err, tc.ErrorIsNil)
}
