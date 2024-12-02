// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/credential"
	coreerrors "github.com/juju/juju/core/errors"
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
	svc := NewService(s.state, s.deleter, DefaultAgentBinaryFinder(), loggertesting.WrapCheckLog(c))
	_, _, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{})
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
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

	svc := NewService(s.state, s.deleter, DefaultAgentBinaryFinder(), loggertesting.WrapCheckLog(c))
	id, activator, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Credential:  cred,
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(activator(context.Background()), jc.ErrorIsNil)

	args, exists := s.state.models[id]
	c.Assert(exists, jc.IsTrue)

	// Test that because we have not specified an agent version that the current
	// controller version is chosen.
	c.Check(args.AgentVersion, gc.Equals, jujuversion.Current)

	modelList, err := svc.ListModelIDs(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(modelList, gc.DeepEquals, []coremodel.UUID{
		id,
	})
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

	svc := NewService(s.state, s.deleter, DefaultAgentBinaryFinder(), loggertesting.WrapCheckLog(c))
	_, _, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
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
	svc := NewService(s.state, s.deleter, DefaultAgentBinaryFinder(), loggertesting.WrapCheckLog(c))
	_, _, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
	})

	c.Assert(err, jc.ErrorIs, coreerrors.NotFound)
}

func (s *serviceSuite) TestModelCreationNoCloudRegion(c *gc.C) {
	s.state.clouds["aws"] = dummyStateCloud{
		Regions: []string{"myregion"},
	}

	svc := NewService(s.state, s.deleter, DefaultAgentBinaryFinder(), loggertesting.WrapCheckLog(c))
	_, _, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "noexist",
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
	})

	c.Assert(err, jc.ErrorIs, coreerrors.NotFound)
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

	svc := NewService(s.state, s.deleter, DefaultAgentBinaryFinder(), loggertesting.WrapCheckLog(c))
	_, _, err = svc.CreateModel(context.Background(), model.ModelCreationArgs{
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

	svc := NewService(s.state, s.deleter, DefaultAgentBinaryFinder(), loggertesting.WrapCheckLog(c))
	_, _, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
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

	c.Assert(err, jc.ErrorIs, coreerrors.NotFound)
}

func (s *serviceSuite) TestModelCreationNameOwnerConflict(c *gc.C) {
	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.Key{},
		Regions:     []string{"myregion"},
	}

	svc := NewService(s.state, s.deleter, DefaultAgentBinaryFinder(), loggertesting.WrapCheckLog(c))
	_, activator, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(activator(context.Background()), jc.ErrorIsNil)

	_, _, err = svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
	})

	c.Assert(err, jc.ErrorIs, modelerrors.AlreadyExists)
}

func (s *serviceSuite) TestUpdateModelCredentialForInvalidModel(c *gc.C) {
	id := modeltesting.GenModelUUID(c)

	svc := NewService(s.state, s.deleter, DefaultAgentBinaryFinder(), loggertesting.WrapCheckLog(c))
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

	svc := NewService(s.state, s.deleter, DefaultAgentBinaryFinder(), loggertesting.WrapCheckLog(c))
	id, activator, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
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

	svc := NewService(s.state, s.deleter, DefaultAgentBinaryFinder(), loggertesting.WrapCheckLog(c))
	id, activator, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
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

	svc := NewService(s.state, s.deleter, DefaultAgentBinaryFinder(), loggertesting.WrapCheckLog(c))
	id, activator, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(activator(context.Background()), jc.ErrorIsNil)

	err = svc.UpdateCredential(context.Background(), id, credential.Key{})
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
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

	svc := NewService(s.state, s.deleter, DefaultAgentBinaryFinder(), loggertesting.WrapCheckLog(c))
	id, activator, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Credential:  cred,
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(activator(context.Background()), jc.ErrorIsNil)

	err = svc.UpdateCredential(context.Background(), id, cred2)
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
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

	svc := NewService(s.state, s.deleter, DefaultAgentBinaryFinder(), loggertesting.WrapCheckLog(c))
	id, activator, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Credential:  cred,
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(activator(context.Background()), jc.ErrorIsNil)

	err = svc.UpdateCredential(context.Background(), id, cred2)
	c.Assert(err, jc.ErrorIs, coreerrors.NotFound)
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

	svc := NewService(s.state, s.deleter, DefaultAgentBinaryFinder(), loggertesting.WrapCheckLog(c))
	id, activator, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
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
	svc := NewService(s.state, notFoundDeleter{}, DefaultAgentBinaryFinder(), loggertesting.WrapCheckLog(c))
	err := svc.DeleteModel(context.Background(), modeltesting.GenModelUUID(c), model.WithDeleteDB())
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

// TestAgentVersionUnsupportedGreater is asserting that if we try and create a
// model with an agent version that is greater then that of the controller the
// operation fails with a [modelerrors.AgentVersionNotSupported] error.
func (s *serviceSuite) TestAgentVersionUnsupportedGreater(c *gc.C) {
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

	agentVersion, err := version.Parse("99.9.9")
	c.Assert(err, jc.ErrorIsNil)

	svc := NewService(s.state, s.deleter, DefaultAgentBinaryFinder(), loggertesting.WrapCheckLog(c))
	id, _, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		AgentVersion: agentVersion,
		Cloud:        "aws",
		CloudRegion:  "myregion",
		Credential:   cred,
		Owner:        s.userUUID,
		Name:         "my-awesome-model",
	})
	c.Assert(err, jc.ErrorIs, modelerrors.AgentVersionNotSupported)

	_, exists := s.state.models[id]
	c.Assert(exists, jc.IsFalse)
}

// TestAgentVersionUnsupportedLess is asserting that if we try and create a
// model with an agent version that is less then that of the controller.
func (s *serviceSuite) TestAgentVersionUnsupportedLess(c *gc.C) {
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

	agentVersion, err := version.Parse("1.9.9")
	c.Assert(err, jc.ErrorIsNil)

	svc := NewService(s.state, s.deleter, DefaultAgentBinaryFinder(), loggertesting.WrapCheckLog(c))
	id, _, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		AgentVersion: agentVersion,
		Cloud:        "aws",
		CloudRegion:  "myregion",
		Credential:   cred,
		Owner:        s.userUUID,
		Name:         "my-awesome-model",
	})

	// This is temporary until we implement tools metadata for the controller.
	c.Assert(err, jc.ErrorIsNil)

	_, exists := s.state.models[id]
	c.Assert(exists, jc.IsFalse)
}

// TestListAllModelsNoResults is asserting that when no models exist the return
// value of ListAllModels is an empty slice.
func (s *serviceSuite) TestListAllModelsNoResults(c *gc.C) {
	svc := NewService(s.state, s.deleter, DefaultAgentBinaryFinder(), loggertesting.WrapCheckLog(c))
	models, err := svc.ListAllModels(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(len(models), gc.Equals, 0)
}

func (s *serviceSuite) TestListAllModels(c *gc.C) {
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

	svc := NewService(s.state, s.deleter, DefaultAgentBinaryFinder(), loggertesting.WrapCheckLog(c))
	id1, activator, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		AgentVersion: jujuversion.Current,
		Cloud:        "aws",
		CloudRegion:  "myregion",
		Credential:   cred,
		Owner:        s.userUUID,
		Name:         "my-awesome-model",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(activator(context.Background()), jc.ErrorIsNil)

	id2, activator, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		AgentVersion: jujuversion.Current,
		Cloud:        "aws",
		CloudRegion:  "myregion",
		Credential:   cred,
		Owner:        usr1,
		Name:         "my-awesome-model1",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(activator(context.Background()), jc.ErrorIsNil)

	models, err := svc.ListAllModels(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	slices.SortFunc(models, func(a, b coremodel.Model) int {
		return strings.Compare(a.Name, b.Name)
	})

	c.Check(models, gc.DeepEquals, []coremodel.Model{
		{
			Name:         "my-awesome-model",
			AgentVersion: jujuversion.Current,
			UUID:         id1,
			Cloud:        "aws",
			CloudRegion:  "myregion",
			ModelType:    coremodel.IAAS,
			Owner:        s.userUUID,
			OwnerName:    usertesting.GenNewName(c, "admin"),
			Credential:   cred,
			Life:         life.Alive,
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
			Credential:   cred,
			Life:         life.Alive,
		},
	})
}

// TestListModelsForUser is asserting that for a non existent user we return
// an empty model result.
func (s *serviceSuite) TestListModelsForNonExistentUser(c *gc.C) {
	fakeUserID := usertesting.GenUserUUID(c)
	svc := NewService(s.state, s.deleter, DefaultAgentBinaryFinder(), loggertesting.WrapCheckLog(c))
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

	svc := NewService(s.state, s.deleter, DefaultAgentBinaryFinder(), loggertesting.WrapCheckLog(c))
	id1, activator, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		AgentVersion: jujuversion.Current,
		Cloud:        "aws",
		CloudRegion:  "myregion",
		Credential:   cred,
		Owner:        usr1,
		Name:         "my-awesome-model",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(activator(context.Background()), jc.ErrorIsNil)

	id2, activator, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		AgentVersion: jujuversion.Current,
		Cloud:        "aws",
		CloudRegion:  "myregion",
		Credential:   cred,
		Owner:        usr1,
		Name:         "my-awesome-model1",
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
			Name:         "my-awesome-model",
			AgentVersion: jujuversion.Current,
			UUID:         id1,
			Cloud:        "aws",
			CloudRegion:  "myregion",
			ModelType:    coremodel.IAAS,
			Owner:        usr1,
			OwnerName:    usertesting.GenNewName(c, "tlm"),
			Credential:   cred,
			Life:         life.Alive,
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
			Credential:   cred,
			Life:         life.Alive,
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

	svc := NewService(s.state, s.deleter, DefaultAgentBinaryFinder(), loggertesting.WrapCheckLog(c))
	_, err := svc.ImportModel(context.Background(), model.ModelImportArgs{
		ModelCreationArgs: model.ModelCreationArgs{
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

	svc := NewService(s.state, s.deleter, DefaultAgentBinaryFinder(), loggertesting.WrapCheckLog(c))
	activator, err := svc.ImportModel(context.Background(), model.ModelImportArgs{
		ModelCreationArgs: model.ModelCreationArgs{
			Cloud:        "aws",
			CloudRegion:  "myregion",
			Credential:   cred,
			Owner:        s.userUUID,
			Name:         "my-awesome-model",
			AgentVersion: jujuversion.Current,
		},
		ID: modelID,
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
	svc := NewService(s.state, s.deleter, DefaultAgentBinaryFinder(), loggertesting.WrapCheckLog(c))
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

	svc := NewService(s.state, s.deleter, DefaultAgentBinaryFinder(), loggertesting.WrapCheckLog(c))
	modelID, activator, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		AgentVersion: jujuversion.Current,
		Cloud:        "aws",
		CloudRegion:  "myregion",
		Credential:   cred,
		Owner:        adminUUID,
		Name:         coremodel.ControllerModelName,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(activator(context.Background()), jc.ErrorIsNil)
	s.state.controllerModelUUID = modelID

	model, err := svc.ControllerModel(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(model, gc.DeepEquals, coremodel.Model{
		Name:         coremodel.ControllerModelName,
		Life:         life.Alive,
		UUID:         modelID,
		ModelType:    coremodel.IAAS,
		AgentVersion: jujuversion.Current,
		Cloud:        "aws",
		CloudRegion:  "myregion",
		Credential:   cred,
		Owner:        adminUUID,
		OwnerName:    coremodel.ControllerModelOwnerUsername,
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
	svc := NewService(s.state, s.deleter, DefaultAgentBinaryFinder(), loggertesting.WrapCheckLog(c))
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
	svc := NewService(s.state, s.deleter, DefaultAgentBinaryFinder(), loggertesting.WrapCheckLog(c))
	_, err := svc.GetModelUsers(context.Background(), "bad-uuid)")
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
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
	svc := NewService(s.state, s.deleter, DefaultAgentBinaryFinder(), loggertesting.WrapCheckLog(c))
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
	svc := NewService(s.state, s.deleter, DefaultAgentBinaryFinder(), loggertesting.WrapCheckLog(c))
	_, err := svc.GetModelUser(context.Background(), "bad-uuid", usertesting.GenNewName(c, "bob"))
	c.Assert(err, jc.ErrorIs, coreerrors.NotValid)
}

func (s *serviceSuite) TestGetModelUserZeroUserName(c *gc.C) {
	svc := NewService(s.state, s.deleter, DefaultAgentBinaryFinder(), loggertesting.WrapCheckLog(c))
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
	svc := NewService(s.state, s.deleter, DefaultAgentBinaryFinder(), loggertesting.WrapCheckLog(c))
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
	svc := NewService(s.state, s.deleter, DefaultAgentBinaryFinder(), loggertesting.WrapCheckLog(c))
	_, err := svc.ListModelsForUser(context.Background(), "((*)(")
	c.Check(err, jc.ErrorIs, coreerrors.NotValid)
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
	svc := NewService(s.state, s.deleter, DefaultAgentBinaryFinder(), loggertesting.WrapCheckLog(c))
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
