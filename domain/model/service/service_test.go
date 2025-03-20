// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	changestream "github.com/juju/juju/core/changestream"
	changestreammock "github.com/juju/juju/core/changestream/mocks"
	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/life"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	jujuversion "github.com/juju/juju/core/version"
	accesserrors "github.com/juju/juju/domain/access/errors"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	secretbackenderrors "github.com/juju/juju/domain/secretbackend/errors"
	changestreamtesting "github.com/juju/juju/internal/changestream/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	jujusecrets "github.com/juju/juju/internal/secrets/provider/juju"
	kubernetessecrets "github.com/juju/juju/internal/secrets/provider/kubernetes"
	jujutesting "github.com/juju/juju/internal/testing"
)

type serviceSuite struct {
	testing.IsolationSuite

	userUUID user.UUID
	state    *dummyState
	deleter  *dummyDeleter

	mockModelDeleter   *MockModelDeleter
	mockState          *MockState
	mockWatcherFactory *MockWatcherFactory
	mockStringsWatcher *MockStringsWatcher[[]string]
	changestreamtesting.ControllerSuite
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) SetUpTest(c *gc.C) {
	var err error
	s.userUUID = usertesting.GenUserUUID(c)
	c.Assert(err, jc.ErrorIsNil)
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

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockModelDeleter = NewMockModelDeleter(ctrl)
	s.mockState = NewMockState(ctrl)
	s.mockWatcherFactory = NewMockWatcherFactory(ctrl)
	s.mockStringsWatcher = NewMockStringsWatcher[[]string](ctrl)

	return ctrl
}

func (s *serviceSuite) setupControllerModel(c *gc.C) {
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

	svc := NewService(s.state, s.deleter, loggertesting.WrapCheckLog(c))
	modelID, activator, err := svc.CreateModel(context.Background(), model.GlobalModelCreationArgs{
		Cloud:       "controller-cloud",
		CloudRegion: "ap-southeast-2",
		Credential:  cred,
		Owner:       adminUUID,
		Name:        coremodel.ControllerModelName,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(activator(context.Background()), jc.ErrorIsNil)
	s.state.controllerModelUUID = modelID
}

// TestControllerModelNameChange is here to make the breaker of this test stop
// and think. There exists business logic in this package that is very dependent
// on the well known value defined in [coremodel.ControllerModelName]. If this
// test has broken it means this value has changed and you could be at risk of
// breaking Juju. Please consider the business logic in this package and if
// changing this well known value is handled correctly for both legacy and
// future Juju versions!!!
func (s *serviceSuite) TestControllerModelNameChange(c *gc.C) {
	c.Assert(coremodel.ControllerModelName, gc.Equals, "controller")
}

// TestControllerModelOwnerUsername is here to make the breaker of this test
// stop and think. There exists business logic in this package that is very
// dependent on the well known value defined in
// [coremodel.ControllerModelOwnerUsername]. If this test has broken it means
// this value has changed and you could be at risk of breaking Juju. Please
// consider the business logic in this package and if changing this well known
// value is handled correctly for both legacy and future Juju versions!!!
func (s *serviceSuite) TestControllerModelOwnerUsername(c *gc.C) {
	c.Assert(coremodel.ControllerModelOwnerUsername, gc.Equals, usertesting.GenNewName(c, "admin"))
}

func (s *serviceSuite) TestCreateModelInvalidArgs(c *gc.C) {
	svc := NewService(s.state, s.deleter, loggertesting.WrapCheckLog(c))
	_, _, err := svc.CreateModel(context.Background(), model.GlobalModelCreationArgs{})
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *serviceSuite) TestModelCreation(c *gc.C) {
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

	svc := NewService(s.state, s.deleter, loggertesting.WrapCheckLog(c))
	id, activator, err := svc.CreateModel(context.Background(), model.GlobalModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Credential:  cred,
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(activator(context.Background()), jc.ErrorIsNil)

	_, exists := s.state.models[id]
	c.Assert(exists, jc.IsTrue)

	modelList, err := svc.ListModelIDs(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(len(modelList), gc.Equals, 2)
}

// TestModelCreationSecretBackendNotFound is asserting that if we try and add a
// model and define a secret backend for the new model that doesn't exist we get
// back a [secretbackenderrors.NotFound] error.
func (s *serviceSuite) TestModelCreationSecretBackendNotFound(c *gc.C) {
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

	svc := NewService(s.state, s.deleter, loggertesting.WrapCheckLog(c))
	_, _, err := svc.CreateModel(context.Background(), model.GlobalModelCreationArgs{
		Cloud:         "aws",
		CloudRegion:   "myregion",
		Credential:    cred,
		Owner:         s.userUUID,
		Name:          "my-awesome-model",
		SecretBackend: "no-exist",
	})

	c.Assert(err, jc.ErrorIs, secretbackenderrors.NotFound)
}

func (s *serviceSuite) TestModelCreationInvalidCloud(c *gc.C) {
	s.state.clouds["aws"] = dummyStateCloud{}
	svc := NewService(s.state, s.deleter, loggertesting.WrapCheckLog(c))
	_, _, err := svc.CreateModel(context.Background(), model.GlobalModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
	})

	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *serviceSuite) TestModelCreationNoCloudRegion(c *gc.C) {
	s.state.clouds["aws"] = dummyStateCloud{
		Regions: []string{"myregion"},
	}

	svc := NewService(s.state, s.deleter, loggertesting.WrapCheckLog(c))
	_, _, err := svc.CreateModel(context.Background(), model.GlobalModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "noexist",
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
	})

	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

// TestModelCreationOwnerNotFound is testing that if we make a model with an
// owner that doesn't exist we get back a [accesserrors.NotFound] error.
func (s *serviceSuite) TestModelCreationOwnerNotFound(c *gc.C) {
	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.Key{},
		Regions:     []string{"myregion"},
	}

	notFoundUser, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	svc := NewService(s.state, s.deleter, loggertesting.WrapCheckLog(c))
	_, _, err = svc.CreateModel(context.Background(), model.GlobalModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Owner:       notFoundUser,
		Name:        "my-awesome-model",
	})

	c.Assert(err, jc.ErrorIs, accesserrors.UserNotFound)
}

func (s *serviceSuite) TestModelCreationNoCloudCredential(c *gc.C) {
	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.Key{},
		Regions:     []string{"myregion"},
	}

	svc := NewService(s.state, s.deleter, loggertesting.WrapCheckLog(c))
	_, _, err := svc.CreateModel(context.Background(), model.GlobalModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Credential: credential.Key{
			Cloud: "aws",
			Name:  "foo",
			Owner: usertesting.GenNewName(c, "owner"),
		},
		Owner: s.userUUID,
		Name:  "my-awesome-model",
	})

	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *serviceSuite) TestModelCreationNameOwnerConflict(c *gc.C) {
	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.Key{},
		Regions:     []string{"myregion"},
	}

	svc := NewService(s.state, s.deleter, loggertesting.WrapCheckLog(c))
	_, activator, err := svc.CreateModel(context.Background(), model.GlobalModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(activator(context.Background()), jc.ErrorIsNil)

	_, _, err = svc.CreateModel(context.Background(), model.GlobalModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
	})

	c.Assert(err, jc.ErrorIs, modelerrors.AlreadyExists)
}

func (s *serviceSuite) TestUpdateModelCredentialForInvalidModel(c *gc.C) {
	id := modeltesting.GenModelUUID(c)

	svc := NewService(s.state, s.deleter, loggertesting.WrapCheckLog(c))
	err := svc.UpdateCredential(context.Background(), id, credential.Key{
		Owner: usertesting.GenNewName(c, "owner"),
		Name:  "foo",
		Cloud: "aws",
	})
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

func (s *serviceSuite) TestUpdateModelCredential(c *gc.C) {
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

	svc := NewService(s.state, s.deleter, loggertesting.WrapCheckLog(c))
	id, activator, err := svc.CreateModel(context.Background(), model.GlobalModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(activator(context.Background()), jc.ErrorIsNil)

	err = svc.UpdateCredential(context.Background(), id, cred)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateModelCredentialReplace(c *gc.C) {
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

	svc := NewService(s.state, s.deleter, loggertesting.WrapCheckLog(c))
	id, activator, err := svc.CreateModel(context.Background(), model.GlobalModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Credential:  cred,
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(activator(context.Background()), jc.ErrorIsNil)

	err = svc.UpdateCredential(context.Background(), id, cred2)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateModelCredentialZeroValue(c *gc.C) {
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

	svc := NewService(s.state, s.deleter, loggertesting.WrapCheckLog(c))
	id, activator, err := svc.CreateModel(context.Background(), model.GlobalModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(activator(context.Background()), jc.ErrorIsNil)

	err = svc.UpdateCredential(context.Background(), id, credential.Key{})
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *serviceSuite) TestUpdateModelCredentialDifferentCloud(c *gc.C) {
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

	svc := NewService(s.state, s.deleter, loggertesting.WrapCheckLog(c))
	id, activator, err := svc.CreateModel(context.Background(), model.GlobalModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Credential:  cred,
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(activator(context.Background()), jc.ErrorIsNil)

	err = svc.UpdateCredential(context.Background(), id, cred2)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *serviceSuite) TestUpdateModelCredentialNotFound(c *gc.C) {
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

	svc := NewService(s.state, s.deleter, loggertesting.WrapCheckLog(c))
	id, activator, err := svc.CreateModel(context.Background(), model.GlobalModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Credential:  cred,
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(activator(context.Background()), jc.ErrorIsNil)

	err = svc.UpdateCredential(context.Background(), id, cred2)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *serviceSuite) TestDeleteModel(c *gc.C) {
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

	svc := NewService(s.state, s.deleter, loggertesting.WrapCheckLog(c))
	id, activator, err := svc.CreateModel(context.Background(), model.GlobalModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Credential:  cred,
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(activator(context.Background()), jc.ErrorIsNil)

	_, exists := s.state.models[id]
	c.Assert(exists, jc.IsTrue)

	err = svc.DeleteModel(context.Background(), id, model.WithDeleteDB())
	c.Assert(err, jc.ErrorIsNil)
	_, exists = s.state.models[id]
	c.Assert(exists, jc.IsFalse)

	_, exists = s.deleter.deleted[id.String()]
	c.Assert(exists, jc.IsTrue)
}

type notFoundDeleter struct{}

func (d notFoundDeleter) DeleteDB(string) error {
	return modelerrors.NotFound
}

func (s *serviceSuite) TestDeleteModelNotFound(c *gc.C) {
	svc := NewService(s.state, notFoundDeleter{}, loggertesting.WrapCheckLog(c))
	err := svc.DeleteModel(context.Background(), modeltesting.GenModelUUID(c), model.WithDeleteDB())
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

// TestListAllModelsNoResults is asserting that when no models exist the return
// value of ListAllModels is an empty slice.
func (s *serviceSuite) TestListAllModelsNoResults(c *gc.C) {
	svc := NewService(s.state, s.deleter, loggertesting.WrapCheckLog(c))
	models, err := svc.ListAllModels(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(len(models), gc.Equals, 1)
}

// TestListAllModel is a basic test to assert the happy path of
// [Service.ListAllModels].
func (s *serviceSuite) TestListAllModels(c *gc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockState,
		s.mockModelDeleter,
		loggertesting.WrapCheckLog(c),
	)

	usr1 := usertesting.GenUserUUID(c)
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
			Owner:        s.userUUID,
			OwnerName:    usertesting.GenNewName(c, "admin"),
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
			Owner:        usr1,
			OwnerName:    usertesting.GenNewName(c, "tlm"),
			Credential: credential.Key{
				Cloud: "aws",
				Name:  "foobar",
				Owner: usertesting.GenNewName(c, "owner"),
			},
			Life: life.Alive,
		},
	}, nil)

	models, err := svc.ListAllModels(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(models, jc.DeepEquals, []coremodel.Model{
		{
			Name:         "my-awesome-model",
			AgentVersion: jujuversion.Current,
			UUID:         id1,
			Cloud:        "aws",
			CloudRegion:  "myregion",
			ModelType:    coremodel.IAAS,
			Owner:        s.userUUID,
			OwnerName:    usertesting.GenNewName(c, "admin"),
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
			Owner:        usr1,
			OwnerName:    usertesting.GenNewName(c, "tlm"),
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
func (s *serviceSuite) TestListModelsForNonExistentUser(c *gc.C) {
	fakeUserID := usertesting.GenUserUUID(c)
	svc := NewService(s.state, s.deleter, loggertesting.WrapCheckLog(c))
	models, err := svc.ListModelsForUser(context.Background(), fakeUserID)
	c.Check(err, jc.ErrorIsNil)
	c.Check(len(models), gc.Equals, 0)
}

func (s *serviceSuite) TestListModelsForUser(c *gc.C) {
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

	svc := NewService(s.state, s.deleter, loggertesting.WrapCheckLog(c))
	id1, activator, err := svc.CreateModel(context.Background(), model.GlobalModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Credential:  cred,
		Owner:       usr1,
		Name:        "my-awesome-model",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(activator(context.Background()), jc.ErrorIsNil)

	id2, activator, err := svc.CreateModel(context.Background(), model.GlobalModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Credential:  cred,
		Owner:       usr1,
		Name:        "my-awesome-model1",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(activator(context.Background()), jc.ErrorIsNil)

	models, err := svc.ListModelsForUser(context.Background(), usr1)
	c.Assert(err, jc.ErrorIsNil)

	slices.SortFunc(models, func(a, b coremodel.Model) int {
		return strings.Compare(a.Name, b.Name)
	})

	c.Check(models, gc.DeepEquals, []coremodel.Model{
		{
			Name:        "my-awesome-model",
			UUID:        id1,
			Cloud:       "aws",
			CloudRegion: "myregion",
			ModelType:   coremodel.IAAS,
			Owner:       usr1,
			OwnerName:   usertesting.GenNewName(c, "tlm"),
			Credential:  cred,
			Life:        life.Alive,
		},
		{
			Name:        "my-awesome-model1",
			UUID:        id2,
			Cloud:       "aws",
			CloudRegion: "myregion",
			ModelType:   coremodel.IAAS,
			Owner:       usr1,
			OwnerName:   usertesting.GenNewName(c, "tlm"),
			Credential:  cred,
			Life:        life.Alive,
		},
	})
}

// TestImportModelWithMissingAgentVersion is asserting that if we try and import
// a model that does not have an agent version set in the args we get back an
// error that satisfies [modelerrors.AgentVersionNotSupported].
func (s *serviceSuite) TestImportModelWithMissingAgentVersion(c *gc.C) {
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

	modelID := modeltesting.GenModelUUID(c)

	svc := NewService(s.state, s.deleter, loggertesting.WrapCheckLog(c))
	_, err := svc.ImportModel(context.Background(), model.ModelImportArgs{
		GlobalModelCreationArgs: model.GlobalModelCreationArgs{
			Cloud:       "aws",
			CloudRegion: "myregion",
			Credential:  cred,
			Owner:       s.userUUID,
			Name:        "my-awesome-model",
		},
		ID: modelID,
	})
	c.Assert(err, jc.ErrorIs, modelerrors.AgentVersionNotSupported)

	_, exists := s.state.models[modelID]
	c.Assert(exists, jc.IsFalse)
}

// TestImportModel is asserting the happy path for importing a model.
func (s *serviceSuite) TestImportModel(c *gc.C) {
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

	modelID := modeltesting.GenModelUUID(c)

	svc := NewService(s.state, s.deleter, loggertesting.WrapCheckLog(c))
	activator, err := svc.ImportModel(context.Background(), model.ModelImportArgs{
		GlobalModelCreationArgs: model.GlobalModelCreationArgs{
			Cloud:       "aws",
			CloudRegion: "myregion",
			Credential:  cred,
			Owner:       s.userUUID,
			Name:        "my-awesome-model",
		},
		ID:           modelID,
		AgentVersion: jujuversion.Current,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(activator(context.Background()), jc.ErrorIsNil)

	_, exists := s.state.models[modelID]
	c.Assert(exists, jc.IsTrue)
}

// TestControllerModelNotFound is testing that if we ask the service for the
// controller model and it doesn't exist we get back a [modelerrors.NotFound]
// error. This should be a very unlikely scenario but we need to test the
// schemantics.
func (s *serviceSuite) TestControllerModelNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.mockState.EXPECT().GetControllerModel(gomock.Any()).Return(
		coremodel.Model{}, modelerrors.NotFound,
	)

	svc := NewService(
		s.mockState,
		s.mockModelDeleter,
		loggertesting.WrapCheckLog(c),
	)
	_, err := svc.ControllerModel(context.Background())
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)
}

// TestControllerModel is asserting the happy path of [Service.ControllerModel].
func (s *serviceSuite) TestControllerModel(c *gc.C) {
	adminUUID := usertesting.GenUserUUID(c)
	s.state.users[adminUUID] = coremodel.ControllerModelOwnerUsername

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

	svc := NewService(s.state, s.deleter, loggertesting.WrapCheckLog(c))
	modelID, activator, err := svc.CreateModel(context.Background(), model.GlobalModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Credential:  cred,
		Owner:       adminUUID,
		Name:        coremodel.ControllerModelName,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(activator(context.Background()), jc.ErrorIsNil)
	s.state.controllerModelUUID = modelID

	model, err := svc.ControllerModel(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(model, gc.DeepEquals, coremodel.Model{
		Name:        coremodel.ControllerModelName,
		Life:        life.Alive,
		UUID:        modelID,
		ModelType:   coremodel.IAAS,
		Cloud:       "aws",
		CloudRegion: "myregion",
		Credential:  cred,
		Owner:       adminUUID,
		OwnerName:   coremodel.ControllerModelOwnerUsername,
	})
}

func (s *serviceSuite) TestGetModelUsers(c *gc.C) {
	uuid, err := coremodel.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	bobName := usertesting.GenNewName(c, "bob")
	jimName := usertesting.GenNewName(c, "jim")
	adminName := usertesting.GenNewName(c, "admin")
	s.state.users = map[user.UUID]user.Name{
		"123": bobName,
		"456": jimName,
		"789": adminName,
	}
	svc := NewService(s.state, s.deleter, loggertesting.WrapCheckLog(c))
	modelUserInfo, err := svc.GetModelUsers(context.Background(), uuid)
	c.Assert(err, gc.IsNil)
	c.Check(modelUserInfo, jc.SameContents, []coremodel.ModelUserInfo{{
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

func (s *serviceSuite) TestGetModelUsersBadUUID(c *gc.C) {
	svc := NewService(s.state, s.deleter, loggertesting.WrapCheckLog(c))
	_, err := svc.GetModelUsers(context.Background(), "bad-uuid)")
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *serviceSuite) TestGetModelUser(c *gc.C) {
	uuid := modeltesting.GenModelUUID(c)
	bobName := usertesting.GenNewName(c, "bob")
	jimName := usertesting.GenNewName(c, "jim")
	adminName := usertesting.GenNewName(c, "admin")
	s.state.users = map[user.UUID]user.Name{
		"123": bobName,
		"456": jimName,
		"789": adminName,
	}
	svc := NewService(s.state, s.deleter, loggertesting.WrapCheckLog(c))
	modelUserInfo, err := svc.GetModelUser(context.Background(), uuid, bobName)
	c.Assert(err, gc.IsNil)
	c.Check(modelUserInfo, gc.Equals, coremodel.ModelUserInfo{
		Name:           bobName,
		DisplayName:    bobName.Name(),
		Access:         permission.AdminAccess,
		LastModelLogin: time.Time{},
	})
}

func (s *serviceSuite) TestGetModelUserBadUUID(c *gc.C) {
	svc := NewService(s.state, s.deleter, loggertesting.WrapCheckLog(c))
	_, err := svc.GetModelUser(context.Background(), "bad-uuid", usertesting.GenNewName(c, "bob"))
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *serviceSuite) TestGetModelUserZeroUserName(c *gc.C) {
	svc := NewService(s.state, s.deleter, loggertesting.WrapCheckLog(c))
	_, err := svc.GetModelUser(context.Background(), modeltesting.GenModelUUID(c), user.Name{})
	c.Assert(err, jc.ErrorIs, accesserrors.UserNameNotValid)
}

func (s *serviceSuite) TestListAllModelSummaries(c *gc.C) {
	uuid1, err := coremodel.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	uuid2, err := coremodel.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	s.state.controllerModelUUID = uuid1
	s.state.models = map[coremodel.UUID]coremodel.Model{
		uuid1: {
			Name:         "my-awesome-model",
			AgentVersion: jujuversion.Current,
			UUID:         uuid1,
			Cloud:        "aws",
			CloudRegion:  "myregion",
			ModelType:    coremodel.IAAS,
			OwnerName:    usertesting.GenNewName(c, "admin"),
			Life:         life.Alive,
		},
		uuid2: {
			Name:         "",
			AgentVersion: jujuversion.Current,
			UUID:         uuid2,
			Cloud:        "aws",
			CloudRegion:  "myregion",
			ModelType:    coremodel.IAAS,
			OwnerName:    usertesting.GenNewName(c, "tlm"),
			Life:         life.Alive,
		},
	}
	svc := NewService(s.state, s.deleter, loggertesting.WrapCheckLog(c))
	models, err := svc.ListAllModelSummaries(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(models, jc.SameContents, []coremodel.ModelSummary{{
		Name:           "my-awesome-model",
		AgentVersion:   jujuversion.Current,
		UUID:           uuid1,
		CloudName:      "aws",
		CloudRegion:    "myregion",
		ModelType:      coremodel.IAAS,
		OwnerName:      usertesting.GenNewName(c, "admin"),
		Life:           life.Alive,
		ControllerUUID: jujutesting.ControllerTag.Id(),
		IsController:   true,
	}, {
		Name:           "",
		AgentVersion:   jujuversion.Current,
		UUID:           uuid2,
		CloudName:      "aws",
		CloudRegion:    "myregion",
		ModelType:      coremodel.IAAS,
		OwnerName:      usertesting.GenNewName(c, "tlm"),
		Life:           life.Alive,
		ControllerUUID: jujutesting.ControllerTag.Id(),
		IsController:   false,
	}})
}

func (s *serviceSuite) TestListModelsForUserBadName(c *gc.C) {
	svc := NewService(s.state, s.deleter, loggertesting.WrapCheckLog(c))
	_, err := svc.ListModelsForUser(context.Background(), "((*)(")
	c.Check(err, jc.ErrorIs, errors.NotValid)
}

func (s *serviceSuite) TestListModelSummariesForUser(c *gc.C) {
	uuid1, err := coremodel.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	s.state.controllerModelUUID = uuid1
	s.state.models = map[coremodel.UUID]coremodel.Model{
		uuid1: {
			Name:         "my-awesome-model",
			AgentVersion: jujuversion.Current,
			UUID:         uuid1,
			Cloud:        "aws",
			CloudRegion:  "myregion",
			ModelType:    coremodel.IAAS,
			OwnerName:    usertesting.GenNewName(c, "admin"),
			Life:         life.Alive,
		},
	}
	svc := NewService(s.state, s.deleter, loggertesting.WrapCheckLog(c))
	models, err := svc.ListModelSummariesForUser(context.Background(), usertesting.GenNewName(c, "admin"))
	c.Assert(err, jc.ErrorIsNil)
	c.Check(models, gc.DeepEquals, []coremodel.UserModelSummary{{
		UserAccess: permission.AdminAccess,
		ModelSummary: coremodel.ModelSummary{
			Name:           "my-awesome-model",
			AgentVersion:   jujuversion.Current,
			UUID:           uuid1,
			CloudName:      "aws",
			CloudRegion:    "myregion",
			ModelType:      coremodel.IAAS,
			OwnerName:      usertesting.GenNewName(c, "admin"),
			Life:           life.Alive,
			ControllerUUID: jujutesting.ControllerTag.Id(),
			IsController:   true,
		},
	}})
}

// setupDefaultStateExpects establishes a common set of well know responses to
// state calls for mock testing.
func (s *serviceSuite) setupDefaultStateExpects(c *gc.C) {
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
func (s *serviceSuite) TestCreateModelEmptyCredentialNotSupported(c *gc.C) {
	defer s.setupMocks(c).Finish()
	s.setupDefaultStateExpects(c)

	s.mockState.EXPECT().CloudSupportsAuthType(gomock.Any(), "foo", cloud.EmptyAuthType)

	svc := NewService(
		s.mockState,
		s.mockModelDeleter,
		loggertesting.WrapCheckLog(c),
	)

	_, _, err := svc.CreateModel(context.Background(), model.GlobalModelCreationArgs{
		Cloud:       "foo",
		CloudRegion: "ap-southeast-2",
		Credential:  credential.Key{}, // zero value of credential implies empty
		Owner:       usertesting.GenUserUUID(c),
		Name:        "new-test-model",
	})
	c.Check(err, jc.ErrorIs, modelerrors.CredentialNotValid)
}

// TestDefaultModelCloudNameAndCredentialNotFound is a white box test that
// purposely returns a [modelerrors.NotFound] error when the controller model is
// asked for. We expect that this error flows back out of the service call.
func (s *serviceSuite) TestDefaultModelCloudNameAndCredentialNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.mockState.EXPECT().GetControllerModelUUID(gomock.Any()).Return(
		coremodel.UUID(""),
		modelerrors.NotFound,
	)

	svc := NewService(
		s.mockState,
		s.mockModelDeleter,
		loggertesting.WrapCheckLog(c),
	)

	_, _, err := svc.DefaultModelCloudNameAndCredential(context.Background())
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)

	// There exists to ways for the controller model to not be found. This is
	// asserting the second path where the code get's the uuid but the model
	// no longer exists for this uuid.
	ctrlModelUUID := modeltesting.GenModelUUID(c)
	s.mockState.EXPECT().GetControllerModelUUID(gomock.Any()).Return(
		ctrlModelUUID,
		nil,
	)
	s.mockState.EXPECT().GetModelCloudNameAndCredential(gomock.Any(), ctrlModelUUID).Return(
		"", credential.Key{}, modelerrors.NotFound,
	)

	_, _, err = svc.DefaultModelCloudNameAndCredential(context.Background())
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)
}

// TestDefaultModelCloudNameAndCredential is asserting the happy path that when
// a controller model exists the cloud name and credential are returned.
func (s *serviceSuite) TestDefaultModelCloudNameAndCredential(c *gc.C) {
	defer s.setupMocks(c).Finish()

	svc := NewService(
		s.mockState,
		s.mockModelDeleter,
		loggertesting.WrapCheckLog(c),
	)

	// There exists to ways for the controller model to not be found. This is
	// asserting the second path where the code get's the uuid but the model
	// no longer exists for this uuid.
	ctrlModelUUID := modeltesting.GenModelUUID(c)
	s.mockState.EXPECT().GetControllerModelUUID(gomock.Any()).Return(
		ctrlModelUUID,
		nil,
	)
	s.mockState.EXPECT().GetModelCloudNameAndCredential(gomock.Any(), ctrlModelUUID).Return(
		"test",
		credential.Key{
			Cloud: "test",
			Owner: usertesting.GenNewName(c, "admin"),
			Name:  "test-cred",
		},
		nil,
	)

	cloud, cred, err := svc.DefaultModelCloudNameAndCredential(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(cloud, gc.Equals, "test")
	c.Check(cred, jc.DeepEquals, credential.Key{
		Cloud: "test",
		Owner: usertesting.GenNewName(c, "admin"),
		Name:  "test-cred",
	})
}

// TestWatchActivatedModels verifies that WatchActivatedModels correctly sets up a watcher
// that emits events for activated models when the watcher receives change events.
func (s *serviceSuite) TestWatchActivatedModels(c *gc.C) {
	defer s.setupMocks(c).Finish()
	ctx := context.Background()
	svc := NewWatchableService(
		s.mockState,
		s.mockModelDeleter,
		loggertesting.WrapCheckLog(c),
		s.mockWatcherFactory,
	)

	// Set up necessary mock return values.
	s.mockState.EXPECT().InitialWatchActivatedModelsStatement().Return(
		"SELECT uuid from model WHERE activated = true",
	)

	changes := make(chan []string, 1)
	activatedModelUUID1 := modeltesting.GenModelUUID(c)
	activatedModelUUID2 := modeltesting.GenModelUUID(c)
	activatedModelUUIDs := []coremodel.UUID{activatedModelUUID1, activatedModelUUID2}
	activatedModelUUIDsStr := transform.Slice(activatedModelUUIDs, func(uuid coremodel.UUID) string {
		return uuid.String()
	})
	changes <- activatedModelUUIDsStr
	close(changes)
	s.mockStringsWatcher.EXPECT().Changes().AnyTimes().Return(changes)

	s.mockWatcherFactory.EXPECT().NewNamespaceMapperWatcher(
		gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(s.mockStringsWatcher, nil)

	// Verifies that the service returns a watcher with the correct model UUIDs string.
	watcher, err := svc.WatchActivatedModels(ctx)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(<-watcher.Changes(), gc.DeepEquals, activatedModelUUIDsStr)
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
func (s *serviceSuite) TestWatchActivatedModelsMapper(c *gc.C) {
	defer s.setupMocks(c).Finish()
	ctx := context.Background()

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
	retrievedChangeEvents, err := mapper(ctx, s.ControllerTxnRunner(), inputChangeEvents)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(retrievedChangeEvents, gc.DeepEquals, expectedChangeEvents)
}
