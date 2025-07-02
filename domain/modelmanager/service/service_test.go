// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"slices"
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/cloud"
	corechangestream "github.com/juju/juju/core/changestream"
	corecredential "github.com/juju/juju/core/credential"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	coreuser "github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	watcher "github.com/juju/juju/core/watcher"
	eventsource "github.com/juju/juju/core/watcher/eventsource"
	accesserrors "github.com/juju/juju/domain/access/errors"
	clouderrors "github.com/juju/juju/domain/cloud/errors"
	credentialerrors "github.com/juju/juju/domain/credential/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/modelmanager"
	modelmanagererrors "github.com/juju/juju/domain/modelmanager/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

// modelChangeEvent is a testing change event to assert behaviour around
// mappers of change events for model uuids.
type modelChangeEvent struct {
	// ModelUUID is the model uuid of the change event.
	ModelUUID coremodel.UUID
}

// serviceSuite is a test suite for the main interface offered by [Service].
type serviceSuite struct {
	mockModelRemover *MockModelRemover
	mockState        *MockState
}

// watchableServiceSuite is a test suite for the interface offered by
// [WatchableService].
type watchableServiceSuite struct {
	mockModelRemover   *MockModelRemover
	mockState          *MockWatchableState
	mockWatcherFactory *MockWatcherFactory
}

// TestServiceSuite runs all of the tests in the [serviceSuite].
func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

// TestWatchableServiceSuite runs all of the tests in the
// [watchableServiceSuite].
func TestWatchableServiceSuite(t *testing.T) {
	tc.Run(t, &watchableServiceSuite{})
}

// Type lets the caller know the type of the change in this event. Hard coded to
// Changed as this not a value that the services tests care about. Implements
// [corechangestream.ChangeEvent].
func (*modelChangeEvent) Type() corechangestream.ChangeType {
	return corechangestream.Changed
}

// Namespace lets the caller know for which namespace this event originated
// from. Hard coded to return a testing namespace for this package. Implements
// [corechangestream.ChangeEvent].
func (*modelChangeEvent) Namespace() string {
	return "modelmanager-service-test"
}

// Changed returns the model uuid set on this event as a string. Implements
// [corechangestream.ChangeEvent].
func (m *modelChangeEvent) Changed() string {
	return m.ModelUUID.String()
}

func (s *serviceSuite) TearDownTest(c *tc.C) {
	s.mockModelRemover = nil
	s.mockState = nil
}

func (s *watchableServiceSuite) TearDownTest(c *tc.C) {
	s.mockModelRemover = nil
	s.mockState = nil
	s.mockWatcherFactory = nil
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockModelRemover = NewMockModelRemover(ctrl)
	s.mockState = NewMockState(ctrl)
	return ctrl
}

func (s *watchableServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockModelRemover = NewMockModelRemover(ctrl)
	s.mockState = NewMockWatchableState(ctrl)
	s.mockWatcherFactory = NewMockWatcherFactory(ctrl)
	return ctrl
}

// TestCheckModelExists tests the CheckModelExists method of the service over
// the happy path.
func (s *serviceSuite) TestCheckModelExists(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(s.mockModelRemover, s.mockState, loggertesting.WrapCheckLog(c))
	modelUUID := modeltesting.GenModelUUID(c)

	s.mockState.EXPECT().CheckModelExists(gomock.Any(), modelUUID).Return(true, nil)
	exists, err := svc.CheckModelExists(c.Context(), modelUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, true)

	s.mockState.EXPECT().CheckModelExists(gomock.Any(), modelUUID).Return(false, nil)
	exists, err = svc.CheckModelExists(c.Context(), modelUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

func (s *serviceSuite) TestCheckModelExistsNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(s.mockModelRemover, s.mockState, loggertesting.WrapCheckLog(c))
	exists, err := svc.CheckModelExists(c.Context(), coremodel.UUID(""))
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
	c.Check(exists, tc.IsFalse)
}

// TestGetControllerModelUUID tests the GetControllerModelUUID method of the
// service returns the controller's model uuid.
func (s *serviceSuite) TestGetControllerModelUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(
		s.mockModelRemover, s.mockState, loggertesting.WrapCheckLog(c),
	)
	modelUUID := modeltesting.GenModelUUID(c)

	s.mockState.EXPECT().GetControllerModelUUID(gomock.Any()).Return(
		modelUUID, nil)
	controllerModelUUID, err := svc.GetControllerModelUUID(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(controllerModelUUID, tc.Equals, modelUUID)
}

// TestGetControllerModelUUIDNotFound tests that if the controller model does
// not exist a [modelerrors.NotFound] error is returned.
func (s *serviceSuite) TestGetControllerModelUUIDNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(
		s.mockModelRemover, s.mockState, loggertesting.WrapCheckLog(c),
	)

	s.mockState.EXPECT().GetControllerModelUUID(gomock.Any()).Return(
		"", modelerrors.NotFound)
	_, err := svc.GetControllerModelUUID(c.Context())
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)
}

// TestCreateModelInvalidArgs tests that if CreateModel is called with invalid
// creation args a [coreerrors.NotValid] error is returned.
func (s *serviceSuite) TestCreateModelInvalidArgs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)
	_, _, err := svc.CreateModel(c.Context(), modelmanager.CreationArgs{})
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestCreateModelInvalidUUID tests that if CreateModel is called and the model
// name supplied is invalid the caller gets back an error that satisfies
// [modelmanagererrors.ModelNameNotValid].
func (s *serviceSuite) TestCreateModelModelNameNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)
	_, _, err := svc.CreateModel(c.Context(), modelmanager.CreationArgs{
		Cloud: "mycloud",
		Owner: usertesting.GenUserUUID(c),

		// Invalid model name
		Name: "$moneymodel",
	})
	c.Check(err, tc.ErrorIs, modelmanagererrors.ModelNameNotValid)
}

// TestCreateModelCloudNotFound tests that when creating a model for the new
// model doesn't exist the caller gets back an error that satisfies
// [clouderrors.NotFound].
//
// There are several places where cloud not found can be returned and all of
// these locations are possible as state may change between operations.
func (s *serviceSuite) TestCreateModelCloudNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)
	userUUID := usertesting.GenUserUUID(c)

	// Test cloud not found from get cloud type call.
	s.mockState.EXPECT().GetCloudType(gomock.Any(), "noexist").Return(
		"", clouderrors.NotFound,
	)
	_, _, err := svc.CreateModel(
		c.Context(),
		modelmanager.CreationArgs{
			Cloud: "noexist",
			Name:  "testmodel",
			Owner: userUUID,
		},
	)
	c.Check(err, tc.ErrorIs, clouderrors.NotFound)

	// Test cloud not found from check cloud supports auth type call.
	s.mockState.EXPECT().GetCloudType(gomock.Any(), "mycloud").Return(
		"ec2", nil,
	)
	s.mockState.EXPECT().CheckCloudSupportsAuthType(
		gomock.Any(), "mycloud", cloud.EmptyAuthType,
	).Return(false, clouderrors.NotFound)

	_, _, err = svc.CreateModel(
		c.Context(),
		modelmanager.CreationArgs{
			Cloud: "mycloud",
			Name:  "testmodel",
			Owner: userUUID,
		},
	)
	c.Check(err, tc.ErrorIs, clouderrors.NotFound)

	// Test cloud not found from create model state call.
	s.mockState.EXPECT().GetCloudType(gomock.Any(), "mycloud").Return(
		"ec2", nil,
	)
	s.mockState.EXPECT().CheckCloudSupportsAuthType(
		gomock.Any(), "mycloud", cloud.EmptyAuthType,
	).Return(true, nil)
	s.mockState.EXPECT().CreateModel(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(clouderrors.NotFound)

	_, _, err = svc.CreateModel(
		c.Context(),
		modelmanager.CreationArgs{
			Cloud: "mycloud",
			Name:  "testmodel",
			Owner: userUUID,
		},
	)
	c.Check(err, tc.ErrorIs, clouderrors.NotFound)
}

// TestCreateModelEmptyCredentialNotSupported tests that if a model is created
// with an empty credential and the cloud does not support empty auth type then
// the caller gets back an error satisfying [modelerrors.CredentialNotValid].
func (s *serviceSuite) TestCreateModelEmptyCredentialNotSupported(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)

	s.mockState.EXPECT().GetCloudType(gomock.Any(), "mycloud").Return(
		"ec2", nil,
	)
	s.mockState.EXPECT().CheckCloudSupportsAuthType(
		gomock.Any(), "mycloud", cloud.EmptyAuthType,
	).Return(false, nil)

	userUUID := usertesting.GenUserUUID(c)
	_, _, err := svc.CreateModel(
		c.Context(),
		modelmanager.CreationArgs{
			Cloud: "mycloud",
			Name:  "testmodel",
			Owner: userUUID,
		},
	)
	c.Check(err, tc.ErrorIs, modelerrors.CredentialNotValid)
}

// TestCreateModelCredentialNotSupported tests the case where the state layer
// for creating the model returns to the caller a
// [modelerrors.CredentialNotValid] error.
func (s *serviceSuite) TestCreateModelCredentialNotSupported(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)

	s.mockState.EXPECT().GetCloudType(gomock.Any(), "mycloud").Return(
		"ec2", nil,
	)
	s.mockState.EXPECT().CreateModel(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(
		modelerrors.CredentialNotValid,
	)

	userUUID := usertesting.GenUserUUID(c)
	userName := usertesting.GenNewName(c, "tlm")
	_, _, err := svc.CreateModel(
		c.Context(),
		modelmanager.CreationArgs{
			Credential: corecredential.Key{
				Cloud: "mycloud",
				Owner: userName,
				Name:  "foocredential",
			},
			Cloud: "mycloud",
			Name:  "testmodel",
			Owner: userUUID,
		},
	)
	c.Check(err, tc.ErrorIs, modelerrors.CredentialNotValid)
}

// TestCreateModelCredentialNotFound tests the case where the state layer
// reports back to the caller an error that satisfies
// [credentialerrors.NotFound].
func (s *serviceSuite) TestCreateModelCredentialNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)

	s.mockState.EXPECT().GetCloudType(gomock.Any(), "mycloud").Return(
		"ec2", nil,
	)
	s.mockState.EXPECT().CreateModel(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(
		credentialerrors.NotFound,
	)

	userUUID := usertesting.GenUserUUID(c)
	userName := usertesting.GenNewName(c, "tlm")
	_, _, err := svc.CreateModel(
		c.Context(),
		modelmanager.CreationArgs{
			Credential: corecredential.Key{
				Cloud: "mycloud",
				Owner: userName,
				Name:  "foocredential",
			},
			Cloud: "mycloud",
			Name:  "testmodel",
			Owner: userUUID,
		},
	)
	c.Check(err, tc.ErrorIs, credentialerrors.NotFound)
}

// TestCreateModelAlreadyExists tests that if a model already exists for the
// same
// name and owner the caller gets back an error that satisfies
// [modelerrors.AlreadyExists].
func (s *serviceSuite) TestCreateModelAlreadyExists(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)

	s.mockState.EXPECT().GetCloudType(gomock.Any(), "mycloud").Return(
		"ec2", nil,
	)
	s.mockState.EXPECT().CreateModel(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(
		modelerrors.AlreadyExists,
	)

	userUUID := usertesting.GenUserUUID(c)
	userName := usertesting.GenNewName(c, "tlm")
	_, _, err := svc.CreateModel(
		c.Context(),
		modelmanager.CreationArgs{
			Credential: corecredential.Key{
				Cloud: "mycloud",
				Owner: userName,
				Name:  "foocredential",
			},
			Cloud: "mycloud",
			Name:  "testmodel",
			Owner: userUUID,
		},
	)
	c.Check(err, tc.ErrorIs, modelerrors.AlreadyExists)
}

// TestCreateModelOwnerNotFound tests the case where a model is created with
// an owner that does not exists. In this case the caller must get back an error
// statisfying [accesserrors.UserNotFound].
func (s *serviceSuite) TestCreateModelOwnerNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)

	s.mockState.EXPECT().GetCloudType(gomock.Any(), "mycloud").Return(
		"ec2", nil,
	)
	s.mockState.EXPECT().CreateModel(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(
		accesserrors.UserNotFound,
	)

	userUUID := usertesting.GenUserUUID(c)
	userName := usertesting.GenNewName(c, "tlm")
	_, _, err := svc.CreateModel(
		c.Context(),
		modelmanager.CreationArgs{
			Credential: corecredential.Key{
				Cloud: "mycloud",
				Owner: userName,
				Name:  "foocredential",
			},
			Cloud: "mycloud",
			Name:  "testmodel",
			Owner: userUUID,
		},
	)
	c.Check(err, tc.ErrorIs, accesserrors.UserNotFound)
}

// TestCreateModelSecretBackendChoiceCAAS tests the default secret backend
// selection based on the proposed new model's type being CAAS.
func (s *serviceSuite) TestCreateModelSecretBackendChoiceCAAS(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)
	userUUID := usertesting.GenUserUUID(c)
	userName := usertesting.GenNewName(c, "tlm")

	s.mockState.EXPECT().GetCloudType(gomock.Any(), "mycloud").Return(
		"kubernetes", nil,
	)
	s.mockState.EXPECT().CreateModel(
		gomock.Any(),
		gomock.Any(),
		coremodel.CAAS,
		modelmanager.CreationArgs{
			Credential: corecredential.Key{
				Cloud: "mycloud",
				Owner: userName,
				Name:  "foocredential",
			},
			Cloud:         "mycloud",
			Name:          "testmodel",
			Owner:         userUUID,
			SecretBackend: "kubernetes",
		},
	).Return(
		nil,
	)

	_, _, err := svc.CreateModel(
		c.Context(),
		modelmanager.CreationArgs{
			Credential: corecredential.Key{
				Cloud: "mycloud",
				Owner: userName,
				Name:  "foocredential",
			},
			Cloud: "mycloud",
			Name:  "testmodel",
			Owner: userUUID,
		},
	)
	c.Check(err, tc.ErrorIsNil)
}

// TestCreateModelSecretBackendChoiceIAAS tests the default secret backend
// selection based on the proposed new model's type being IAAS.
func (s *serviceSuite) TestCreateModelSecretBackendChoiceIAAS(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)
	userUUID := usertesting.GenUserUUID(c)
	userName := usertesting.GenNewName(c, "tlm")

	s.mockState.EXPECT().GetCloudType(gomock.Any(), "mycloud").Return(
		"ec2", nil,
	)
	s.mockState.EXPECT().CreateModel(
		gomock.Any(),
		gomock.Any(),
		coremodel.IAAS,
		modelmanager.CreationArgs{
			Credential: corecredential.Key{
				Cloud: "mycloud",
				Owner: userName,
				Name:  "foocredential",
			},
			Cloud:         "mycloud",
			Name:          "testmodel",
			Owner:         userUUID,
			SecretBackend: "internal",
		},
	).Return(
		nil,
	)

	_, _, err := svc.CreateModel(
		c.Context(),
		modelmanager.CreationArgs{
			Credential: corecredential.Key{
				Cloud: "mycloud",
				Owner: userName,
				Name:  "foocredential",
			},
			Cloud: "mycloud",
			Name:  "testmodel",
			Owner: userUUID,
		},
	)
	c.Check(err, tc.ErrorIsNil)
}

// TestCreateModel tests the happy path of creating a model with
// [Service.CreateModel].
func (s *serviceSuite) TestCreateModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)
	userUUID := usertesting.GenUserUUID(c)
	userName := usertesting.GenNewName(c, "tlm")

	s.mockState.EXPECT().GetCloudType(gomock.Any(), "mycloud").Return(
		"ec2", nil,
	)
	s.mockState.EXPECT().CreateModel(
		gomock.Any(),
		gomock.Any(),
		coremodel.IAAS,
		modelmanager.CreationArgs{
			Credential: corecredential.Key{
				Cloud: "mycloud",
				Owner: userName,
				Name:  "foocredential",
			},
			Cloud:         "mycloud",
			Name:          "testmodel",
			Owner:         userUUID,
			SecretBackend: "internal",
		},
	).Return(
		nil,
	)

	modelUUID, activator, err := svc.CreateModel(
		c.Context(),
		modelmanager.CreationArgs{
			Credential: corecredential.Key{
				Cloud: "mycloud",
				Owner: userName,
				Name:  "foocredential",
			},
			Cloud: "mycloud",
			Name:  "testmodel",
			Owner: userUUID,
		},
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(modelUUID.Validate(), tc.ErrorIsNil)

	s.mockState.EXPECT().ActivateModel(gomock.Any(), modelUUID)
	err = activator(c.Context())
	c.Check(err, tc.ErrorIsNil)
}

// TestGetDefaultModelCloudInfoNotFound tests that when a caller requests the
// default model cloud infomration and the controller model that this
// information is taken from doesn't exist an error satisfying
// [modelerrors.NotFound] is returned.
func (s *serviceSuite) TestGetDefaultModelCloudInfoNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)

	// Test not found error from getting controller model uuid
	s.mockState.EXPECT().GetControllerModelUUID(gomock.Any()).Return(
		coremodel.UUID(""), modelerrors.NotFound,
	)
	_, _, err := svc.GetDefaultModelCloudInfo(c.Context())
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)

	// Test not found error from get model cloud name and region
	modelUUID := modeltesting.GenModelUUID(c)
	s.mockState.EXPECT().GetControllerModelUUID(gomock.Any()).Return(
		modelUUID, nil,
	)
	s.mockState.EXPECT().GetModelCloudNameAndRegion(
		gomock.Any(), modelUUID,
	).Return("", "", modelerrors.NotFound)
	_, _, err = svc.GetDefaultModelCloudInfo(c.Context())
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)
}

// TestGetDefaultModelCloudInfo tests the happy path of
// [Service.GetDefaultModelCloudInfo].
func (s *serviceSuite) TestGetDefaultModelCloudInfo(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)

	modelUUID := modeltesting.GenModelUUID(c)
	cloudName := "mycloud"
	region := "us-east-1"

	s.mockState.EXPECT().GetControllerModelUUID(gomock.Any()).Return(
		modelUUID, nil,
	)
	s.mockState.EXPECT().GetModelCloudNameAndRegion(
		gomock.Any(), modelUUID,
	).Return(cloudName, region, nil)

	name, region, err := svc.GetDefaultModelCloudInfo(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(name, tc.Equals, cloudName)
	c.Check(region, tc.Equals, region)
}

// TestGetModelUUIDForNameAndOwnerUserNotValid tests that if a request is made
// for the model uuid based on name and owner and the user name isn't valid. The
// caller gets back an error satisfying [coreerrors.NotValid].
func (s *serviceSuite) TestGetModelUUIDForNameAndOwnerUserNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)

	modelName := "testmodel"

	_, err := svc.GetModelUUIDForNameAndOwner(
		c.Context(), modelName, coreuser.Name{},
	)
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestGetModelUUIDForNameAndOwnerInvalidModelName tests that if a request is
// made for a model uuid based on name and owner and the model name is not
// valid. The caller gets back an error satisfying
// [modelmanagererrors.ModelNameNotValid].
func (s *serviceSuite) TestGetModelUUIDForNameAndOwnerModelNameNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)

	_, err := svc.GetModelUUIDForNameAndOwner(
		c.Context(),
		"$moneyModel",
		usertesting.GenNewName(c, "tlm"),
	)
	c.Check(err, tc.ErrorIs, modelmanagererrors.ModelNameNotValid)
}

// TestGetModelUUIDForNameAndOwnerNotFoud tests that if a request is made for a
// model uuid based on name and owner and no model exists for this tuple the
// caller gets back an error satisfying [modelerrors.NotFound].
func (s *serviceSuite) TestGetModelUUIDForNameAndOwnerNotFoud(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)

	modelName := "testmodel"
	userName := usertesting.GenNewName(c, "tlm")

	s.mockState.EXPECT().GetModelUUIDForNameAndOwner(
		gomock.Any(), modelName, userName,
	).Return("", modelerrors.NotFound)

	_, err := svc.GetModelUUIDForNameAndOwner(
		c.Context(), modelName, userName,
	)
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)
}

// TestGetModelUUIDForNameAndOwnerUserNotFound tests that if a request is made
// for a model uuid based on name and owner and the user does not exist. The
// caller will receive an error that satisfies [accesserrors.UserNotFound].
func (s *serviceSuite) TestGetModelUUIDForNameAndOwnerUserNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)

	modelName := "testmodel"
	userName := usertesting.GenNewName(c, "tlm")

	s.mockState.EXPECT().GetModelUUIDForNameAndOwner(
		gomock.Any(), modelName, userName,
	).Return("", accesserrors.UserNotFound)

	_, err := svc.GetModelUUIDForNameAndOwner(
		c.Context(), modelName, userName,
	)
	c.Check(err, tc.ErrorIs, accesserrors.UserNotFound)
}

// TestImportModelInvalidModelUUID tests that if [service.ImportModel] is called
// with an invalid model uuid a [coreerrors.NotValid] error is returned.
func (s *serviceSuite) TestImportModelInvalidModelUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)
	_, err := svc.ImportModel(c.Context(), modelmanager.ImportArgs{
		UUID: coremodel.UUID(""),
		CreationArgs: modelmanager.CreationArgs{
			Cloud:       "mrcloud",
			CloudRegion: "",
			Name:        "testmodel",
			Owner:       usertesting.GenUserUUID(c),
		},
	})
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestImportModelModelNameNotValid tests that if ImportModel is called and the
// model name supplied is invalid the caller gets back an error that satisfies
// [modelmanagererrors.ModelNameNotValid].
func (s *serviceSuite) TestImportModelModelNameNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)
	_, err := svc.ImportModel(c.Context(), modelmanager.ImportArgs{
		UUID: modeltesting.GenModelUUID(c),
		CreationArgs: modelmanager.CreationArgs{
			Cloud: "mycloud",
			Owner: usertesting.GenUserUUID(c),

			// Invalid model name
			Name: "$moneymodel",
		},
	})
	c.Check(err, tc.ErrorIs, modelmanagererrors.ModelNameNotValid)
}

// TestImportModelInvalidArgs tests that if [service.ImportModel] is called with
// invalid creation args a [coreerrors.NotValid] error is returned.
func (s *serviceSuite) TestImportModelInvalidArgs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)
	_, err := svc.ImportModel(c.Context(), modelmanager.ImportArgs{
		UUID: modeltesting.GenModelUUID(c),
	})
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestImportModelCloudNotFound tests that when importing a model and the cloud
// doesn't exist the caller gets back an error that satisfies
// [clouderrors.NotFound].
//
// There are several places where cloud not found can be returned and all of
// these locations are possible as state may change between operations.
func (s *serviceSuite) TestImportModelCloudNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)
	userUUID := usertesting.GenUserUUID(c)

	// Test cloud not found from get cloud type call.
	s.mockState.EXPECT().GetCloudType(gomock.Any(), "noexist").Return(
		"", clouderrors.NotFound,
	)
	_, err := svc.ImportModel(
		c.Context(),
		modelmanager.ImportArgs{
			UUID: modeltesting.GenModelUUID(c),
			CreationArgs: modelmanager.CreationArgs{
				Cloud: "noexist",
				Name:  "testmodel",
				Owner: userUUID,
			},
		},
	)
	c.Check(err, tc.ErrorIs, clouderrors.NotFound)

	// Test cloud not found from check cloud supports auth type call.
	s.mockState.EXPECT().GetCloudType(gomock.Any(), "mycloud").Return(
		"ec2", nil,
	)
	s.mockState.EXPECT().CheckCloudSupportsAuthType(
		gomock.Any(), "mycloud", cloud.EmptyAuthType,
	).Return(false, clouderrors.NotFound)

	_, err = svc.ImportModel(
		c.Context(),
		modelmanager.ImportArgs{
			UUID: modeltesting.GenModelUUID(c),
			CreationArgs: modelmanager.CreationArgs{
				Cloud: "mycloud",
				Name:  "testmodel",
				Owner: userUUID,
			},
		},
	)
	c.Check(err, tc.ErrorIs, clouderrors.NotFound)

	// Test cloud not found from create model state call.
	s.mockState.EXPECT().GetCloudType(gomock.Any(), "mycloud").Return(
		"ec2", nil,
	)
	s.mockState.EXPECT().CheckCloudSupportsAuthType(
		gomock.Any(), "mycloud", cloud.EmptyAuthType,
	).Return(true, nil)
	s.mockState.EXPECT().CreateModel(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(clouderrors.NotFound)

	_, err = svc.ImportModel(
		c.Context(),
		modelmanager.ImportArgs{
			UUID: modeltesting.GenModelUUID(c),
			CreationArgs: modelmanager.CreationArgs{
				Cloud: "mycloud",
				Name:  "testmodel",
				Owner: userUUID,
			},
		},
	)
	c.Check(err, tc.ErrorIs, clouderrors.NotFound)
}

// TestImportModelEmptyCredentialNotSupported tests that if a model is imported
// with an empty credential and the cloud does not support empty auth type then
// the caller gets back an error satisfying [modelerrors.CredentialNotValid].
func (s *serviceSuite) TestImportModelEmptyCredentialNotSupported(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)

	s.mockState.EXPECT().GetCloudType(gomock.Any(), "mycloud").Return(
		"ec2", nil,
	)
	s.mockState.EXPECT().CheckCloudSupportsAuthType(
		gomock.Any(), "mycloud", cloud.EmptyAuthType,
	).Return(false, nil)

	userUUID := usertesting.GenUserUUID(c)
	_, err := svc.ImportModel(
		c.Context(),
		modelmanager.ImportArgs{
			UUID: modeltesting.GenModelUUID(c),
			CreationArgs: modelmanager.CreationArgs{
				Cloud: "mycloud",
				Name:  "testmodel",
				Owner: userUUID,
			},
		},
	)
	c.Check(err, tc.ErrorIs, modelerrors.CredentialNotValid)
}

// TestImportModelCredentialNotSupported tests the case where the state layer
// for importing the model returns to the caller a
// [modelerrors.CredentialNotValid] error.
func (s *serviceSuite) TestImportModelCredentialNotSupported(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)

	s.mockState.EXPECT().GetCloudType(gomock.Any(), "mycloud").Return(
		"ec2", nil,
	)
	s.mockState.EXPECT().CreateModel(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(
		modelerrors.CredentialNotValid,
	)

	userUUID := usertesting.GenUserUUID(c)
	userName := usertesting.GenNewName(c, "tlm")
	_, err := svc.ImportModel(
		c.Context(),
		modelmanager.ImportArgs{
			UUID: modeltesting.GenModelUUID(c),
			CreationArgs: modelmanager.CreationArgs{
				Cloud: "mycloud",
				Credential: corecredential.Key{
					Cloud: "mycloud",
					Owner: userName,
					Name:  "foocredential",
				},
				Name:  "testmodel",
				Owner: userUUID,
			},
		},
	)
	c.Check(err, tc.ErrorIs, modelerrors.CredentialNotValid)
}

// TestImportModelCredentialNotFound tests the case where the state layer
// reports back to the caller an error that satisfies
// [credentialerrors.NotFound].
func (s *serviceSuite) TestImportModelCredentialNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)

	s.mockState.EXPECT().GetCloudType(gomock.Any(), "mycloud").Return(
		"ec2", nil,
	)
	s.mockState.EXPECT().CreateModel(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(
		credentialerrors.NotFound,
	)

	userUUID := usertesting.GenUserUUID(c)
	userName := usertesting.GenNewName(c, "tlm")
	_, err := svc.ImportModel(
		c.Context(),
		modelmanager.ImportArgs{
			UUID: modeltesting.GenModelUUID(c),
			CreationArgs: modelmanager.CreationArgs{
				Cloud: "mycloud",
				Credential: corecredential.Key{
					Cloud: "mycloud",
					Owner: userName,
					Name:  "foocredential",
				},
				Name:  "testmodel",
				Owner: userUUID,
			},
		},
	)
	c.Check(err, tc.ErrorIs, credentialerrors.NotFound)
}

// TestImportModelAlreadyExists tests that if a model already exists for the
// same name and owner the caller gets back an error that satisfies
// [modelerrors.AlreadyExists].
func (s *serviceSuite) TestImportModelAlreadyExists(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)

	s.mockState.EXPECT().GetCloudType(gomock.Any(), "mycloud").Return(
		"ec2", nil,
	)
	s.mockState.EXPECT().CreateModel(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(
		modelerrors.AlreadyExists,
	)

	userUUID := usertesting.GenUserUUID(c)
	userName := usertesting.GenNewName(c, "tlm")
	_, err := svc.ImportModel(
		c.Context(),
		modelmanager.ImportArgs{
			UUID: modeltesting.GenModelUUID(c),
			CreationArgs: modelmanager.CreationArgs{
				Cloud: "mycloud",
				Credential: corecredential.Key{
					Cloud: "mycloud",
					Owner: userName,
					Name:  "foocredential",
				},
				Name:  "testmodel",
				Owner: userUUID,
			},
		},
	)
	c.Check(err, tc.ErrorIs, modelerrors.AlreadyExists)
}

// TestImportModelOwnerNotFound tests the case where a model is imported with
// an owner that does not exists. In this case the caller must get back an error
// statisfying [accesserrors.UserNotFound].
func (s *serviceSuite) TestImportModelOwnerNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)

	s.mockState.EXPECT().GetCloudType(gomock.Any(), "mycloud").Return(
		"ec2", nil,
	)
	s.mockState.EXPECT().CreateModel(
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
		gomock.Any(),
	).Return(
		accesserrors.UserNotFound,
	)

	userUUID := usertesting.GenUserUUID(c)
	userName := usertesting.GenNewName(c, "tlm")
	_, err := svc.ImportModel(
		c.Context(),
		modelmanager.ImportArgs{
			UUID: modeltesting.GenModelUUID(c),
			CreationArgs: modelmanager.CreationArgs{
				Cloud: "mycloud",
				Credential: corecredential.Key{
					Cloud: "mycloud",
					Owner: userName,
					Name:  "foocredential",
				},
				Name:  "testmodel",
				Owner: userUUID,
			},
		},
	)
	c.Check(err, tc.ErrorIs, accesserrors.UserNotFound)
}

// TestImportModelSecretBackendChoiceCAAS tests the default secret backend
// selection based on the imported model's type being CAAS.
func (s *serviceSuite) TestImportModelSecretBackendChoiceCAAS(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)
	userUUID := usertesting.GenUserUUID(c)
	userName := usertesting.GenNewName(c, "tlm")

	s.mockState.EXPECT().GetCloudType(gomock.Any(), "mycloud").Return(
		"kubernetes", nil,
	)
	s.mockState.EXPECT().CreateModel(
		gomock.Any(),
		gomock.Any(),
		coremodel.CAAS,
		modelmanager.CreationArgs{
			Credential: corecredential.Key{
				Cloud: "mycloud",
				Owner: userName,
				Name:  "foocredential",
			},
			Cloud:         "mycloud",
			Name:          "testmodel",
			Owner:         userUUID,
			SecretBackend: "kubernetes",
		},
	).Return(
		nil,
	)

	_, err := svc.ImportModel(
		c.Context(),
		modelmanager.ImportArgs{
			UUID: modeltesting.GenModelUUID(c),
			CreationArgs: modelmanager.CreationArgs{
				Cloud: "mycloud",
				Credential: corecredential.Key{
					Cloud: "mycloud",
					Owner: userName,
					Name:  "foocredential",
				},
				Name:  "testmodel",
				Owner: userUUID,
			},
		},
	)
	c.Check(err, tc.ErrorIsNil)
}

// TestImportModelSecretBackendChoiceIAAS tests the default secret backend
// selection based on the imported new model's type being IAAS.
func (s *serviceSuite) TestImportModelSecretBackendChoiceIAAS(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)
	userUUID := usertesting.GenUserUUID(c)
	userName := usertesting.GenNewName(c, "tlm")

	s.mockState.EXPECT().GetCloudType(gomock.Any(), "mycloud").Return(
		"ec2", nil,
	)
	s.mockState.EXPECT().CreateModel(
		gomock.Any(),
		gomock.Any(),
		coremodel.IAAS,
		modelmanager.CreationArgs{
			Credential: corecredential.Key{
				Cloud: "mycloud",
				Owner: userName,
				Name:  "foocredential",
			},
			Cloud:         "mycloud",
			Name:          "testmodel",
			Owner:         userUUID,
			SecretBackend: "internal",
		},
	).Return(
		nil,
	)

	_, err := svc.ImportModel(
		c.Context(),
		modelmanager.ImportArgs{
			UUID: modeltesting.GenModelUUID(c),
			CreationArgs: modelmanager.CreationArgs{
				Cloud: "mycloud",
				Credential: corecredential.Key{
					Cloud: "mycloud",
					Owner: userName,
					Name:  "foocredential",
				},
				Name:  "testmodel",
				Owner: userUUID,
			},
		},
	)
	c.Check(err, tc.ErrorIsNil)
}

// TestImportModel tests the happy path of importing a model with
// [Service.ImportModel].
func (s *serviceSuite) TestImportModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)
	userUUID := usertesting.GenUserUUID(c)
	userName := usertesting.GenNewName(c, "tlm")
	modelUUID := modeltesting.GenModelUUID(c)

	s.mockState.EXPECT().GetCloudType(gomock.Any(), "mycloud").Return(
		"ec2", nil,
	)
	s.mockState.EXPECT().CreateModel(
		gomock.Any(),
		modelUUID,
		coremodel.IAAS,
		modelmanager.CreationArgs{
			Credential: corecredential.Key{
				Cloud: "mycloud",
				Owner: userName,
				Name:  "foocredential",
			},
			Cloud:         "mycloud",
			Name:          "testmodel",
			Owner:         userUUID,
			SecretBackend: "internal",
		},
	).Return(
		nil,
	)

	activator, err := svc.ImportModel(
		c.Context(),
		modelmanager.ImportArgs{
			UUID: modelUUID,
			CreationArgs: modelmanager.CreationArgs{
				Credential: corecredential.Key{
					Cloud: "mycloud",
					Owner: userName,
					Name:  "foocredential",
				},
				Cloud: "mycloud",
				Name:  "testmodel",
				Owner: userUUID,
			},
		},
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(modelUUID.Validate(), tc.ErrorIsNil)

	s.mockState.EXPECT().ActivateModel(gomock.Any(), modelUUID)
	err = activator(c.Context())
	c.Check(err, tc.ErrorIsNil)
}

// TestListModelUUIDs is testing the happy path of listing all of the model
// uuids in the controller that are active.
func (s *serviceSuite) TestListModelUUIDs(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)

	modelUUID1 := modeltesting.GenModelUUID(c)
	modelUUID2 := modeltesting.GenModelUUID(c)
	modelUUID3 := modeltesting.GenModelUUID(c)
	s.mockState.EXPECT().ListModelUUIDs(gomock.Any()).Return(
		[]coremodel.UUID{
			// Purposely out of order
			modelUUID2, modelUUID3, modelUUID1,
		}, nil,
	)

	list, err := svc.ListModelUUIDs(c.Context())
	c.Check(err, tc.ErrorIsNil)

	expect := []coremodel.UUID{modelUUID1, modelUUID2, modelUUID3}
	c.Check(list, tc.SameContents, expect)
}

// TestListModelUUIDsEmpty is testing [service.ListModelUUIDs] that when no
// models exist in the controller the caller gets back an empty list.
func (s *serviceSuite) TestListModelUUIDsEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)

	s.mockState.EXPECT().ListModelUUIDs(gomock.Any()).Return(
		[]coremodel.UUID{}, nil,
	)

	list, err := svc.ListModelUUIDs(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(list, tc.HasLen, 0)
}

// TestListModelUUIDsForUserInvalidUser tests that when requesting the model
// uuids that a user has access for and supplying and invalid user uuid. The
// caller will get back an error satisfying [coreerrors.NotValid].
func (s *serviceSuite) TestListModelUUIDsForUserInvalidUser(c *tc.C) {
	defer s.setupMocks(c).Finish()
	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)

	_, err := svc.ListModelUUIDsForUser(c.Context(), coreuser.UUID("123"))
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestListModelUUIDsForUserNotFound tests that when requesting the model uuids
// that a user has access for and the user does not exist. The caller will get
// back an error statisfying [accesserrors.UserNotFound].
func (s *serviceSuite) TestListModelUUIDsForUserNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)

	userUUID := usertesting.GenUserUUID(c)
	s.mockState.EXPECT().ListModelUUIDsForUser(
		gomock.Any(), userUUID,
	).Return(nil, accesserrors.UserNotFound)

	_, err := svc.ListModelUUIDsForUser(c.Context(), userUUID)
	c.Check(err, tc.ErrorIs, accesserrors.UserNotFound)
}

// TestListModelUUIDsForUser is a happy path test for
// [Service.ListModelUUIDsForUser].
func (s *serviceSuite) TestListModelUUIDsForUser(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)

	userUUID := usertesting.GenUserUUID(c)
	modelUUID1 := modeltesting.GenModelUUID(c)
	modelUUID2 := modeltesting.GenModelUUID(c)
	modelUUID3 := modeltesting.GenModelUUID(c)
	s.mockState.EXPECT().ListModelUUIDsForUser(
		gomock.Any(), userUUID,
	).Return(
		[]coremodel.UUID{
			// Purposely out of order
			modelUUID2, modelUUID3, modelUUID1,
		}, nil,
	)

	list, err := svc.ListModelUUIDsForUser(c.Context(), userUUID)
	c.Check(err, tc.ErrorIsNil)

	expect := []coremodel.UUID{modelUUID1, modelUUID2, modelUUID3}
	c.Check(list, tc.SameContents, expect)
}

// TestRemoveNonActivateModelNotValid tests that if a request is made to remove
// a model for an invalid model uuid the caller gets back an error satisfying
// [coreerrors.NotValid].
func (s *serviceSuite) TestRemoveNonActivateModelNotValid(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)

	err := svc.RemoveNonActivatedModel(c.Context(), coremodel.UUID(""))
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestRemoveNonActivatedModelNotFound tests that if a request is made to remove
// a non active model that doesn't exist the caller gets back an error
// satisfying [modelerrors.NotFound].
func (s *serviceSuite) TestRemoveNonActivatedModelNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)
	modelUUID := modeltesting.GenModelUUID(c)

	s.mockState.EXPECT().RemoveNonActivatedModel(gomock.Any(), modelUUID).Return(
		modelerrors.NotFound,
	)
	err := svc.RemoveNonActivatedModel(c.Context(), modelUUID)
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)
}

// TestRemoveAlreadyActivatedModel tests that if a caller asks for a non
// activated model to be removed and the model has already been active the
// caller gets back an error satisfying [modelmanagererrors.AlreadyActivated].
func (s *serviceSuite) TestRemoveAlreadyActivatedModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)
	modelUUID := modeltesting.GenModelUUID(c)

	s.mockState.EXPECT().RemoveNonActivatedModel(gomock.Any(), modelUUID).Return(
		modelmanagererrors.AlreadyActivated,
	)
	err := svc.RemoveNonActivatedModel(c.Context(), modelUUID)
	c.Check(err, tc.ErrorIs, modelmanagererrors.AlreadyActivated)
}

// TestRemoveNonActivatedModel is testing removing a non activated model with
// the option of having the model db deleted as part of the operation.
func (s *serviceSuite) TestRemoveNonActivatedModelWithDeleteDB(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)
	modelUUID := modeltesting.GenModelUUID(c)

	s.mockState.EXPECT().RemoveNonActivatedModel(gomock.Any(), modelUUID).Return(
		nil,
	)
	s.mockModelRemover.EXPECT().DeleteDB(modelUUID).Return(nil)
	err := svc.RemoveNonActivatedModel(
		c.Context(), modelUUID, modelmanager.WithDeleteDB(),
	)
	c.Check(err, tc.ErrorIsNil)
}

// TestRemoveNonActivatedModelWithNoDeleteDB is testing removing a non activated
// model and that the model db is not deleted as part of the operation.
func (s *serviceSuite) TestRemoveNonActivatedModelWithNoDeleteDB(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockModelRemover,
		s.mockState,
		loggertesting.WrapCheckLog(c),
	)
	modelUUID := modeltesting.GenModelUUID(c)

	s.mockState.EXPECT().RemoveNonActivatedModel(gomock.Any(), modelUUID).Return(
		nil,
	)
	err := svc.RemoveNonActivatedModel(
		c.Context(),
		modelUUID,
		modelmanager.WithoutDeleteDB(),
	)
	c.Check(err, tc.ErrorIsNil)
}

// TestWatchActiveModelsMapperMaintainsOrder is a test to assert the behaviour
// of the mapper functionality behind [WatchableService.WatchActivatedModels].
// In this test we want to see that change events are filtered to just those
// models in the controller that are active and also that the order of change
// events is maintained after filtering.
//
// It is also important to see in this test that change events are not
// re-written to another value implementing [corechangestream.ChangeEvent].
//
// This test exists because we know that the
// [WatchableState.IdentifyActiveModelsFromList] won't maintain the order of
// model uuids supplied.
func (s *watchableServiceSuite) TestWatchActiveModelsMapperMaintainsOrder(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// providedMapper exists to capture the mapper registered with the watcher.
	// It is better if this test doesn't assume to much about implementation.
	var providedMapper eventsource.Mapper
	s.mockWatcherFactory.EXPECT().NewNamespaceMapperWatcher(
		gomock.Any(), gomock.Any(), gomock.Any(),
	).DoAndReturn(
		func(_ eventsource.NamespaceQuery,
			mapper eventsource.Mapper,
			_ eventsource.FilterOption,
			_ ...eventsource.FilterOption,
		) (watcher.Watcher[[]string], error) {
			providedMapper = mapper
			return nil, nil
		})
	changedModelUUIDs := []coremodel.UUID{
		modeltesting.GenModelUUID(c),
		modeltesting.GenModelUUID(c),
		modeltesting.GenModelUUID(c),
		modeltesting.GenModelUUID(c),
	}
	events := make([]corechangestream.ChangeEvent, 0, len(changedModelUUIDs))
	for _, uuid := range changedModelUUIDs {
		events = append(events, &modelChangeEvent{uuid})
	}
	s.mockState.EXPECT().InitialWatchActivatedModelsStatement().Return("", "")
	s.mockState.EXPECT().IdentifyActiveModelsFromList(
		gomock.Any(), gomock.Any(),
	).DoAndReturn(
		func(_ context.Context, uuids []coremodel.UUID) ([]coremodel.UUID, error) {
			// Purposely drop the first model uuid as to indicate that this
			// uuid is not yet active in the model. Reverse the order so we can
			// prove order is maintained by the caller.
			rval := uuids[1:]
			slices.Reverse(rval)
			return rval, nil
		},
	)
	expectedEvents := events[1:]

	svc := NewWatchableService(
		s.mockModelRemover,
		s.mockState,
		s.mockWatcherFactory,
		loggertesting.WrapCheckLog(c),
	)
	_, err := svc.WatchActivatedModels(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	filteredEvents, err := providedMapper(c.Context(), events)
	c.Check(err, tc.ErrorIsNil)
	c.Check(filteredEvents, tc.DeepEquals, expectedEvents)
}
