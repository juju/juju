// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	. "gopkg.in/check.v1"

	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/credential"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	usererrors "github.com/juju/juju/domain/user/errors"
	"github.com/juju/juju/internal/uuid"
	jujuversion "github.com/juju/juju/version"
)

type dummyStateCloud struct {
	Credentials map[string]credential.ID
	Regions     []string
}

type dummyState struct {
	clouds         map[string]dummyStateCloud
	models         map[coremodel.UUID]model.ModelCreationArgs
	secretBackends map[coremodel.UUID]model.SecretBackendIdentifier
	users          set.Strings
}

type serviceSuite struct {
	testing.IsolationSuite

	modelUUID coremodel.UUID
	userUUID  user.UUID
	state     *dummyState
}

var _ = Suite(&serviceSuite{})

func (d *dummyState) Create(
	_ context.Context,
	uuid coremodel.UUID,
	args model.ModelCreationArgs,
) error {
	if _, exists := d.models[uuid]; exists {
		return fmt.Errorf("%w %q", modelerrors.AlreadyExists, uuid)
	}

	for _, v := range d.models {
		if v.Name == args.Name && v.Owner == args.Owner {
			return fmt.Errorf("%w for name %q and owner %q", modelerrors.AlreadyExists, v.Name, v.Owner)
		}
	}

	cloud, exists := d.clouds[args.Cloud]
	if !exists {
		return fmt.Errorf("%w cloud %q", errors.NotFound, args.Cloud)
	}

	if !d.users.Contains(args.Owner.String()) {
		return fmt.Errorf("%w for owner %q", usererrors.NotFound, args.Owner)
	}

	hasRegion := false
	for _, region := range cloud.Regions {
		if region == args.CloudRegion {
			hasRegion = true
		}
	}
	if !hasRegion {
		return fmt.Errorf("%w cloud %q region %q", errors.NotFound, args.Cloud, args.CloudRegion)
	}

	if !args.Credential.IsZero() {
		if _, exists := cloud.Credentials[args.Credential.String()]; !exists {
			return fmt.Errorf("%w credential %q", errors.NotFound, args.Credential.String())
		}
	}

	d.models[uuid] = args
	return nil
}

func (d *dummyState) Delete(
	_ context.Context,
	uuid coremodel.UUID,
) error {
	if _, exists := d.models[uuid]; !exists {
		return fmt.Errorf("%w %q", modelerrors.NotFound, uuid)
	}
	delete(d.models, uuid)
	return nil
}

func (d *dummyState) List(
	_ context.Context,
) ([]coremodel.UUID, error) {
	rval := make([]coremodel.UUID, 0, len(d.models))
	for k := range d.models {
		rval = append(rval, k)
	}

	return rval, nil
}

func (d *dummyState) UpdateCredential(
	_ context.Context,
	uuid coremodel.UUID,
	credentialId credential.ID,
) error {
	info, exists := d.models[uuid]
	if !exists {
		return fmt.Errorf("%w %q", modelerrors.NotFound, uuid)
	}

	cloud, exists := d.clouds[credentialId.Cloud]
	if !exists {
		return fmt.Errorf("%w cloud %q", errors.NotFound, credentialId.Cloud)
	}

	if _, exists := cloud.Credentials[credentialId.String()]; !exists {
		return fmt.Errorf("%w credential %q", errors.NotFound, credentialId.String())
	}

	if info.Cloud != credentialId.Cloud {
		return fmt.Errorf("%w credential cloud is different to that of the model", errors.NotValid)
	}

	return nil
}

func (d *dummyState) Get(_ context.Context, uuid coremodel.UUID) (*model.Model, error) {
	args, exists := d.models[uuid]
	if !exists {
		return nil, fmt.Errorf("%w %q", modelerrors.NotFound, uuid)
	}
	return &model.Model{
		UUID:      uuid.String(),
		Name:      args.Name,
		ModelType: args.Type.String(),
	}, nil
}

func (d *dummyState) SetSecretBackend(_ context.Context, modelUUID coremodel.UUID, backendName string) error {
	if _, exists := d.models[modelUUID]; !exists {
		return fmt.Errorf("%w %q", modelerrors.NotFound, modelUUID)
	}
	if d.secretBackends == nil {
		d.secretBackends = map[coremodel.UUID]model.SecretBackendIdentifier{}
	}
	d.secretBackends[modelUUID] = model.SecretBackendIdentifier{
		UUID: uuid.MustNewUUID().String(),
		Name: backendName,
	}
	return nil
}

func (d *dummyState) GetSecretBackend(_ context.Context, modelUUID coremodel.UUID) (model.SecretBackendIdentifier, error) {
	if _, exists := d.models[modelUUID]; !exists {
		return model.SecretBackendIdentifier{}, fmt.Errorf("%w %q", modelerrors.NotFound, modelUUID)
	}
	return d.secretBackends[modelUUID], nil
}

func (s *serviceSuite) SetUpTest(c *C) {
	s.modelUUID = modeltesting.GenModelUUID(c)
	var err error
	s.userUUID, err = user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	s.state = &dummyState{
		clouds: map[string]dummyStateCloud{},
		models: map[coremodel.UUID]model.ModelCreationArgs{},
		users:  set.NewStrings(s.userUUID.String()),
	}
}

func (s *serviceSuite) TestCreateModelInvalidArgs(c *C) {
	svc := NewService(s.state, DefaultAgentBinaryFinder())
	_, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{})
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *serviceSuite) TestModelCreation(c *C) {
	cred := credential.ID{
		Cloud: "aws",
		Name:  "foobar",
		Owner: s.userUUID.String(),
	}
	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.ID{
			cred.String(): cred,
		},
		Regions: []string{"myregion"},
	}

	svc := NewService(s.state, DefaultAgentBinaryFinder())
	id, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Credential:  cred,
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
		Type:        coremodel.IAAS,
	})

	c.Assert(err, jc.ErrorIsNil)

	args, exists := s.state.models[id]
	c.Assert(exists, jc.IsTrue)

	// Test that because we have not specified an agent version that the current
	// controller version is chosen.
	c.Check(args.AgentVersion, Equals, jujuversion.Current)

	modelList, err := svc.ModelList(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(modelList, DeepEquals, []coremodel.UUID{
		id,
	})
}

func (s *serviceSuite) TestModelCreationInvalidCloud(c *C) {
	svc := NewService(s.state, DefaultAgentBinaryFinder())
	_, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
		Type:        coremodel.IAAS,
	})

	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *serviceSuite) TestModelCreationNoCloudRegion(c *C) {
	s.state.clouds["aws"] = dummyStateCloud{
		Regions: []string{"myregion"},
	}

	svc := NewService(s.state, DefaultAgentBinaryFinder())
	_, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "noexist",
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
		Type:        coremodel.IAAS,
	})

	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

// TestModelCreationOwnerNotFound is testing that if we make a model with an
// owner that doesn't exist we get back a [usererrors.NotFound] error.
func (s *serviceSuite) TestModelCreationOwnerNotFound(c *C) {
	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.ID{},
		Regions:     []string{"myregion"},
	}

	notFoundUser, err := user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	svc := NewService(s.state, DefaultAgentBinaryFinder())
	_, err = svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Owner:       notFoundUser,
		Name:        "my-awesome-model",
		Type:        coremodel.IAAS,
	})

	c.Assert(err, jc.ErrorIs, usererrors.NotFound)
}

func (s *serviceSuite) TestModelCreationNoCloudCredential(c *C) {
	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.ID{},
		Regions:     []string{"myregion"},
	}

	svc := NewService(s.state, DefaultAgentBinaryFinder())
	_, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Credential: credential.ID{
			Cloud: "aws",
			Name:  "foo",
			Owner: s.userUUID.String(),
		},
		Owner: s.userUUID,
		Name:  "my-awesome-model",
		Type:  coremodel.IAAS,
	})

	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *serviceSuite) TestModelCreationNameOwnerConflict(c *C) {
	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.ID{},
		Regions:     []string{"myregion"},
	}

	svc := NewService(s.state, DefaultAgentBinaryFinder())
	_, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
		Type:        coremodel.IAAS,
	})

	c.Assert(err, jc.ErrorIsNil)

	_, err = svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
		Type:        coremodel.IAAS,
	})

	c.Assert(err, jc.ErrorIs, modelerrors.AlreadyExists)
}

func (s *serviceSuite) TestUpdateModelCredentialForInvalidModel(c *C) {
	id := modeltesting.GenModelUUID(c)

	svc := NewService(s.state, DefaultAgentBinaryFinder())
	err := svc.UpdateCredential(context.Background(), id, credential.ID{
		Owner: s.userUUID.String(),
		Name:  "foo",
		Cloud: "aws",
	})
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

func (s *serviceSuite) TestUpdateModelCredential(c *C) {
	cred := credential.ID{
		Cloud: "aws",
		Owner: s.userUUID.String(),
		Name:  "foobar",
	}

	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.ID{
			cred.String(): cred,
		},
		Regions: []string{"myregion"},
	}

	svc := NewService(s.state, DefaultAgentBinaryFinder())
	id, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
		Type:        coremodel.IAAS,
	})

	c.Assert(err, jc.ErrorIsNil)

	err = svc.UpdateCredential(context.Background(), id, cred)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateModelCredentialReplace(c *C) {
	cred := credential.ID{
		Cloud: "aws",
		Owner: s.userUUID.String(),
		Name:  "foobar",
	}
	cred2 := credential.ID{
		Cloud: "aws",
		Owner: s.userUUID.String(),
		Name:  "foobar2",
	}

	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.ID{
			cred.String():  cred,
			cred2.String(): cred2,
		},
		Regions: []string{"myregion"},
	}

	svc := NewService(s.state, DefaultAgentBinaryFinder())
	id, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Credential:  cred,
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
		Type:        coremodel.IAAS,
	})

	c.Assert(err, jc.ErrorIsNil)

	err = svc.UpdateCredential(context.Background(), id, cred2)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateModelCredentialZeroValue(c *C) {
	cred := credential.ID{
		Cloud: "aws",
		Owner: s.userUUID.String(),
		Name:  "foobar",
	}

	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.ID{
			cred.String(): cred,
		},
		Regions: []string{"myregion"},
	}

	svc := NewService(s.state, DefaultAgentBinaryFinder())
	id, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
		Type:        coremodel.IAAS,
	})

	c.Assert(err, jc.ErrorIsNil)

	err = svc.UpdateCredential(context.Background(), id, credential.ID{})
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *serviceSuite) TestUpdateModelCredentialDifferentCloud(c *C) {
	cred := credential.ID{
		Cloud: "aws",
		Owner: s.userUUID.String(),
		Name:  "foobar",
	}
	cred2 := credential.ID{
		Cloud: "kubernetes",
		Owner: s.userUUID.String(),
		Name:  "foobar2",
	}

	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.ID{
			cred.String(): cred,
		},
		Regions: []string{"myregion"},
	}
	s.state.clouds["kubernetes"] = dummyStateCloud{
		Credentials: map[string]credential.ID{
			cred2.String(): cred2,
		},
		Regions: []string{"myregion"},
	}

	svc := NewService(s.state, DefaultAgentBinaryFinder())
	id, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Credential:  cred,
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
		Type:        coremodel.IAAS,
	})

	c.Assert(err, jc.ErrorIsNil)

	err = svc.UpdateCredential(context.Background(), id, cred2)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *serviceSuite) TestUpdateModelCredentialNotFound(c *C) {
	cred := credential.ID{
		Cloud: "aws",
		Owner: s.userUUID.String(),
		Name:  "foobar",
	}
	cred2 := credential.ID{
		Cloud: "aws",
		Owner: s.userUUID.String(),
		Name:  "foobar2",
	}

	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.ID{
			cred.String(): cred,
		},
		Regions: []string{"myregion"},
	}

	svc := NewService(s.state, DefaultAgentBinaryFinder())
	id, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Credential:  cred,
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
		Type:        coremodel.IAAS,
	})

	c.Assert(err, jc.ErrorIsNil)

	err = svc.UpdateCredential(context.Background(), id, cred2)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *serviceSuite) TestDeleteModel(c *C) {
	cred := credential.ID{
		Cloud: "aws",
		Name:  "foobar",
		Owner: s.userUUID.String(),
	}
	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.ID{
			cred.String(): cred,
		},
		Regions: []string{"myregion"},
	}

	svc := NewService(s.state, DefaultAgentBinaryFinder())
	id, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Credential:  cred,
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
		Type:        coremodel.IAAS,
	})
	c.Assert(err, jc.ErrorIsNil)

	_, exists := s.state.models[id]
	c.Assert(exists, jc.IsTrue)

	err = svc.DeleteModel(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	_, exists = s.state.models[id]
	c.Assert(exists, jc.IsFalse)
}

func (s *serviceSuite) TestDeleteModelNotFound(c *C) {
	svc := NewService(s.state, DefaultAgentBinaryFinder())
	err := svc.DeleteModel(context.Background(), s.modelUUID)
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

// TestAgentVersionUnsupportedGreater is asserting that if we try and create a
// model with an agent version that is greater then that of the controller the
// operation fails with a [modelerrors.AgentVersionNotSupported] error.
func (s *serviceSuite) TestAgentVersionUnsupportedGreater(c *C) {
	cred := credential.ID{
		Cloud: "aws",
		Name:  "foobar",
		Owner: s.userUUID.String(),
	}
	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.ID{
			cred.String(): cred,
		},
		Regions: []string{"myregion"},
	}

	agentVersion, err := version.Parse("99.9.9")
	c.Assert(err, jc.ErrorIsNil)

	svc := NewService(s.state, DefaultAgentBinaryFinder())
	id, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		AgentVersion: agentVersion,
		Cloud:        "aws",
		CloudRegion:  "myregion",
		Credential:   cred,
		Owner:        s.userUUID,
		Name:         "my-awesome-model",
		Type:         coremodel.IAAS,
	})

	c.Assert(err, jc.ErrorIs, modelerrors.AgentVersionNotSupported)

	_, exists := s.state.models[id]
	c.Assert(exists, jc.IsFalse)
}

// TestAgentVersionUnsupportedGreater is asserting that if we try and create a
// model with an agent version that is less then that of the controller the
// operation fails with a [modelerrors.AgentVersionNotSupported] error. This
// fails because find tools will report [errors.NotFound].
func (s *serviceSuite) TestAgentVersionUnsupportedLess(c *C) {
	cred := credential.ID{
		Cloud: "aws",
		Name:  "foobar",
		Owner: s.userUUID.String(),
	}
	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.ID{
			cred.String(): cred,
		},
		Regions: []string{"myregion"},
	}

	agentVersion, err := version.Parse("1.9.9")
	c.Assert(err, jc.ErrorIsNil)

	svc := NewService(s.state, DefaultAgentBinaryFinder())
	id, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		AgentVersion: agentVersion,
		Cloud:        "aws",
		CloudRegion:  "myregion",
		Credential:   cred,
		Owner:        s.userUUID,
		Name:         "my-awesome-model",
		Type:         coremodel.IAAS,
	})

	c.Assert(err, jc.ErrorIs, modelerrors.AgentVersionNotSupported)

	_, exists := s.state.models[id]
	c.Assert(exists, jc.IsFalse)
}

func (s *serviceSuite) TestGetSetSecretBackend(c *C) {
	cred := credential.ID{
		Cloud: "aws",
		Name:  "foobar",
		Owner: s.userUUID.String(),
	}
	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.ID{
			cred.String(): cred,
		},
		Regions: []string{"myregion"},
	}

	svc := NewService(s.state, DefaultAgentBinaryFinder())
	id, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Credential:  cred,
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
		Type:        coremodel.IAAS,
	})
	c.Assert(err, jc.ErrorIsNil)

	err = svc.SetSecretBackend(context.Background(), id, "my-backend")
	c.Assert(err, jc.ErrorIsNil)

	backend, err := svc.GetSecretBackend(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(backend, DeepEquals, model.SecretBackendIdentifier{
		UUID: backend.UUID,
		Name: "my-backend",
	})
}

func (s *serviceSuite) TestGetModel(c *C) {
	cred := credential.ID{
		Cloud: "aws",
		Name:  "foobar",
		Owner: s.userUUID.String(),
	}
	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.ID{
			cred.String(): cred,
		},
		Regions: []string{"myregion"},
	}

	svc := NewService(s.state, DefaultAgentBinaryFinder())
	id, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Credential:  cred,
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
		Type:        coremodel.IAAS,
	})
	c.Assert(err, jc.ErrorIsNil)

	result, err := svc.GetModel(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, DeepEquals, &coremodel.Model{
		UUID:      id.String(),
		Name:      "my-awesome-model",
		ModelType: "iaas",
	})
}
