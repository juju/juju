// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/user"
	usererrors "github.com/juju/juju/domain/access/errors"
	clouderrors "github.com/juju/juju/domain/cloud/errors"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	jujuversion "github.com/juju/juju/version"
)

type dummyStateCloud struct {
	Credentials map[string]credential.Key
	Regions     []string
}

type dummyState struct {
	clouds map[string]dummyStateCloud
	models map[coremodel.UUID]coremodel.Model
	users  map[user.UUID]string
}

type serviceSuite struct {
	testing.IsolationSuite

	modelUUID coremodel.UUID
	userUUID  user.UUID
	state     *dummyState
}

var _ = gc.Suite(&serviceSuite{})

func (d *dummyState) CloudType(
	_ context.Context,
	name string,
) (string, error) {
	_, exists := d.clouds[name]
	if !exists {
		return "", clouderrors.NotFound
	}

	return "aws", nil
}

func (d *dummyState) Create(
	_ context.Context,
	uuid coremodel.UUID,
	modelType coremodel.ModelType,
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

	_, exists = d.users[user.UUID(args.Owner.String())]
	if !exists {
		return fmt.Errorf("%w for owner %q", usererrors.UserNotFound, args.Owner)
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

	d.models[uuid] = coremodel.Model{
		AgentVersion: args.AgentVersion,
		Name:         args.Name,
		UUID:         uuid,
		ModelType:    modelType,
		Cloud:        args.Cloud,
		CloudRegion:  args.CloudRegion,
		Credential:   args.Credential,
		Owner:        args.Owner,
	}
	return nil
}

func (d *dummyState) Get(
	_ context.Context,
	uuid coremodel.UUID,
) (coremodel.Model, error) {
	info, exists := d.models[uuid]
	if !exists {
		return coremodel.Model{}, fmt.Errorf("%w %q", modelerrors.NotFound, uuid)
	}
	return info, nil
}

func (d *dummyState) GetModelType(
	_ context.Context,
	uuid coremodel.UUID,
) (coremodel.ModelType, error) {
	info, exists := d.models[uuid]
	if !exists {
		return "", fmt.Errorf("%w %q", modelerrors.NotFound, uuid)
	}
	return info.ModelType, nil
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

func (d *dummyState) ModelCloudNameAndCredential(
	_ context.Context,
	modelName string,
	ownerName string,
) (string, credential.Key, error) {
	var ownerUUID user.UUID
	for k, v := range d.users {
		if v == ownerName {
			ownerUUID = k
		}
	}

	for _, model := range d.models {
		if model.Owner == ownerUUID && model.Name == modelName {
			return model.Cloud, model.Credential, nil
		}
	}
	return "", credential.Key{}, modelerrors.NotFound
}

func (d *dummyState) UpdateCredential(
	_ context.Context,
	uuid coremodel.UUID,
	credentialKey credential.Key,
) error {
	info, exists := d.models[uuid]
	if !exists {
		return fmt.Errorf("%w %q", modelerrors.NotFound, uuid)
	}

	cloud, exists := d.clouds[credentialKey.Cloud]
	if !exists {
		return fmt.Errorf("%w cloud %q", errors.NotFound, credentialKey.Cloud)
	}

	if _, exists := cloud.Credentials[credentialKey.String()]; !exists {
		return fmt.Errorf("%w credential %q", errors.NotFound, credentialKey.String())
	}

	if info.Cloud != credentialKey.Cloud {
		return fmt.Errorf("%w credential cloud is different to that of the model", errors.NotValid)
	}

	return nil
}

func (s *serviceSuite) SetUpTest(c *gc.C) {
	s.modelUUID = modeltesting.GenModelUUID(c)
	var err error
	s.userUUID, err = user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	s.state = &dummyState{
		clouds: map[string]dummyStateCloud{},
		models: map[coremodel.UUID]coremodel.Model{},
		users: map[user.UUID]string{
			s.userUUID: "admin",
		},
	}
}

func (s *serviceSuite) TestCreateModelInvalidArgs(c *gc.C) {
	svc := NewService(s.state, DefaultAgentBinaryFinder())
	_, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{})
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *serviceSuite) TestModelCreation(c *gc.C) {
	cred := credential.Key{
		Cloud: "aws",
		Name:  "foobar",
		Owner: s.userUUID.String(),
	}
	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.Key{
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
	})

	c.Assert(err, jc.ErrorIsNil)

	args, exists := s.state.models[id]
	c.Assert(exists, jc.IsTrue)

	// Test that because we have not specified an agent version that the current
	// controller version is chosen.
	c.Check(args.AgentVersion, gc.Equals, jujuversion.Current)

	modelList, err := svc.ModelList(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(modelList, gc.DeepEquals, []coremodel.UUID{
		id,
	})
}

func (s *serviceSuite) TestModelCreationInvalidCloud(c *gc.C) {
	s.state.clouds["aws"] = dummyStateCloud{}
	svc := NewService(s.state, DefaultAgentBinaryFinder())
	_, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
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

	svc := NewService(s.state, DefaultAgentBinaryFinder())
	_, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "noexist",
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
	})

	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

// TestModelCreationOwnerNotFound is testing that if we make a model with an
// owner that doesn't exist we get back a [usererrors.NotFound] error.
func (s *serviceSuite) TestModelCreationOwnerNotFound(c *gc.C) {
	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.Key{},
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
	})

	c.Assert(err, jc.ErrorIs, usererrors.UserNotFound)
}

func (s *serviceSuite) TestModelCreationNoCloudCredential(c *gc.C) {
	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.Key{},
		Regions:     []string{"myregion"},
	}

	svc := NewService(s.state, DefaultAgentBinaryFinder())
	_, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Credential: credential.Key{
			Cloud: "aws",
			Name:  "foo",
			Owner: s.userUUID.String(),
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

	svc := NewService(s.state, DefaultAgentBinaryFinder())
	_, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
	})

	c.Assert(err, jc.ErrorIsNil)

	_, err = svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
	})

	c.Assert(err, jc.ErrorIs, modelerrors.AlreadyExists)
}

func (s *serviceSuite) TestUpdateModelCredentialForInvalidModel(c *gc.C) {
	id := modeltesting.GenModelUUID(c)

	svc := NewService(s.state, DefaultAgentBinaryFinder())
	err := svc.UpdateCredential(context.Background(), id, credential.Key{
		Owner: s.userUUID.String(),
		Name:  "foo",
		Cloud: "aws",
	})
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

func (s *serviceSuite) TestUpdateModelCredential(c *gc.C) {
	cred := credential.Key{
		Cloud: "aws",
		Owner: s.userUUID.String(),
		Name:  "foobar",
	}

	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.Key{
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
	})

	c.Assert(err, jc.ErrorIsNil)

	err = svc.UpdateCredential(context.Background(), id, cred)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateModelCredentialReplace(c *gc.C) {
	cred := credential.Key{
		Cloud: "aws",
		Owner: s.userUUID.String(),
		Name:  "foobar",
	}
	cred2 := credential.Key{
		Cloud: "aws",
		Owner: s.userUUID.String(),
		Name:  "foobar2",
	}

	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.Key{
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
	})

	c.Assert(err, jc.ErrorIsNil)

	err = svc.UpdateCredential(context.Background(), id, cred2)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateModelCredentialZeroValue(c *gc.C) {
	cred := credential.Key{
		Cloud: "aws",
		Owner: s.userUUID.String(),
		Name:  "foobar",
	}

	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.Key{
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
	})

	c.Assert(err, jc.ErrorIsNil)

	err = svc.UpdateCredential(context.Background(), id, credential.Key{})
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *serviceSuite) TestUpdateModelCredentialDifferentCloud(c *gc.C) {
	cred := credential.Key{
		Cloud: "aws",
		Owner: s.userUUID.String(),
		Name:  "foobar",
	}
	cred2 := credential.Key{
		Cloud: "kubernetes",
		Owner: s.userUUID.String(),
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

	svc := NewService(s.state, DefaultAgentBinaryFinder())
	id, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Credential:  cred,
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
	})

	c.Assert(err, jc.ErrorIsNil)

	err = svc.UpdateCredential(context.Background(), id, cred2)
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *serviceSuite) TestUpdateModelCredentialNotFound(c *gc.C) {
	cred := credential.Key{
		Cloud: "aws",
		Owner: s.userUUID.String(),
		Name:  "foobar",
	}
	cred2 := credential.Key{
		Cloud: "aws",
		Owner: s.userUUID.String(),
		Name:  "foobar2",
	}

	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.Key{
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
	})

	c.Assert(err, jc.ErrorIsNil)

	err = svc.UpdateCredential(context.Background(), id, cred2)
	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *serviceSuite) TestDeleteModel(c *gc.C) {
	cred := credential.Key{
		Cloud: "aws",
		Name:  "foobar",
		Owner: s.userUUID.String(),
	}
	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.Key{
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
	})
	c.Assert(err, jc.ErrorIsNil)

	_, exists := s.state.models[id]
	c.Assert(exists, jc.IsTrue)

	err = svc.DeleteModel(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	_, exists = s.state.models[id]
	c.Assert(exists, jc.IsFalse)
}

func (s *serviceSuite) TestDeleteModelNotFound(c *gc.C) {
	svc := NewService(s.state, DefaultAgentBinaryFinder())
	err := svc.DeleteModel(context.Background(), s.modelUUID)
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

// TestAgentVersionUnsupportedGreater is asserting that if we try and create a
// model with an agent version that is greater then that of the controller the
// operation fails with a [modelerrors.AgentVersionNotSupported] error.
func (s *serviceSuite) TestAgentVersionUnsupportedGreater(c *gc.C) {
	cred := credential.Key{
		Cloud: "aws",
		Name:  "foobar",
		Owner: s.userUUID.String(),
	}
	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.Key{
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
	})

	c.Assert(err, jc.ErrorIs, modelerrors.AgentVersionNotSupported)

	_, exists := s.state.models[id]
	c.Assert(exists, jc.IsFalse)
}

// TestAgentVersionUnsupportedGreater is asserting that if we try and create a
// model with an agent version that is less then that of the controller the
// operation fails with a [modelerrors.AgentVersionNotSupported] error. This
// fails because find tools will report [errors.NotFound].
func (s *serviceSuite) TestAgentVersionUnsupportedLess(c *gc.C) {
	cred := credential.Key{
		Cloud: "aws",
		Name:  "foobar",
		Owner: s.userUUID.String(),
	}
	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.Key{
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
	})

	c.Assert(err, jc.ErrorIs, modelerrors.AgentVersionNotSupported)

	_, exists := s.state.models[id]
	c.Assert(exists, jc.IsFalse)
}
