// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"testing"
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/agentbinary"
	coreconstraints "github.com/juju/juju/core/constraints"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	corelife "github.com/juju/juju/core/life"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	corepermission "github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/semversion"
	corestatus "github.com/juju/juju/core/status"
	usertesting "github.com/juju/juju/core/user/testing"
	jujuversion "github.com/juju/juju/core/version"
	accesserrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/domain/constraints"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/modelagent"
	networkerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/domain/storage"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/simplestreams"
	internalstorage "github.com/juju/juju/internal/storage"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

type modelServiceSuite struct {
	testhelpers.IsolationSuite

	mockControllerState        *MockControllerState
	mockModelState             *MockModelState
	mockEnvironVersionProvider *MockEnvironVersionProvider
}

func (s *modelServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockControllerState = NewMockControllerState(ctrl)
	s.mockModelState = NewMockModelState(ctrl)
	s.mockEnvironVersionProvider = NewMockEnvironVersionProvider(ctrl)
	return ctrl
}
func TestModelServiceSuite(t *testing.T) {
	tc.Run(t, &modelServiceSuite{})
}

// environVersionProviderGetter provides a test implementation of
// [EnvironVersionProviderFunc] that uses the mocked [EnvironVersionProvider] on
// this suite.
func (s *modelServiceSuite) environVersionProviderGetter() EnvironVersionProviderFunc {
	return func(string) (EnvironVersionProvider, error) {
		return s.mockEnvironVersionProvider, nil
	}
}

// TestGetModelConstraints is asserting the happy path of retrieving the set
// model constraints.
func (s *modelServiceSuite) TestGetModelConstraints(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelConstraints := constraints.Constraints{
		Arch:      ptr("amd64"),
		Container: ptr(instance.NONE),
		CpuCores:  ptr(uint64(4)),
		Mem:       ptr(uint64(1024)),
		RootDisk:  ptr(uint64(1024)),
	}
	s.mockModelState.EXPECT().GetModelConstraints(gomock.Any()).Return(modelConstraints, nil)

	svc := NewModelService(
		modeltesting.GenModelUUID(c),
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	result, err := svc.GetModelConstraints(c.Context())
	c.Check(err, tc.ErrorIsNil)

	cons := coreconstraints.Value{
		Arch:      ptr("amd64"),
		Container: ptr(instance.NONE),
		CpuCores:  ptr(uint64(4)),
		Mem:       ptr(uint64(1024)),
		RootDisk:  ptr(uint64(1024)),
	}
	c.Check(result, tc.DeepEquals, cons)
}

// TestGetModelConstraintsNotFound is asserting that when the state layer
// reports that no model constraints exist with an error of
// [modelerrors.ConstraintsNotFound] that we correctly handle this error and
// receive a zero value constraints object back with no error.
func (s *modelServiceSuite) TestGetModelConstraintsNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.mockModelState.EXPECT().GetModelConstraints(gomock.Any()).Return(
		constraints.Constraints{},
		modelerrors.ConstraintsNotFound,
	)

	svc := NewModelService(
		modeltesting.GenModelUUID(c),
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	result, err := svc.GetModelConstraints(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, coreconstraints.Value{})
}

// TestGetModelConstraintsFailedModelNotFound is asserting that if we ask for
// model constraints and the model does not exist in the database we get back
// an error satisfying [modelerrors.NotFound].
func (s *modelServiceSuite) TestGetModelConstraintsFailedModelNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.mockModelState.EXPECT().GetModelConstraints(gomock.Any()).Return(constraints.Constraints{}, modelerrors.NotFound)

	svc := NewModelService(
		modeltesting.GenModelUUID(c),
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	_, err := svc.GetModelConstraints(c.Context())
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *modelServiceSuite) TestSetModelConstraints(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelCons := constraints.Constraints{
		Arch:      ptr("amd64"),
		Container: ptr(instance.NONE),
		CpuCores:  ptr(uint64(4)),
		Mem:       ptr(uint64(1024)),
		RootDisk:  ptr(uint64(1024)),
		Spaces: ptr([]constraints.SpaceConstraint{
			{SpaceName: "space1", Exclude: false},
			{SpaceName: "space2", Exclude: true},
		}),
	}
	s.mockModelState.EXPECT().SetModelConstraints(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, m constraints.Constraints) error {
			c.Check(m, tc.DeepEquals, modelCons)
			return nil
		})

	svc := NewModelService(
		modeltesting.GenModelUUID(c),
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)

	cons := coreconstraints.Value{
		Arch:      ptr("amd64"),
		Container: ptr(instance.NONE),
		CpuCores:  ptr(uint64(4)),
		Mem:       ptr(uint64(1024)),
		RootDisk:  ptr(uint64(1024)),
		Spaces:    ptr([]string{"space1", "^space2"}),
	}
	err := svc.SetModelConstraints(c.Context(), cons)
	c.Check(err, tc.ErrorIsNil)
}

// TestSetModelConstraintsInvalidContainerType is asserting that if we provide
// a constraints that uses an invalid container type we get back an error that
// satisfies [machineerrors.InvalidContainerType].
func (s *modelServiceSuite) TestSetModelConstraintsInvalidContainerType(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	badConstraints := coreconstraints.Value{
		Container: ptr(instance.ContainerType("bad")),
	}
	modelCons := constraints.Constraints{
		Container: ptr(instance.ContainerType("bad")),
	}

	s.mockModelState.EXPECT().SetModelConstraints(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, m constraints.Constraints) error {
			c.Check(m, tc.DeepEquals, modelCons)
			return machineerrors.InvalidContainerType
		})

	svc := NewModelService(
		modeltesting.GenModelUUID(c),
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	err := svc.SetModelConstraints(c.Context(), badConstraints)
	c.Check(err, tc.ErrorIs, machineerrors.InvalidContainerType)
}

func (s *modelServiceSuite) TestSetModelConstraintsFailedSpaceNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	cons := coreconstraints.Value{
		Arch:      ptr("amd64"),
		Container: ptr(instance.NONE),
		CpuCores:  ptr(uint64(4)),
		Mem:       ptr(uint64(1024)),
		RootDisk:  ptr(uint64(1024)),
		Spaces:    ptr([]string{"space1"}),
	}
	modelCons := constraints.Constraints{
		Arch:      ptr("amd64"),
		Container: ptr(instance.NONE),
		CpuCores:  ptr(uint64(4)),
		Mem:       ptr(uint64(1024)),
		RootDisk:  ptr(uint64(1024)),
		Spaces: ptr([]constraints.SpaceConstraint{
			{SpaceName: "space1", Exclude: false},
		}),
	}
	s.mockModelState.EXPECT().SetModelConstraints(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, m constraints.Constraints) error {
			c.Check(m, tc.DeepEquals, modelCons)
			return networkerrors.SpaceNotFound
		})

	svc := NewModelService(
		modeltesting.GenModelUUID(c),
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	err := svc.SetModelConstraints(c.Context(), cons)
	c.Check(err, tc.ErrorIs, networkerrors.SpaceNotFound)
}

func (s *modelServiceSuite) TestSetModelConstraintsFailedModelNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	cons := coreconstraints.Value{
		Arch:      ptr("amd64"),
		Container: ptr(instance.NONE),
		CpuCores:  ptr(uint64(4)),
		Mem:       ptr(uint64(1024)),
		RootDisk:  ptr(uint64(1024)),
	}
	modelCons := constraints.Constraints{
		Arch:      ptr("amd64"),
		Container: ptr(instance.NONE),
		CpuCores:  ptr(uint64(4)),
		Mem:       ptr(uint64(1024)),
		RootDisk:  ptr(uint64(1024)),
	}
	s.mockModelState.EXPECT().SetModelConstraints(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, m constraints.Constraints) error {
			c.Check(m, tc.DeepEquals, modelCons)
			return modelerrors.NotFound
		})

	svc := NewModelService(
		modeltesting.GenModelUUID(c),
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	err := svc.SetModelConstraints(c.Context(), cons)
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *modelServiceSuite) TestGetModelMetrics(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	controllerUUID := uuid.MustNewUUID()
	modelUUID := modeltesting.GenModelUUID(c)
	metrics := coremodel.ModelMetrics{
		Model: coremodel.ModelInfo{
			UUID:           modelUUID,
			ControllerUUID: controllerUUID,
			Name:           "my-awesome-model",
			Cloud:          "aws",
			CloudType:      "ec2",
			CloudRegion:    "myregion",
			Type:           coremodel.IAAS,
		},
	}
	s.mockModelState.EXPECT().GetModelMetrics(gomock.Any()).Return(metrics, nil)

	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	result, err := svc.GetModelMetrics(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, metrics)
}

// TestCreateModelAgentVersionUnsupportedGreater is asserting that if we try and
// create a model with an agent version that is greater then that of the
// controller the operation fails with a [modelerrors.AgentVersionNotSupported]
// error.
func (s *modelServiceSuite) TestCreateModelAgentVersionUnsupportedGreater(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)

	s.mockControllerState.EXPECT().GetModelSeedInformation(
		gomock.Any(), modelUUID).Return(coremodel.ModelInfo{}, nil)

	agentVersion, err := semversion.Parse("99.9.9")
	c.Assert(err, tc.ErrorIsNil)

	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)

	err = svc.CreateModelWithAgentVersion(
		c.Context(), agentVersion,
	)
	c.Assert(err, tc.ErrorIs, modelerrors.AgentVersionNotSupported)
}

// TestAgentVersionUnsupportedLess is asserting that if we try and create a
// model with an agent version that is less then that of the controller.
func (s *modelServiceSuite) TestAgentVersionUnsupportedLess(c *tc.C) {
	c.Skip("This tests needs to be rewritten once tools metadata is implemented for the controller")

	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)

	s.mockControllerState.EXPECT().GetModelSeedInformation(
		gomock.Any(), modelUUID,
	).Return(coremodel.ModelInfo{}, nil)

	agentVersion, err := semversion.Parse("1.9.9")
	c.Assert(err, tc.ErrorIsNil)

	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	err = svc.CreateModelWithAgentVersion(
		c.Context(), agentVersion,
	)
	// Add the correct error detail when restoring this test.
	c.Assert(err, tc.NotNil)
}

// TestCreateModelForVersionInvalidStream is testing that when
// [ModelService.CreateModelForVersionAndStream] is called with an agent stream
// that isn't understood or supported we get back an error that satisfies
// [modelerrors.AgentStreamNotValid].
func (s *modelServiceSuite) TestCreateModelForVersionInvalidStream(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	s.mockControllerState.EXPECT().GetModelSeedInformation(gomock.Any(), modelUUID).Return(coremodel.ModelInfo{}, nil)

	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	err := svc.CreateModelWithAgentVersionStream(
		c.Context(),
		jujuversion.Current,
		agentbinary.AgentStream("bad stream"),
	)
	c.Check(err, tc.ErrorIs, modelerrors.AgentStreamNotValid)
}

func (s *modelServiceSuite) TestDeleteModel(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)

	s.mockModelState.EXPECT().Delete(gomock.Any(), modelUUID).Return(nil)

	err := svc.DeleteModel(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *modelServiceSuite) TestDeleteModelFailedNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)

	s.mockModelState.EXPECT().Delete(gomock.Any(), modelUUID).Return(modelerrors.NotFound)

	err := svc.DeleteModel(c.Context())
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *modelServiceSuite) TestGetEnvironVersion(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)

	s.mockModelState.EXPECT().GetModelCloudType(gomock.Any()).Return("ec2", nil)
	s.mockEnvironVersionProvider.EXPECT().Version().Return(2)

	version, err := svc.GetEnvironVersion(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(version, tc.Equals, 2)
}

func (s *modelServiceSuite) TestGetEnvironVersionFailedModelNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)

	s.mockModelState.EXPECT().GetModelCloudType(gomock.Any()).Return("", modelerrors.NotFound)

	_, err := svc.GetEnvironVersion(c.Context())
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *modelServiceSuite) TestGetModelCloudType(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)

	s.mockModelState.EXPECT().GetModelCloudType(gomock.Any()).Return("ec2", nil)

	cloudType, err := svc.GetModelCloudType(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cloudType, tc.Equals, "ec2")
}

func (s *modelServiceSuite) TestGetModelCloudTypeFailedModelNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)

	s.mockModelState.EXPECT().GetModelCloudType(gomock.Any()).Return("", modelerrors.NotFound)

	_, err := svc.GetModelCloudType(c.Context())
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *modelServiceSuite) TestIsControllerModel(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	s.mockModelState.EXPECT().IsControllerModel(gomock.Any()).Return(true, nil)

	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	isControllerModel, err := svc.IsControllerModel(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(isControllerModel, tc.IsTrue)

	modelUUID = modeltesting.GenModelUUID(c)
	s.mockModelState.EXPECT().IsControllerModel(gomock.Any()).Return(false, nil)

	svc = NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	isControllerModel, err = svc.IsControllerModel(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(isControllerModel, tc.IsFalse)
}

func (s *modelServiceSuite) TestIsControllerModelNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	s.mockModelState.EXPECT().IsControllerModel(gomock.Any()).Return(false, modelerrors.NotFound)

	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	_, err := svc.IsControllerModel(c.Context())
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *modelServiceSuite) TestHasValidCredential(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	s.mockControllerState.EXPECT().HasValidCredential(gomock.Any(), modelUUID).Return(true, nil)

	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	hasValidCredential, err := svc.HasValidCredential(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(hasValidCredential, tc.IsTrue)

	modelUUID = modeltesting.GenModelUUID(c)
	s.mockControllerState.EXPECT().HasValidCredential(gomock.Any(), modelUUID).Return(false, nil)

	svc = NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	hasValidCredential, err = svc.HasValidCredential(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(hasValidCredential, tc.IsFalse)
}

func (s *modelServiceSuite) TestHasValidCredentialNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	s.mockControllerState.EXPECT().HasValidCredential(gomock.Any(), modelUUID).Return(false, modelerrors.NotFound)

	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)
	_, err := svc.HasValidCredential(c.Context())
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)
}

// GetModelType asserts the happy path of getting the models current
// [coremodel.ModelType]. We are looking to see here that the service correctly
// passes along the information received from the state layer.
func (s *modelServiceSuite) TestGetModelType(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.mockModelState.EXPECT().GetModelType(gomock.Any()).Return(coremodel.IAAS, nil)

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)

	modelType, err := svc.GetModelType(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(modelType, tc.Equals, coremodel.IAAS)
}

// GetModelTypeNotFound is asserting that if we ask for the model type of the
// current model but it doesn't exist in the state layer we correctly pass only
// the [modelerrors.NotFound] error received. This fulfills the contract defined
// for [ModelService.GetModelType].
func (s *modelServiceSuite) TestGetModelTypeNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.mockModelState.EXPECT().GetModelType(gomock.Any()).Return(coremodel.ModelType(""), modelerrors.NotFound)

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)

	_, err := svc.GetModelType(c.Context())
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)
}

// TestGetModelSummaryNotFound is asserting that if we ask for the model summary
// and the model doesn't exist, the caller gets an error satisfying
// [modelerrors.NotFound].
func (s *modelServiceSuite) TestGetModelSummaryNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)

	s.mockControllerState.EXPECT().GetModelSummary(
		gomock.Any(), modelUUID,
	).Return(model.ModelSummary{}, modelerrors.NotFound).AnyTimes()
	s.mockModelState.EXPECT().GetModelInfoSummary(
		gomock.Any(),
	).Return(model.ModelInfoSummary{}, modelerrors.NotFound).AnyTimes()

	_, err := svc.GetModelSummary(c.Context())
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)
}

// TestGetModelSummary is asserting the happy path of getting the model summary.
func (s *modelServiceSuite) TestGetModelSummary(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)

	controllerUUID, err := uuid.NewUUID()
	c.Check(err, tc.ErrorIsNil)
	s.mockModelState.EXPECT().GetModelInfoSummary(gomock.Any()).Return(model.ModelInfoSummary{
		UUID:           modelUUID,
		Name:           "my-awesome-model",
		ControllerUUID: controllerUUID.String(),
		ModelType:      coremodel.IAAS,
		CloudName:      "aws",
		CloudType:      "ec2",
		CloudRegion:    "myregion",
		AgentVersion:   jujuversion.Current,
		MachineCount:   10,
		CoreCount:      10,
		UnitCount:      10,
	}, nil)
	s.mockControllerState.EXPECT().GetModelSummary(gomock.Any(), modelUUID).Return(model.ModelSummary{
		Life: corelife.Alive,
		State: model.ModelState{
			Destroying:                false,
			Migrating:                 false,
			HasInvalidCloudCredential: false,
		},
	}, nil)

	mc := tc.NewMultiChecker()
	mc.AddExpr("_.Status.Since", tc.Ignore)
	summary, err := svc.GetModelSummary(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Assert(summary, mc, coremodel.ModelSummary{
		Name:           "my-awesome-model",
		UUID:           modelUUID,
		ModelType:      coremodel.IAAS,
		CloudName:      "aws",
		CloudType:      "ec2",
		CloudRegion:    "myregion",
		Life:           corelife.Alive,
		ControllerUUID: controllerUUID.String(),
		IsController:   false,
		AgentVersion:   jujuversion.Current,
		Status: corestatus.StatusInfo{
			Status: corestatus.Available,
		},
		MachineCount: 10,
		CoreCount:    10,
		UnitCount:    10,
	})
}

// TestGetUserModelSummaryModelNotFound is asserting that if a caller asks for a
// user model summary and the model doesn't exist, we get back a
// [modelerrors.NotFound] error.
func (s *modelServiceSuite) TestGetUserModelSummaryModelNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)

	userUUID := usertesting.GenUserUUID(c)
	s.mockControllerState.EXPECT().GetUserModelSummary(
		gomock.Any(),
		userUUID, modelUUID,
	).Return(model.UserModelSummary{}, modelerrors.NotFound).AnyTimes()
	s.mockControllerState.EXPECT().GetModelSummary(
		gomock.Any(), modelUUID,
	).Return(model.ModelSummary{}, modelerrors.NotFound).AnyTimes()

	_, err := svc.GetUserModelSummary(c.Context(), userUUID)
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)
}

// TestGetUserModelSummaryUserNotFound tests that if a model summary is asked
// for by a caller but the user doesn't exist, an error satisfying
// [accesserrors.UserNotFound] is returned.
func (s *modelServiceSuite) TestGetUserModelSummaryUserNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)

	userUUID := usertesting.GenUserUUID(c)
	s.mockControllerState.EXPECT().GetUserModelSummary(
		gomock.Any(),
		userUUID, modelUUID,
	).Return(model.UserModelSummary{}, accesserrors.UserNotFound)
	s.mockControllerState.EXPECT().GetModelSummary(
		gomock.Any(), modelUUID,
	).Return(model.ModelSummary{}, nil).AnyTimes()

	_, err := svc.GetUserModelSummary(c.Context(), userUUID)
	c.Check(err, tc.ErrorIs, accesserrors.UserNotFound)
}

// TestGetUserModelSummaryAccessNotFound tests that if a user model summary is
// asked for by a caller but the user doesn't have access to the model, an error
// satisfying [accesserrors.AccessNotFound] is returned.
func (s *modelServiceSuite) TestGetUserModelSummaryAccessNotFound(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)

	userUUID := usertesting.GenUserUUID(c)
	s.mockControllerState.EXPECT().GetUserModelSummary(
		gomock.Any(),
		userUUID, modelUUID,
	).Return(model.UserModelSummary{}, accesserrors.AccessNotFound)
	s.mockControllerState.EXPECT().GetModelSummary(
		gomock.Any(), modelUUID,
	).Return(model.ModelSummary{}, nil).AnyTimes()

	_, err := svc.GetUserModelSummary(c.Context(), userUUID)
	c.Check(err, tc.ErrorIs, accesserrors.AccessNotFound)
}

// TestGetUserModelSummaryUserUUIDNotValid verifies that requesting a user model
// summary with an invalid user UUID, results in an error that satisfies
// [coreerrors.NotValid].
func (s *modelServiceSuite) TestGetUserModelSummaryUserUUIDNotValid(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)

	_, err := svc.GetUserModelSummary(c.Context(), "")
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestGetUserModelSummary tests the happy path of
// [ModelService.GetUserModelSummary].
func (s *modelServiceSuite) TestGetUserModelSummary(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	svc := NewModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		DefaultAgentBinaryFinder(),
	)

	controllerUUID, err := uuid.NewUUID()
	c.Check(err, tc.ErrorIsNil)
	s.mockModelState.EXPECT().GetModelInfoSummary(gomock.Any()).Return(model.ModelInfoSummary{
		UUID:           modelUUID,
		Name:           "my-awesome-model",
		ControllerUUID: controllerUUID.String(),
		ModelType:      coremodel.IAAS,
		CloudName:      "aws",
		CloudType:      "ec2",
		CloudRegion:    "myregion",
		AgentVersion:   jujuversion.Current,
		MachineCount:   10,
		CoreCount:      10,
		UnitCount:      10,
	}, nil)

	lastConnection := time.Now()
	userUUID := usertesting.GenUserUUID(c)
	s.mockControllerState.EXPECT().GetUserModelSummary(gomock.Any(), userUUID, modelUUID).Return(
		model.UserModelSummary{
			ModelSummary: model.ModelSummary{
				Life: corelife.Alive,
				State: model.ModelState{
					Destroying:                false,
					Migrating:                 false,
					HasInvalidCloudCredential: false,
				},
			},
			UserAccess:         corepermission.AddModelAccess,
			UserLastConnection: &lastConnection,
		}, nil,
	)

	mc := tc.NewMultiChecker()
	mc.AddExpr("_.ModelSummary.Status.Since", tc.Ignore)
	summary, err := svc.GetUserModelSummary(c.Context(), userUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(summary, mc, coremodel.UserModelSummary{
		UserAccess:         corepermission.AddModelAccess,
		UserLastConnection: &lastConnection,
		ModelSummary: coremodel.ModelSummary{
			Name:           "my-awesome-model",
			UUID:           modelUUID,
			ModelType:      coremodel.IAAS,
			Life:           corelife.Alive,
			CloudName:      "aws",
			CloudType:      "ec2",
			CloudRegion:    "myregion",
			ControllerUUID: controllerUUID.String(),
			IsController:   false,
			AgentVersion:   jujuversion.Current,
			Status: corestatus.StatusInfo{
				Status: corestatus.Available,
			},
			MachineCount: 10,
			CoreCount:    10,
			UnitCount:    10,
		},
	})
}

type providerModelServiceSuite struct {
	modelServiceSuite
	mockProvider                      *MockModelResourcesProvider
	mockCloudInfoProvider             *MockCloudInfoProvider
	mockRegionProvider                *MockRegionProvider
	mockStorageProviderRegistryGetter *MockStorageProviderRegistryGetter
	mockStorageProviderRegistry       *MockProviderRegistry
}

func TestProviderModelServiceSuite(t *testing.T) {
	tc.Run(t, &providerModelServiceSuite{})
}

func (s *providerModelServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := s.modelServiceSuite.setupMocks(c)
	s.mockProvider = NewMockModelResourcesProvider(ctrl)
	s.mockCloudInfoProvider = NewMockCloudInfoProvider(ctrl)
	s.mockRegionProvider = NewMockRegionProvider(ctrl)
	s.mockStorageProviderRegistryGetter = NewMockStorageProviderRegistryGetter(ctrl)
	s.mockStorageProviderRegistry = NewMockProviderRegistry(ctrl)

	s.mockStorageProviderRegistryGetter.EXPECT().GetStorageRegistry(
		gomock.Any(),
	).Return(s.mockStorageProviderRegistry, nil).AnyTimes()

	c.Cleanup(func() {
		s.mockCloudInfoProvider = nil
		s.mockProvider = nil
		s.mockRegionProvider = nil
		s.mockStorageProviderRegistryGetter = nil
		s.mockStorageProviderRegistry = nil
	})

	return ctrl
}

func (s *providerModelServiceSuite) providerService(modelUUID coremodel.UUID) *ProviderModelService {
	return NewProviderModelService(
		modelUUID,
		s.mockControllerState,
		s.mockModelState,
		s.environVersionProviderGetter(),
		func(context.Context) (ModelResourcesProvider, error) { return s.mockProvider, nil },
		func(context.Context) (CloudInfoProvider, error) { return s.mockCloudInfoProvider, nil },
		func(context.Context) (RegionProvider, error) { return s.mockRegionProvider, nil },
		s.mockStorageProviderRegistryGetter,
		DefaultAgentBinaryFinder(),
	)
}

// defaultStoragePool represents a default storage pool from a storage provider
// within this testing scope. It can be directly used in go mock expect calls
// where a [model.CreateModelDefaultStoragePoolArg] is expected.
type defaultStoragePool struct {
	Name       string
	Type       string
	Attributes map[string]string
}

// Matches implements the [gomock.Matcher] interface. This type can be mateched
// against a [model.CreateModelDefaultStoragePoolArg] or a slice.
func (d defaultStoragePool) Matches(x any) bool {
	arg, isSingular := x.(model.CreateModelDefaultStoragePoolArg)

	sliceOf, isSliceType := x.([]model.CreateModelDefaultStoragePoolArg)
	if isSliceType {
		if len(sliceOf) != 1 {
			return false
		}
		arg = sliceOf[0]
	} else if !isSingular {
		return false
	}

	if len(d.Attributes) != len(arg.Attributes) {
		return false
	}

	for k, v := range d.Attributes {
		if v != arg.Attributes[k] {
			return false
		}
	}
	return arg.Name == d.Name && arg.Type == d.Type
}

// String describes what this gomock matcher matches. Implements the
// [gomock.Matcher] interface.
func (d defaultStoragePool) String() string {
	return "matches a defaultStoragePool against a single CreatemodelDefaultStoragePoolArg or a slice"
}

// newDefaultStoragePool established a new storage provider with a default
// storage pool in the testing dependencies.
func (s *providerModelServiceSuite) newDefaultStoragePool(
	c *tc.C, ctrl *gomock.Controller,
) defaultStoragePool {
	rval := defaultStoragePool{
		Name: "my-def-pool-A",
		Type: "test1",
		Attributes: map[string]string{
			"attr1": "val1",
		},
	}

	storageAttributes := internalstorage.Attrs{}
	for k, v := range rval.Attributes {
		storageAttributes[k] = v
	}
	defaultPool1, _ := internalstorage.NewConfig(
		rval.Name,
		internalstorage.ProviderType(rval.Type),
		storageAttributes,
	)
	sp1 := NewMockStorageProvider(ctrl)
	s.mockStorageProviderRegistry.EXPECT().StorageProviderTypes().Return(
		[]internalstorage.ProviderType{
			"test1",
		},
		nil,
	)
	s.mockStorageProviderRegistry.EXPECT().StorageProvider(
		internalstorage.ProviderType("test1"),
	).Return(sp1, nil).AnyTimes()

	sp1.EXPECT().DefaultPools().Return([]*internalstorage.Config{defaultPool1})

	return rval
}

func (s *providerModelServiceSuite) TestCreateModel(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	controllerUUID := uuid.MustNewUUID()
	modelUUID := modeltesting.GenModelUUID(c)
	defaultPool := s.newDefaultStoragePool(c, ctrl)
	s.mockControllerState.EXPECT().GetModelSeedInformation(gomock.Any(), gomock.Any()).Return(coremodel.ModelInfo{
		UUID:           modelUUID,
		ControllerUUID: controllerUUID,
		Name:           "my-awesome-model",
		Qualifier:      "prod",
		Cloud:          "aws",
		CloudType:      "ec2",
		CloudRegion:    "myregion",
		Type:           coremodel.IAAS,
	}, nil)
	s.mockModelState.EXPECT().Create(gomock.Any(), model.ModelDetailArgs{
		UUID:               modelUUID,
		ControllerUUID:     controllerUUID,
		Name:               "my-awesome-model",
		Qualifier:          "prod",
		Type:               coremodel.IAAS,
		Cloud:              "aws",
		CloudType:          "ec2",
		CloudRegion:        "myregion",
		AgentStream:        modelagent.AgentStreamReleased,
		AgentVersion:       jujuversion.Current,
		LatestAgentVersion: jujuversion.Current,
	}).Return(nil)
	s.mockModelState.EXPECT().CreateDefaultStoragePools(gomock.Any(), defaultPool).Return(nil)
	s.mockModelState.EXPECT().GetControllerUUID(gomock.Any()).Return(controllerUUID, nil)
	s.mockProvider.EXPECT().ValidateProviderForNewModel(gomock.Any()).Return(nil)
	s.mockProvider.EXPECT().CreateModelResources(gomock.Any(), environs.CreateParams{ControllerUUID: controllerUUID.String()}).Return(nil)

	svc := s.providerService(modelUUID)
	err := svc.CreateModel(c.Context())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *providerModelServiceSuite) TestCreateModelFailedErrorAlreadyExists(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	controllerUUID := uuid.MustNewUUID()
	modelUUID := modeltesting.GenModelUUID(c)
	s.mockControllerState.EXPECT().GetModelSeedInformation(gomock.Any(), gomock.Any()).Return(coremodel.ModelInfo{
		UUID:           modelUUID,
		Name:           "my-awesome-model",
		Qualifier:      "prod",
		ControllerUUID: controllerUUID,
		Cloud:          "aws",
		CloudType:      "ec2",
		CloudRegion:    "myregion",
		Type:           coremodel.IAAS,
	}, nil)
	s.mockModelState.EXPECT().Create(gomock.Any(), model.ModelDetailArgs{
		UUID:               modelUUID,
		ControllerUUID:     controllerUUID,
		Name:               "my-awesome-model",
		Qualifier:          "prod",
		Type:               coremodel.IAAS,
		Cloud:              "aws",
		CloudType:          "ec2",
		CloudRegion:        "myregion",
		AgentStream:        modelagent.AgentStreamReleased,
		AgentVersion:       jujuversion.Current,
		LatestAgentVersion: jujuversion.Current,
	}).Return(modelerrors.AlreadyExists)

	svc := s.providerService(modelUUID)
	err := svc.CreateModel(c.Context())
	c.Assert(err, tc.ErrorIs, modelerrors.AlreadyExists)
}

func (s *providerModelServiceSuite) TestCloudAPIVersion(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)

	s.mockCloudInfoProvider.EXPECT().APIVersion().Return("666", nil)

	svc := s.providerService(modelUUID)
	vers, err := svc.CloudAPIVersion(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(vers, tc.Equals, "666")
}

func (s *providerModelServiceSuite) TestResolveConstraints(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	modelUUID := modeltesting.GenModelUUID(c)

	s.mockModelState.EXPECT().GetModelConstraints(gomock.Any()).Return(constraints.Constraints{
		Arch:      ptr("amd64"),
		Container: ptr(instance.NONE),
		CpuCores:  ptr(uint64(4)),
	}, nil)

	validator := coreconstraints.NewValidator()
	s.mockProvider.EXPECT().ConstraintsValidator(gomock.Any()).Return(validator, nil)

	svc := s.providerService(modelUUID)
	result, err := svc.ResolveConstraints(c.Context(), coreconstraints.Value{
		Arch:      ptr("arm64"),
		Container: ptr(instance.NONE),
		CpuCores:  ptr(uint64(4)),
		Mem:       ptr(uint64(1024)),
		RootDisk:  ptr(uint64(1024)),
	})
	c.Check(err, tc.ErrorIsNil)

	cons := coreconstraints.Value{
		Arch:      ptr("arm64"),
		Container: ptr(instance.NONE),
		CpuCores:  ptr(uint64(4)),
		Mem:       ptr(uint64(1024)),
		RootDisk:  ptr(uint64(1024)),
	}
	c.Check(result, tc.DeepEquals, cons)
}

func (s *providerModelServiceSuite) TestGetModelRegion(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.mockRegionProvider.EXPECT().Region().Return(simplestreams.CloudSpec{Region: "region"}, nil)

	svc := s.providerService(modeltesting.GenModelUUID(c))
	spec, err := svc.GetRegionCloudSpec(c.Context())

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(spec, tc.DeepEquals, simplestreams.CloudSpec{Region: "region"})
}

func (s *providerModelServiceSuite) TestGetModelRegionNotSupported(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	svc := s.providerService(modeltesting.GenModelUUID(c))
	svc.environRegionGetter = func(context.Context) (RegionProvider, error) { return nil, coreerrors.NotSupported }

	spec, err := svc.GetRegionCloudSpec(c.Context())

	c.Assert(err, tc.ErrorIsNil)
	c.Assert(spec, tc.DeepEquals, simplestreams.CloudSpec{})
}

func (s *providerModelServiceSuite) TestSeedDefaultStoragePools(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	defaultPool1, _ := internalstorage.NewConfig(
		"my-def-pool-A", "test1", internalstorage.Attrs{
			"attr1": "val1",
		},
	)
	defaultPool2, _ := internalstorage.NewConfig(
		"my-def-pool-B", "test2", internalstorage.Attrs{
			"attr1": "val1",
		},
	)
	sp1 := NewMockStorageProvider(ctrl)
	sp2 := NewMockStorageProvider(ctrl)
	s.mockStorageProviderRegistry.EXPECT().StorageProviderTypes().Return(
		[]internalstorage.ProviderType{
			"test1",
			"test2",
		},
		nil,
	)
	s.mockStorageProviderRegistry.EXPECT().StorageProvider(
		internalstorage.ProviderType("test1"),
	).Return(sp1, nil)
	s.mockStorageProviderRegistry.EXPECT().StorageProvider(
		internalstorage.ProviderType("test2"),
	).Return(sp2, nil)

	sp1.EXPECT().DefaultPools().Return([]*internalstorage.Config{defaultPool1})
	sp2.EXPECT().DefaultPools().Return([]*internalstorage.Config{defaultPool2})

	s.mockModelState.EXPECT().CreateDefaultStoragePools(
		gomock.Any(),
		gomock.Any(),
	).DoAndReturn(func(
		_ context.Context, args []model.CreateModelDefaultStoragePoolArg) error {

		argChecker := tc.NewMultiChecker().AddExpr("_[_].UUID", tc.Ignore)
		c.Check(args, argChecker, []model.CreateModelDefaultStoragePoolArg{
			{
				Attributes: map[string]string{
					"attr1": "val1",
				},
				Name:   "my-def-pool-A",
				Origin: storage.StoragePoolOriginProviderDefault,
				Type:   "test1",
			},
			{
				Attributes: map[string]string{
					"attr1": "val1",
				},
				Name:   "my-def-pool-B",
				Origin: storage.StoragePoolOriginProviderDefault,
				Type:   "test2",
			},
		})
		return nil
	})

	svc := s.providerService(modeltesting.GenModelUUID(c))
	err := svc.SeedDefaultStoragePools(c.Context())
	c.Check(err, tc.ErrorIsNil)
}
