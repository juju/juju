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
	. "gopkg.in/check.v1"

	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/credential"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	modeltesting "github.com/juju/juju/domain/model/testing"
	usererrors "github.com/juju/juju/domain/user/errors"
)

type dummyStateCloud struct {
	Credentials map[string]credential.ID
	Regions     []string
}

type dummyState struct {
	clouds map[string]dummyStateCloud
	models map[model.UUID]model.ModelCreationArgs
	users  set.Strings
}

type serviceSuite struct {
	testing.IsolationSuite

	modelUUID model.UUID
	userUUID  user.UUID
	state     *dummyState
}

var _ = Suite(&serviceSuite{})

func (d *dummyState) Create(
	_ context.Context,
	uuid model.UUID,
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
	uuid model.UUID,
) error {
	if _, exists := d.models[uuid]; !exists {
		return fmt.Errorf("%w %q", modelerrors.NotFound, uuid)
	}
	delete(d.models, uuid)
	return nil
}

func (d *dummyState) UpdateCredential(
	_ context.Context,
	uuid model.UUID,
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

func (s *serviceSuite) SetUpTest(c *C) {
	s.modelUUID = modeltesting.GenModelUUID(c)
	var err error
	s.userUUID, err = user.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	s.state = &dummyState{
		clouds: map[string]dummyStateCloud{},
		models: map[model.UUID]model.ModelCreationArgs{},
		users:  set.NewStrings(s.userUUID.String()),
	}
}

func (s *serviceSuite) TestCreateModelInvalidArgs(c *C) {
	svc := NewService(s.state)
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

	svc := NewService(s.state)
	id, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Credential:  cred,
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
		Type:        model.TypeIAAS,
	})

	c.Assert(err, jc.ErrorIsNil)

	_, exists := s.state.models[id]
	c.Assert(exists, jc.IsTrue)
}

func (s *serviceSuite) TestModelCreationInvalidCloud(c *C) {
	svc := NewService(s.state)
	_, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
		Type:        model.TypeIAAS,
	})

	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *serviceSuite) TestModelCreationNoCloudRegion(c *C) {
	s.state.clouds["aws"] = dummyStateCloud{
		Regions: []string{"myregion"},
	}

	svc := NewService(s.state)
	_, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "noexist",
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
		Type:        model.TypeIAAS,
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

	svc := NewService(s.state)
	_, err = svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Owner:       notFoundUser,
		Name:        "my-awesome-model",
		Type:        model.TypeIAAS,
	})

	c.Assert(err, jc.ErrorIs, usererrors.NotFound)
}

func (s *serviceSuite) TestModelCreationNoCloudCredential(c *C) {
	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.ID{},
		Regions:     []string{"myregion"},
	}

	svc := NewService(s.state)
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
		Type:  model.TypeIAAS,
	})

	c.Assert(err, jc.ErrorIs, errors.NotFound)
}

func (s *serviceSuite) TestModelCreationNameOwnerConflict(c *C) {
	s.state.clouds["aws"] = dummyStateCloud{
		Credentials: map[string]credential.ID{},
		Regions:     []string{"myregion"},
	}

	svc := NewService(s.state)
	_, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
		Type:        model.TypeIAAS,
	})

	c.Assert(err, jc.ErrorIsNil)

	_, err = svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
		Type:        model.TypeIAAS,
	})

	c.Assert(err, jc.ErrorIs, modelerrors.AlreadyExists)
}

func (s *serviceSuite) TestUpdateModelCredentialForInvalidModel(c *C) {
	id, err := model.NewUUID()
	c.Assert(err, jc.ErrorIsNil)

	svc := NewService(s.state)
	err = svc.UpdateCredential(context.Background(), id, credential.ID{
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

	svc := NewService(s.state)
	id, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
		Type:        model.TypeIAAS,
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

	svc := NewService(s.state)
	id, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Credential:  cred,
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
		Type:        model.TypeIAAS,
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

	svc := NewService(s.state)
	id, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
		Type:        model.TypeIAAS,
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

	svc := NewService(s.state)
	id, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Credential:  cred,
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
		Type:        model.TypeIAAS,
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

	svc := NewService(s.state)
	id, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Credential:  cred,
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
		Type:        model.TypeIAAS,
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

	svc := NewService(s.state)
	id, err := svc.CreateModel(context.Background(), model.ModelCreationArgs{
		Cloud:       "aws",
		CloudRegion: "myregion",
		Credential:  cred,
		Owner:       s.userUUID,
		Name:        "my-awesome-model",
		Type:        model.TypeIAAS,
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
	svc := NewService(s.state)
	err := svc.DeleteModel(context.Background(), s.modelUUID)
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}
