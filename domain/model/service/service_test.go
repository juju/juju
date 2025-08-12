// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/transform"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/changestream"
	changestreammock "github.com/juju/juju/core/changestream/mocks"
	cloudtesting "github.com/juju/juju/core/cloud/testing"
	"github.com/juju/juju/core/credential"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/life"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/core/watcher/watchertest"
	accesserrors "github.com/juju/juju/domain/access/errors"
	domainlife "github.com/juju/juju/domain/life"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	jujusecrets "github.com/juju/juju/internal/secrets/provider/juju"
	kubernetessecrets "github.com/juju/juju/internal/secrets/provider/kubernetes"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

type serviceSuite struct {
	testhelpers.IsolationSuite

	userUUID user.UUID
	state    *dummyState
	deleter  *dummyDeleter

	mockModelDeleter   *MockModelDeleter
	mockState          *MockState
	mockWatcherFactory *MockWatcherFactory
	mockStringsWatcher *MockStringsWatcher[[]string]
	changestreamtesting.ControllerSuite
}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) SetUpTest(c *tc.C) {
	var err error
	s.userUUID = usertesting.GenUserUUID(c)
	c.Assert(err, tc.ErrorIsNil)
	s.state = &dummyState{
		clouds:             map[string]dummyStateCloud{},
		models:             map[coremodel.UUID]coremodel.Model{},
		nonActivatedModels: map[coremodel.UUID]coremodel.Model{},
		users: map[user.UUID]user.Name{
			s.userUUID: user.AdminUserName,
		},
		secretBackends: []string{
			jujusecrets.BackendName,
			kubernetessecrets.BackendName,
		},
	}
	s.deleter = &dummyDeleter{
		deleted: map[string]struct{}{},
	}

	s.setupControllerModel(c)
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockModelDeleter = NewMockModelDeleter(ctrl)
	s.mockState = NewMockState(ctrl)
	s.mockWatcherFactory = NewMockWatcherFactory(ctrl)
	s.mockStringsWatcher = NewMockStringsWatcher[[]string](ctrl)

	return ctrl
}

func (s *serviceSuite) newStubService(c *tc.C) *Service {
	return NewService(
		s.state,
		newStatusHistoryGetter(c),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
}

func (s *serviceSuite) newService(c *tc.C) *Service {
	return NewService(
		s.mockState,
		newStatusHistoryGetter(c),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
}

func (s *serviceSuite) newWatchableService(c *tc.C) *WatchableService {
	return NewWatchableService(
		s.mockState,
		s.mockWatcherFactory,
		newStatusHistoryGetter(c),
		clock.WallClock,
		loggertesting.WrapCheckLog(c),
	)
}

func (s *serviceSuite) setupControllerModel(c *tc.C) {
	adminUUID := usertesting.GenUserUUID(c)
	s.state.users[adminUUID] = coremodel.ControllerModelOwnerUsername

	cred := credential.Key{
		Cloud: "controller-cloud",
		Name:  "controller-cloud-cred",
		Owner: usertesting.GenNewName(c, "owner"),
	}
	s.state.clouds["controller-cloud"] = dummyStateCloud{
		Credentials: map[string]credential.Key{
			cred.String(): cred,
		},
		Regions: []string{"ap-southeast-2"},
	}

	svc := s.newStubService(c)
	modelID, activator, err := svc.CreateModel(c.Context(), model.GlobalModelCreationArgs{
		Cloud:       "controller-cloud",
		CloudRegion: "ap-southeast-2",
		Credential:  cred,
		AdminUsers:  []user.UUID{adminUUID},
		Name:        coremodel.ControllerModelName,
		Qualifier:   "prod",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(activator(c.Context()), tc.ErrorIsNil)
	s.state.controllerModelUUID = modelID
}

// TestControllerModelNameChange is here to make the breaker of this test stop
// and think. There exists business logic in this package that is very dependent
// on the well known value defined in [coremodel.ControllerModelName]. If this
// test has broken it means this value has changed and you could be at risk of
// breaking Juju. Please consider the business logic in this package and if
// changing this well known value is handled correctly for both legacy and
// future Juju versions!!!
func (s *serviceSuite) TestControllerModelNameChange(c *tc.C) {
	c.Assert(coremodel.ControllerModelName, tc.Equals, "controller")
}

// TestControllerModelOwnerUsername is here to make the breaker of this test
// stop and think. There exists business logic in this package that is very
// dependent on the well known value defined in
// [coremodel.ControllerModelOwnerUsername]. If this test has broken it means
// this value has changed and you could be at risk of breaking Juju. Please
// consider the business logic in this package and if changing this well known
// value is handled correctly for both legacy and future Juju versions!!!
func (s *serviceSuite) TestControllerModelOwnerUsername(c *tc.C) {
	c.Assert(coremodel.ControllerModelOwnerUsername, tc.Equals, usertesting.GenNewName(c, "admin"))
}

func (s *serviceSuite) TestCreateModelInvalidArgs(c *tc.C) {
	svc := s.newStubService(c)
	_, _, err := svc.CreateModel(c.Context(), model.GlobalModelCreationArgs{})
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *serviceSuite) TestModelCreation(c *tc.C) {
	cred := credential.Key{
		Cloud: "aws",
		Name:  "foobar",
		Owner: usertesting.GenNewName(c, "test-user"),
	}
	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.Key{
			cred.String(): cred,
		},
		Regions: []string{"myregion"},
	}

	svc := s.newStubService(c)
	id, activator, err := svc.CreateModel(c.Context(), model.GlobalModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Credential:  cred,
		AdminUsers:  []user.UUID{s.userUUID},
		Name:        "my-awesome-model",
		Qualifier:   "prod",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(activator(c.Context()), tc.ErrorIsNil)

	exists, err := svc.CheckModelExists(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(exists, tc.IsTrue)

	modelList, err := svc.ListModelUUIDs(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(len(modelList), tc.Equals, 2)
}

func (s *serviceSuite) TestCheckExistsNoModel(c *tc.C) {
	svc := s.newStubService(c)
	id := modeltesting.GenModelUUID(c)
	exists, err := svc.CheckModelExists(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(exists, tc.IsFalse)
}

// TestModelCreationSecretBackendNotFound is asserting that if we try and add a
// model and define a secret backend for the new model that doesn't exist we get
// back a [secretbackenderrors.NotFound] error.
func (s *serviceSuite) TestModelCreationSecretBackendNotFound(c *tc.C) {
	cred := credential.Key{
		Cloud: "aws",
		Name:  "foobar",
		Owner: usertesting.GenNewName(c, "owner"),
	}
	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.Key{
			cred.String(): cred,
		},
		Regions: []string{"myregion"},
	}

	svc := s.newStubService(c)
	_, _, err := svc.CreateModel(c.Context(), model.GlobalModelCreationArgs{
		Cloud:         "aws",
		CloudRegion:   "myregion",
		Credential:    cred,
		AdminUsers:    []user.UUID{s.userUUID},
		Name:          "my-awesome-model",
		Qualifier:     "prod",
		SecretBackend: "no-exist",
	})

	c.Assert(err, tc.ErrorIs, secretbackenderrors.NotFound)
}

func (s *serviceSuite) TestModelCreationInvalidCloud(c *tc.C) {
	s.state.clouds["aws"] = dummyStateCloud{}
	svc := s.newStubService(c)
	_, _, err := svc.CreateModel(c.Context(), model.GlobalModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		AdminUsers:  []user.UUID{s.userUUID},
		Name:        "my-awesome-model",
		Qualifier:   "prod",
	})

	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)
}

func (s *serviceSuite) TestModelCreationNoCloudRegion(c *tc.C) {
	s.state.clouds["aws"] = dummyStateCloud{
		Regions: []string{"myregion"},
	}

	svc := s.newStubService(c)
	_, _, err := svc.CreateModel(c.Context(), model.GlobalModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "noexist",
		AdminUsers:  []user.UUID{s.userUUID},
		Name:        "my-awesome-model",
		Qualifier:   "prod",
	})

	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)
}

// TestModelCreationOwnerNotFound is testing that if we make a model with an
// owner that doesn't exist we get back a [accesserrors.NotFound] error.
func (s *serviceSuite) TestModelCreationOwnerNotFound(c *tc.C) {
	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.Key{},
		Regions:     []string{"myregion"},
	}

	notFoundUser, err := user.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	svc := s.newStubService(c)
	_, _, err = svc.CreateModel(c.Context(), model.GlobalModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		AdminUsers:  []user.UUID{notFoundUser},
		Name:        "my-awesome-model",
		Qualifier:   "prod",
	})

	c.Assert(err, tc.ErrorIs, accesserrors.UserNotFound)
}

func (s *serviceSuite) TestModelCreationNoCloudCredential(c *tc.C) {
	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.Key{},
		Regions:     []string{"myregion"},
	}

	svc := s.newStubService(c)
	_, _, err := svc.CreateModel(c.Context(), model.GlobalModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Credential: credential.Key{
			Cloud: "aws",
			Name:  "foo",
			Owner: usertesting.GenNewName(c, "owner"),
		},
		AdminUsers: []user.UUID{s.userUUID},
		Name:       "my-awesome-model",
		Qualifier:  "prod",
	})

	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)
}

func (s *serviceSuite) TestModelCreationNameOwnerConflict(c *tc.C) {
	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.Key{},
		Regions:     []string{"myregion"},
	}

	svc := s.newStubService(c)
	_, activator, err := svc.CreateModel(c.Context(), model.GlobalModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		AdminUsers:  []user.UUID{s.userUUID},
		Name:        "my-awesome-model",
		Qualifier:   "prod",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(activator(c.Context()), tc.ErrorIsNil)

	_, _, err = svc.CreateModel(c.Context(), model.GlobalModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		AdminUsers:  []user.UUID{s.userUUID},
		Name:        "my-awesome-model",
		Qualifier:   "prod",
	})

	c.Assert(err, tc.ErrorIs, modelerrors.AlreadyExists)
}

func (s *serviceSuite) TestUpdateModelCredentialForInvalidModel(c *tc.C) {
	id := modeltesting.GenModelUUID(c)

	svc := s.newStubService(c)
	err := svc.UpdateCredential(c.Context(), id, credential.Key{
		Owner: usertesting.GenNewName(c, "owner"),
		Name:  "foo",
		Cloud: "aws",
	})
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *serviceSuite) TestUpdateModelCredential(c *tc.C) {
	cred := credential.Key{
		Cloud: "aws",
		Owner: usertesting.GenNewName(c, "owner"),
		Name:  "foobar",
	}

	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.Key{
			cred.String(): cred,
		},
		Regions: []string{"myregion"},
	}

	svc := s.newStubService(c)
	id, activator, err := svc.CreateModel(c.Context(), model.GlobalModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		AdminUsers:  []user.UUID{s.userUUID},
		Name:        "my-awesome-model",
		Qualifier:   "prod",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(activator(c.Context()), tc.ErrorIsNil)

	err = svc.UpdateCredential(c.Context(), id, cred)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateModelCredentialReplace(c *tc.C) {
	cred := credential.Key{
		Cloud: "aws",
		Owner: usertesting.GenNewName(c, "owner"),
		Name:  "foobar",
	}
	cred2 := credential.Key{
		Cloud: "aws",
		Owner: usertesting.GenNewName(c, "owner"),
		Name:  "foobar2",
	}

	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.Key{
			cred.String():  cred,
			cred2.String(): cred2,
		},
		Regions: []string{"myregion"},
	}

	svc := s.newStubService(c)
	id, activator, err := svc.CreateModel(c.Context(), model.GlobalModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Credential:  cred,
		AdminUsers:  []user.UUID{s.userUUID},
		Name:        "my-awesome-model",
		Qualifier:   "prod",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(activator(c.Context()), tc.ErrorIsNil)

	err = svc.UpdateCredential(c.Context(), id, cred2)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateModelCredentialZeroValue(c *tc.C) {
	cred := credential.Key{
		Cloud: "aws",
		Owner: usertesting.GenNewName(c, "owner"),
		Name:  "foobar",
	}

	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.Key{
			cred.String(): cred,
		},
		Regions: []string{"myregion"},
	}

	svc := s.newStubService(c)
	id, activator, err := svc.CreateModel(c.Context(), model.GlobalModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		AdminUsers:  []user.UUID{s.userUUID},
		Name:        "my-awesome-model",
		Qualifier:   "prod",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(activator(c.Context()), tc.ErrorIsNil)

	err = svc.UpdateCredential(c.Context(), id, credential.Key{})
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *serviceSuite) TestUpdateModelCredentialDifferentCloud(c *tc.C) {
	cred := credential.Key{
		Cloud: "aws",
		Owner: usertesting.GenNewName(c, "owner"),
		Name:  "foobar",
	}
	cred2 := credential.Key{
		Cloud: "kubernetes",
		Owner: usertesting.GenNewName(c, "owner"),
		Name:  "foobar2",
	}

	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.Key{
			cred.String(): cred,
		},
		Regions: []string{"myregion"},
	}
	s.state.clouds["kubernetes"] = dummyStateCloud{
		Credentials: map[string]credential.Key{
			cred2.String(): cred2,
		},
		Regions: []string{"myregion"},
	}

	svc := s.newStubService(c)
	id, activator, err := svc.CreateModel(c.Context(), model.GlobalModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Credential:  cred,
		AdminUsers:  []user.UUID{s.userUUID},
		Name:        "my-awesome-model",
		Qualifier:   "prod",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(activator(c.Context()), tc.ErrorIsNil)

	err = svc.UpdateCredential(c.Context(), id, cred2)
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *serviceSuite) TestUpdateModelCredentialNotFound(c *tc.C) {
	cred := credential.Key{
		Cloud: "aws",
		Owner: usertesting.GenNewName(c, "owner"),
		Name:  "foobar",
	}
	cred2 := credential.Key{
		Cloud: "aws",
		Owner: usertesting.GenNewName(c, "owner"),
		Name:  "foobar2",
	}

	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.Key{
			cred.String(): cred,
		},
		Regions: []string{"myregion"},
	}

	svc := s.newStubService(c)
	id, activator, err := svc.CreateModel(c.Context(), model.GlobalModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Credential:  cred,
		AdminUsers:  []user.UUID{s.userUUID},
		Name:        "my-awesome-model",
		Qualifier:   "prod",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(activator(c.Context()), tc.ErrorIsNil)

	err = svc.UpdateCredential(c.Context(), id, cred2)
	c.Assert(err, tc.ErrorIs, coreerrors.NotFound)
}

// TestListAllModelsNoResults is asserting that when no models exist the return
// value of ListAllModels is an empty slice.
func (s *serviceSuite) TestListAllModelsNoResults(c *tc.C) {
	svc := s.newStubService(c)
	models, err := svc.ListAllModels(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(len(models), tc.Equals, 1)
}

// TestListAllModel is a basic test to assert the happy path of
// [Service.ListAllModels].
func (s *serviceSuite) TestListAllModels(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := s.newService(c)

	id1 := modeltesting.GenModelUUID(c)
	id2 := modeltesting.GenModelUUID(c)
	s.mockState.EXPECT().ListAllModels(gomock.Any()).Return([]coremodel.Model{
		{
			Name:         "my-awesome-model",
			AgentVersion: jujuversion.Current,
			UUID:         id1,
			Cloud:        "aws",
			CloudRegion:  "myregion",
			ModelType:    coremodel.IAAS,
			Qualifier:    "prod",
			Credential: credential.Key{
				Cloud: "aws",
				Name:  "foobar",
				Owner: usertesting.GenNewName(c, "owner"),
			},
			Life: life.Alive,
		},
		{
			Name:         "my-awesome-model1",
			AgentVersion: jujuversion.Current,
			UUID:         id2,
			Cloud:        "aws",
			CloudRegion:  "myregion",
			ModelType:    coremodel.IAAS,
			Qualifier:    "staging",
			Credential: credential.Key{
				Cloud: "aws",
				Name:  "foobar",
				Owner: usertesting.GenNewName(c, "owner"),
			},
			Life: life.Alive,
		},
	}, nil)

	models, err := svc.ListAllModels(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(models, tc.DeepEquals, []coremodel.Model{
		{
			Name:         "my-awesome-model",
			AgentVersion: jujuversion.Current,
			UUID:         id1,
			Cloud:        "aws",
			CloudRegion:  "myregion",
			ModelType:    coremodel.IAAS,
			Qualifier:    "prod",
			Credential: credential.Key{
				Cloud: "aws",
				Name:  "foobar",
				Owner: usertesting.GenNewName(c, "owner"),
			},
			Life: life.Alive,
		},
		{
			Name:         "my-awesome-model1",
			AgentVersion: jujuversion.Current,
			UUID:         id2,
			Cloud:        "aws",
			CloudRegion:  "myregion",
			ModelType:    coremodel.IAAS,
			Qualifier:    "staging",
			Credential: credential.Key{
				Cloud: "aws",
				Name:  "foobar",
				Owner: usertesting.GenNewName(c, "owner"),
			},
			Life: life.Alive,
		},
	})
}

// TestListModelsForUser is asserting that for a non existent user we return
// an empty model result.
func (s *serviceSuite) TestListModelsForNonExistentUser(c *tc.C) {
	fakeUserID := usertesting.GenUserUUID(c)
	svc := s.newStubService(c)
	models, err := svc.ListModelsForUser(c.Context(), fakeUserID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(len(models), tc.Equals, 0)
}

func (s *serviceSuite) TestListModelsForUser(c *tc.C) {
	cred := credential.Key{
		Cloud: "aws",
		Name:  "foobar",
		Owner: usertesting.GenNewName(c, "owner"),
	}
	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.Key{
			cred.String(): cred,
		},
		Regions: []string{"myregion"},
	}

	usr1 := usertesting.GenUserUUID(c)
	s.state.users[usr1] = usertesting.GenNewName(c, "tlm")

	svc := s.newStubService(c)
	id1, activator, err := svc.CreateModel(c.Context(), model.GlobalModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Credential:  cred,
		AdminUsers:  []user.UUID{usr1},
		Name:        "my-awesome-model",
		Qualifier:   "tlm",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(activator(c.Context()), tc.ErrorIsNil)

	id2, activator, err := svc.CreateModel(c.Context(), model.GlobalModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Credential:  cred,
		AdminUsers:  []user.UUID{usr1},
		Name:        "my-awesome-model1",
		Qualifier:   "tlm",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(activator(c.Context()), tc.ErrorIsNil)

	models, err := svc.ListModelsForUser(c.Context(), usr1)
	c.Assert(err, tc.ErrorIsNil)

	slices.SortFunc(models, func(a, b coremodel.Model) int {
		return strings.Compare(a.Name, b.Name)
	})

	c.Check(models, tc.DeepEquals, []coremodel.Model{
		{
			Name:        "my-awesome-model",
			UUID:        id1,
			Cloud:       "aws",
			CloudRegion: "myregion",
			ModelType:   coremodel.IAAS,
			Qualifier:   "tlm",
			Credential:  cred,
			Life:        life.Alive,
		},
		{
			Name:        "my-awesome-model1",
			UUID:        id2,
			Cloud:       "aws",
			CloudRegion: "myregion",
			ModelType:   coremodel.IAAS,
			Qualifier:   "tlm",
			Credential:  cred,
			Life:        life.Alive,
		},
	})
}

// TestControllerModelNotFound is testing that if we ask the service for the
// controller model and it doesn't exist we get back a [modelerrors.NotFound]
// error. This should be a very unlikely scenario but we need to test the
// schemantics.
func (s *serviceSuite) TestControllerModelNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.mockState.EXPECT().GetControllerModel(gomock.Any()).Return(
		coremodel.Model{}, modelerrors.NotFound,
	)

	svc := s.newService(c)
	_, err := svc.ControllerModel(c.Context())
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)
}

// TestControllerModel is asserting the happy path of [Service.ControllerModel].
func (s *serviceSuite) TestControllerModel(c *tc.C) {
	adminUUID := usertesting.GenUserUUID(c)
	s.state.users[adminUUID] = coremodel.ControllerModelOwnerUsername

	cred := credential.Key{
		Cloud: "controller-cloud",
		Name:  "controller-cloud-cred",
		Owner: usertesting.GenNewName(c, "owner"),
	}

	svc := s.newStubService(c)
	modelID := s.state.controllerModelUUID
	model, err := svc.ControllerModel(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(model, tc.DeepEquals, coremodel.Model{
		Name:        coremodel.ControllerModelName,
		Qualifier:   "prod",
		Life:        life.Alive,
		UUID:        modelID,
		ModelType:   coremodel.IAAS,
		Cloud:       "controller-cloud",
		CloudRegion: "ap-southeast-2",
		Credential:  cred,
	})
}

func (s *serviceSuite) TestGetModelUsers(c *tc.C) {
	uuid, err := coremodel.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	bobName := usertesting.GenNewName(c, "bob")
	jimName := usertesting.GenNewName(c, "jim")
	adminName := usertesting.GenNewName(c, "admin")
	s.state.users = map[user.UUID]user.Name{
		"123": bobName,
		"456": jimName,
		"789": adminName,
	}
	svc := s.newStubService(c)
	modelUserInfo, err := svc.GetModelUsers(c.Context(), uuid)
	c.Assert(err, tc.IsNil)
	c.Check(modelUserInfo, tc.SameContents, []coremodel.ModelUserInfo{{
		Name:           bobName,
		DisplayName:    bobName.Name(),
		Access:         permission.AdminAccess,
		LastModelLogin: time.Time{},
	}, {
		Name:           jimName,
		DisplayName:    jimName.Name(),
		Access:         permission.AdminAccess,
		LastModelLogin: time.Time{},
	}, {
		Name:           adminName,
		DisplayName:    adminName.Name(),
		Access:         permission.AdminAccess,
		LastModelLogin: time.Time{},
	}})
}

func (s *serviceSuite) TestGetModelUsersBadUUID(c *tc.C) {
	svc := s.newStubService(c)
	_, err := svc.GetModelUsers(c.Context(), "bad-uuid)")
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *serviceSuite) TestGetModelUser(c *tc.C) {
	uuid := modeltesting.GenModelUUID(c)
	bobName := usertesting.GenNewName(c, "bob")
	jimName := usertesting.GenNewName(c, "jim")
	adminName := usertesting.GenNewName(c, "admin")
	s.state.users = map[user.UUID]user.Name{
		"123": bobName,
		"456": jimName,
		"789": adminName,
	}
	svc := s.newStubService(c)
	modelUserInfo, err := svc.GetModelUser(c.Context(), uuid, bobName)
	c.Assert(err, tc.IsNil)
	c.Check(modelUserInfo, tc.Equals, coremodel.ModelUserInfo{
		Name:           bobName,
		DisplayName:    bobName.Name(),
		Access:         permission.AdminAccess,
		LastModelLogin: time.Time{},
	})
}

func (s *serviceSuite) TestGetModelUserBadUUID(c *tc.C) {
	svc := s.newStubService(c)
	_, err := svc.GetModelUser(c.Context(), "bad-uuid", usertesting.GenNewName(c, "bob"))
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *serviceSuite) TestGetModelUserZeroUserName(c *tc.C) {
	svc := s.newStubService(c)
	_, err := svc.GetModelUser(c.Context(), modeltesting.GenModelUUID(c), user.Name{})
	c.Assert(err, tc.ErrorIs, accesserrors.UserNameNotValid)
}

// setupDefaultStateExpects establishes a common set of well know responses to
// state calls for mock testing.
func (s *serviceSuite) setupDefaultStateExpects(c *tc.C) {
	// This establishes a common response to a cloud's type
	s.mockState.EXPECT().CloudType(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, name string) (string, error) {
			if name == "cloud-caas" {
				return cloud.CloudTypeKubernetes, nil
			}
			return "aws", nil
		},
	).AnyTimes()
}

// TestCreateModelEmptyCredentialNotSupported is asserting the case where a
// model is attempted to being created with empty credentials and the cloud
// does not support this. In this case we expect a error that satisfies
// [modelerrors.CredentialNotValid]
func (s *serviceSuite) TestCreateModelEmptyCredentialNotSupported(c *tc.C) {
	defer s.setupMocks(c).Finish()
	s.setupDefaultStateExpects(c)

	s.mockState.EXPECT().CloudSupportsAuthType(gomock.Any(), "foo", cloud.EmptyAuthType)

	svc := s.newService(c)

	_, _, err := svc.CreateModel(c.Context(), model.GlobalModelCreationArgs{
		Cloud:       "foo",
		CloudRegion: "ap-southeast-2",
		Credential:  credential.Key{}, // zero value of credential implies empty
		AdminUsers:  []user.UUID{usertesting.GenUserUUID(c)},
		Name:        "new-test-model",
		Qualifier:   "prod",
	})
	c.Check(err, tc.ErrorIs, modelerrors.CredentialNotValid)
}

// TestDefaultModelCloudInfoNotFound is a white box test that
// purposely returns a [modelerrors.NotFound] error when the controller model is
// asked for. We expect that this error flows back out of the service call.
func (s *serviceSuite) TestDefaultModelCloudInfoNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.mockState.EXPECT().GetControllerModelUUID(gomock.Any()).Return(
		coremodel.UUID(""),
		modelerrors.NotFound,
	)

	svc := s.newService(c)

	_, _, err := svc.DefaultModelCloudInfo(c.Context())
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)

	// There exists to ways for the controller model to not be found. This is
	// asserting the second path where the code get's the uuid but the model
	// no longer exists for this uuid.
	ctrlModelUUID := modeltesting.GenModelUUID(c)
	s.mockState.EXPECT().GetControllerModelUUID(gomock.Any()).Return(
		ctrlModelUUID,
		nil,
	)
	s.mockState.EXPECT().GetModelCloudInfo(gomock.Any(), ctrlModelUUID).Return(
		"", "", modelerrors.NotFound,
	)

	_, _, err = svc.DefaultModelCloudInfo(c.Context())
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)
}

// TestDefaultModelCloudInfo is asserting the happy path that when
// a controller model exists the cloud name and credential are returned.
func (s *serviceSuite) TestDefaultModelCloudInfo(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := s.newService(c)

	// There exists to ways for the controller model to not be found. This is
	// asserting the second path where the code get's the uuid but the model
	// no longer exists for this uuid.
	ctrlModelUUID := modeltesting.GenModelUUID(c)
	s.mockState.EXPECT().GetControllerModelUUID(gomock.Any()).Return(
		ctrlModelUUID,
		nil,
	)
	s.mockState.EXPECT().GetModelCloudInfo(gomock.Any(), ctrlModelUUID).Return(
		"test", "test-region", // cloud name and region
		nil,
	)

	cloud, region, err := svc.DefaultModelCloudInfo(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(cloud, tc.Equals, "test")
	c.Check(region, tc.Equals, "test-region")
}

// TestWatchActivatedModels verifies that WatchActivatedModels correctly sets up a watcher
// that emits events for activated models when the watcher receives change events.
func (s *serviceSuite) TestWatchActivatedModels(c *tc.C) {
	defer s.setupMocks(c).Finish()
	ctx := c.Context()
	svc := s.newWatchableService(c)

	// Set up necessary mock return values.
	s.mockState.EXPECT().InitialWatchActivatedModelsStatement().Return(
		"model", "SELECT uuid from model WHERE activated = true",
	)

	changes := make(chan []string, 1)
	activatedModelUUID1 := modeltesting.GenModelUUID(c)
	activatedModelUUID2 := modeltesting.GenModelUUID(c)
	activatedModelUUIDs := []coremodel.UUID{activatedModelUUID1, activatedModelUUID2}
	activatedModelUUIDsStr := transform.Slice(activatedModelUUIDs, func(uuid coremodel.UUID) string {
		return uuid.String()
	})

	select {
	case changes <- activatedModelUUIDsStr:
	case <-time.After(testhelpers.LongWait):
		c.Fatalf("failed to send changes to channel")
	}

	s.mockStringsWatcher.EXPECT().Changes().AnyTimes().Return(changes)

	s.mockWatcherFactory.EXPECT().NewNamespaceMapperWatcher(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(s.mockStringsWatcher, nil)

	// Verifies that the service returns a watcher with the correct model UUIDs string.
	watcher, err := svc.WatchActivatedModels(ctx)
	c.Assert(err, tc.ErrorIsNil)

	select {
	case change := <-watcher.Changes():
		c.Check(change, tc.DeepEquals, activatedModelUUIDsStr)
	case <-time.After(testhelpers.LongWait):
		c.Fatalf("failed to receive changes from watcher")
	}
}

func (s *serviceSuite) createMockChangeEventsFromUUIDs(ctrl *gomock.Controller, uuids ...coremodel.UUID) []changestream.ChangeEvent {
	events := make([]changestream.ChangeEvent, len(uuids))
	for i, uuid := range uuids {
		event := changestreammock.NewMockChangeEvent(ctrl)
		event.EXPECT().Changed().AnyTimes().Return(
			uuid.String(),
		)
		events[i] = event
	}
	return events
}

// TestWatchActivatedModelsMapper verifies that the WatchActivatedModelsMapper correctly
// filters change events to include only those associated with activated models and that
// the subset of changes returned is maintained in the same order as they are received.
func (s *serviceSuite) TestWatchActivatedModelsMapper(c *tc.C) {
	defer s.setupMocks(c).Finish()
	ctx := c.Context()

	activatedModelUUID1 := modeltesting.GenModelUUID(c)
	activatedModelUUID2 := modeltesting.GenModelUUID(c)
	activatedModelUUID3 := modeltesting.GenModelUUID(c)
	activatedModelUUID4 := modeltesting.GenModelUUID(c)
	activatedModelUUID5 := modeltesting.GenModelUUID(c)
	duplicateActivatedModelUUID := activatedModelUUID1
	unactivatedModelUUID1 := modeltesting.GenModelUUID(c)
	unactivatedModelUUID2 := modeltesting.GenModelUUID(c)

	inputModelUUIDs := []coremodel.UUID{activatedModelUUID1, activatedModelUUID2, unactivatedModelUUID1,
		activatedModelUUID3, unactivatedModelUUID2, activatedModelUUID4, activatedModelUUID5, duplicateActivatedModelUUID}
	activatedModelUUIDs := []coremodel.UUID{activatedModelUUID1, activatedModelUUID2, activatedModelUUID3,
		activatedModelUUID4, activatedModelUUID5, duplicateActivatedModelUUID}

	s.mockState.EXPECT().GetActivatedModelUUIDs(gomock.Any(), inputModelUUIDs).Return(
		activatedModelUUIDs, nil,
	)

	// // Change events received by the watcher mapper.
	inputChangeEvents := s.createMockChangeEventsFromUUIDs(s.mockWatcherFactory.ctrl, inputModelUUIDs...)

	// Change events containing model UUIDs of activated models retrieved from the database.
	// The order of returned events should be maintained after filter.
	expectedChangeEvents := s.createMockChangeEventsFromUUIDs(s.mockWatcherFactory.ctrl, activatedModelUUIDs...)

	// Tests if mapper correctly filters changes
	mapper := getWatchActivatedModelsMapper(s.mockState)

	// Use service mapper to retrieve change events containing only model UUIDs of activated models.
	retrievedChangeEvents, err := mapper(ctx, inputChangeEvents)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(retrievedChangeEvents, tc.DeepEquals, transform.Slice(expectedChangeEvents, func(event changestream.ChangeEvent) string {
		return event.Changed()
	}))
}

// TestGetModelByNameAndQualifierSuccess verifies that GetModelByNameAndQualifier successfully
// returns the model associated with the specified qualifier and model name.
func (s *serviceSuite) TestGetModelByNameAndQualifierSuccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := s.newService(c)

	modelUUID := modeltesting.GenModelUUID(c)
	modelName := "test"
	ownerUserName := usertesting.GenNewName(c, "test-user")
	model := coremodel.Model{
		Name:         modelName,
		AgentVersion: jujuversion.Current,
		UUID:         modelUUID,
		Cloud:        "aws",
		CloudRegion:  "testregion",
		ModelType:    coremodel.IAAS,
		Qualifier:    "test-user",
		Credential: credential.Key{
			Cloud: "aws",
			Name:  "testcredential",
			Owner: ownerUserName,
		},
		Life: life.Alive,
	}
	s.mockState.EXPECT().GetModelByName(gomock.Any(), "prod", modelName).Return(
		model,
		nil,
	)

	svcModel, err := svc.GetModelByNameAndQualifier(c.Context(), modelName, "prod")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(model, tc.Equals, svcModel)
}

// TestGetModelByNameAndQualifierInvalidQualifier verifies that
// GetModelByNameAndQualifier returns a [coreerrors.NotValid] error when
// the provided qualifier is invalid.
func (s *serviceSuite) TestGetModelByNameAndQualifierInvalidQualifier(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := s.newService(c)

	modelName := "test"

	_, err := svc.GetModelByNameAndQualifier(c.Context(), modelName, "")
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestGetModelByNameAndQualifierNotFound verifies that GetModelByNameAndQualifier
// returns a [modelerrors.NotFound] error
// when no model exists for the given qualifier and model name.
func (s *serviceSuite) TestGetModelByNameAndQualifierNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := s.newService(c)

	modelName := "test"
	s.mockState.EXPECT().GetModelByName(gomock.Any(), "prod", modelName).Return(
		coremodel.Model{},
		modelerrors.NotFound,
	)

	_, err := svc.GetModelByNameAndQualifier(c.Context(), modelName, "prod")
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *serviceSuite) TestGetModelLife(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := modeltesting.GenModelUUID(c)

	s.mockState.EXPECT().GetModelLife(gomock.Any(), modelUUID).Return(
		domainlife.Alive,
		nil,
	)

	svc := s.newService(c)

	result, err := svc.GetModelLife(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.Equals, life.Alive)
}

func (s *serviceSuite) TestGetModelLifeInvalidUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	svc := s.newService(c)

	_, err := svc.GetModelLife(c.Context(), "!!!!")
	c.Assert(err, tc.ErrorMatches, `*.not valid`)
}

func (s *serviceSuite) TestGetModelLifeNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := modeltesting.GenModelUUID(c)

	s.mockState.EXPECT().GetModelLife(gomock.Any(), modelUUID).Return(
		domainlife.Alive,
		modelerrors.NotFound,
	)

	svc := s.newService(c)

	_, err := svc.GetModelLife(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *serviceSuite) TestWatchModelCloudCredential(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	cloudUUID := cloudtesting.GenCloudUUID(c)
	credentialUUID := credential.UUID(uuid.MustNewUUID().String())
	s.mockState.EXPECT().GetModelCloudAndCredential(gomock.Any(), modelUUID).Return(cloudUUID, credentialUUID, nil)

	ch := make(chan struct{}, 1)
	watcher := watchertest.NewMockNotifyWatcher(ch)
	s.mockWatcherFactory.EXPECT().NewNotifyMapperWatcher(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(watcher, nil)

	svc := s.newWatchableService(c)
	w, err := svc.WatchModelCloudCredential(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	select {
	case ch <- struct{}{}:
	case <-time.After(testhelpers.LongWait):
		c.Fatalf("failed to send changes to channel")
	}

	select {
	case <-w.Changes():
	case <-time.After(testhelpers.LongWait):
		c.Fatalf("failed to receive changes from watcher")
	}
}

// TestListModelUUIDsForUser is asserting the happy path of
// [Service.ListModelUUIDsForUser].
func (s *serviceSuite) TestListModelUUIDsForUser(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	s.mockState.EXPECT().ListModelUUIDsForUser(gomock.Any(), s.userUUID).Return(
		[]coremodel.UUID{modelUUID}, nil,
	)

	svc := s.newService(c)
	uuids, err := svc.ListModelUUIDsForUser(c.Context(), s.userUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(uuids, tc.SameContents, []coremodel.UUID{modelUUID})
}

// TestListModelUUIDsForUserNotFound is asserting that when the list of model
// uuids for a user is asked for and the user does not exist the caller gets an
// error satisfying [accesserrors.UserNotFound].
func (s *serviceSuite) TestListModelUUIDsForUserNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.mockState.EXPECT().ListModelUUIDsForUser(gomock.Any(), s.userUUID).Return(
		nil, accesserrors.UserNotFound,
	)

	svc := s.newService(c)
	_, err := svc.ListModelUUIDsForUser(c.Context(), s.userUUID)
	c.Check(err, tc.ErrorIs, accesserrors.UserNotFound)
}

// TestListModelUUIDsForUserNotValid is asserting that when the list of model
// uuids for a user is asked for and the user uuid is not valid the caller gets
// an error satisfying [coreerrors.NotValid].
func (s *serviceSuite) TestListModelUUIDsForUserNotValid(c *tc.C) {
	svc := s.newStubService(c)
	_, err := svc.ListModelUUIDsForUser(c.Context(), "not-a-uuid")
	c.Check(err, tc.ErrorIs, coreerrors.NotValid)
}

// TestGetControllerModelUUID tests that the controller model uuid can be
// retrieved correctly.
func (s *serviceSuite) TestGetControllerModelUUID(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expectedControllerModelUUID := modeltesting.GenModelUUID(c)
	s.mockState.EXPECT().GetControllerModelUUID(gomock.Any()).Return(
		expectedControllerModelUUID,
		nil,
	)

	svc := s.newService(c)
	obtainedControllerModelUUID, err := svc.GetControllerModelUUID(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(obtainedControllerModelUUID, tc.DeepEquals, expectedControllerModelUUID)
}

// TestGetControllerModelUUIDNotFound tests that the controller model uuid
// cannot be retrieved due to an error returned from state.
func (s *serviceSuite) TestGetControllerModelUUIDNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.mockState.EXPECT().GetControllerModelUUID(gomock.Any()).Return(
		coremodel.UUID(""),
		errors.New("boom"),
	)

	svc := s.newService(c)
	_, err := svc.GetControllerModelUUID(c.Context())
	c.Assert(err, tc.ErrorMatches, "boom")
}
