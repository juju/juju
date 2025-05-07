// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	corecredential "github.com/juju/juju/core/credential"
	credentialtesting "github.com/juju/juju/core/credential/testing"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain/credential"
	credentialerrors "github.com/juju/juju/domain/credential/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	jujutesting "github.com/juju/juju/internal/testing"
)

type serviceSuite struct {
	baseSuite
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) service(c *gc.C) *WatchableService {
	return NewWatchableService(s.state, s.watcherFactory, loggertesting.WrapCheckLog(c))
}

func (s *serviceSuite) TestUpdateCloudCredential(c *gc.C) {
	defer s.setupMocks(c).Finish()

	key := corecredential.Key{Cloud: "cirrus", Owner: usertesting.GenNewName(c, "fred"), Name: "foo"}
	cred := credential.CloudCredentialInfo{
		AuthType: string(cloud.UserPassAuthType),
		Attributes: map[string]string{
			"hello": "world",
		},
		Label: "foo",
	}
	s.state.EXPECT().UpsertCloudCredential(gomock.Any(), key, cred)

	err := s.service(c).UpdateCloudCredential(
		context.Background(), key,
		cloud.NewNamedCredential("foo", cloud.UserPassAuthType, map[string]string{"hello": "world"}, false))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateCloudCredentialInvalidID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	key := corecredential.Key{Cloud: "cirrus", Owner: usertesting.GenNewName(c, "fred")}
	err := s.service(c).UpdateCloudCredential(context.Background(), key, cloud.Credential{})
	c.Assert(err, gc.ErrorMatches, "invalid id updating cloud credential.*")
}

func (s *serviceSuite) TestCloudCredentials(c *gc.C) {
	defer s.setupMocks(c).Finish()

	one := credential.CloudCredentialResult{
		CloudCredentialInfo: credential.CloudCredentialInfo{
			AuthType: string(cloud.UserPassAuthType),
			Attributes: map[string]string{
				"hello": "world",
			},
			Label: "foo",
		},
	}
	two := credential.CloudCredentialResult{
		CloudCredentialInfo: credential.CloudCredentialInfo{
			AuthType: string(cloud.UserPassAuthType),
			Attributes: map[string]string{
				"hello": "world",
			},
			Label: "foobar",
		},
	}
	s.state.EXPECT().CloudCredentialsForOwner(gomock.Any(), usertesting.GenNewName(c, "fred"), "cirrus").Return(map[string]credential.CloudCredentialResult{
		"foo":    one,
		"foobar": two,
	}, nil)

	creds, err := s.service(c).CloudCredentialsForOwner(context.Background(), usertesting.GenNewName(c, "fred"), "cirrus")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(creds, jc.DeepEquals, map[string]cloud.Credential{
		"foo":    cloud.NewNamedCredential("foo", cloud.UserPassAuthType, map[string]string{"hello": "world"}, false),
		"foobar": cloud.NewNamedCredential("foobar", cloud.UserPassAuthType, map[string]string{"hello": "world"}, false),
	})
}

func (s *serviceSuite) TestCloudCredential(c *gc.C) {
	defer s.setupMocks(c).Finish()

	key := corecredential.Key{Cloud: "cirrus", Owner: usertesting.GenNewName(c, "fred"), Name: "foo"}
	cred := credential.CloudCredentialResult{
		CloudCredentialInfo: credential.CloudCredentialInfo{
			AuthType: string(cloud.UserPassAuthType),
			Attributes: map[string]string{
				"hello": "world",
			},
			Label: "foo",
		},
	}
	s.state.EXPECT().CloudCredential(gomock.Any(), key).Return(cred, nil)

	result, err := s.service(c).CloudCredential(context.Background(), key)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, cloud.NewNamedCredential("foo", cloud.UserPassAuthType, map[string]string{"hello": "world"}, false))
}

func (s *serviceSuite) TestCloudCredentialInvalidID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	key := corecredential.Key{Cloud: "cirrus", Owner: usertesting.GenNewName(c, "fred")}
	_, err := s.service(c).CloudCredential(context.Background(), key)
	c.Assert(err, gc.ErrorMatches, "invalid id getting cloud credential.*")
}

func (s *serviceSuite) TestRemoveCloudCredential(c *gc.C) {
	defer s.setupMocks(c).Finish()

	key := corecredential.Key{Cloud: "cirrus", Owner: usertesting.GenNewName(c, "fred"), Name: "foo"}
	s.state.EXPECT().RemoveCloudCredential(gomock.Any(), key)

	err := s.service(c).RemoveCloudCredential(context.Background(), key)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestRemoveCloudCredentialInvalidID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	key := corecredential.Key{Cloud: "cirrus", Owner: usertesting.GenNewName(c, "fred")}
	err := s.service(c).RemoveCloudCredential(context.Background(), key)
	c.Assert(err, gc.ErrorMatches, "invalid id removing cloud credential.*")
}

func (s *serviceSuite) TestInvalidateCloudCredential(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uuid := credentialtesting.GenCredentialUUID(c)
	key := corecredential.Key{Cloud: "cirrus", Owner: usertesting.GenNewName(c, "fred"), Name: "foo"}
	s.state.EXPECT().CredentialUUIDForKey(gomock.Any(), key).Return(uuid, nil)
	s.state.EXPECT().InvalidateCloudCredential(gomock.Any(), uuid, "gone bad")

	err := s.service(c).InvalidateCredential(context.Background(), key, "gone bad")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestInvalidateCloudCredentialInvalidID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	key := corecredential.Key{Cloud: "cirrus", Owner: usertesting.GenNewName(c, "fred")}
	err := s.service(c).InvalidateCredential(context.Background(), key, "nope")
	c.Assert(err, gc.ErrorMatches, "invalidating cloud credential with invalid key.*")
}

func (s *serviceSuite) TestAllCloudCredentials(c *gc.C) {
	defer s.setupMocks(c).Finish()

	credId := corecredential.Key{Cloud: "cirrus", Owner: usertesting.GenNewName(c, "fred"), Name: "foo"}
	credInfoResult := credential.CloudCredentialResult{
		CloudCredentialInfo: credential.CloudCredentialInfo{
			Label:      "foo",
			AuthType:   string(cloud.UserPassAuthType),
			Attributes: map[string]string{"hello": "world"},
		},
		CloudName: "cirrus",
	}
	s.state.EXPECT().AllCloudCredentialsForOwner(gomock.Any(), usertesting.GenNewName(c, "fred")).Return(
		map[corecredential.Key]credential.CloudCredentialResult{credId: credInfoResult}, nil)

	result, err := s.service(c).AllCloudCredentialsForOwner(context.Background(), usertesting.GenNewName(c, "fred"))
	c.Assert(err, jc.ErrorIsNil)
	cred := cloud.NewNamedCredential("foo", cloud.UserPassAuthType, map[string]string{"hello": "world"}, false)
	c.Assert(result, jc.DeepEquals, map[corecredential.Key]cloud.Credential{credId: cred})
}

func (s *serviceSuite) TestWatchCredential(c *gc.C) {
	defer s.setupMocks(c).Finish()

	nw := watchertest.NewMockNotifyWatcher(nil)

	key := corecredential.Key{Cloud: "cirrus", Owner: usertesting.GenNewName(c, "fred"), Name: "foo"}
	s.state.EXPECT().WatchCredential(gomock.Any(), gomock.Any(), key).Return(nw, nil)

	w, err := s.service(c).WatchCredential(context.Background(), key)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
}

func (s *serviceSuite) TestWatchCredentialInvalidID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	key := corecredential.Key{Cloud: "cirrus", Owner: usertesting.GenNewName(c, "fred")}
	_, err := s.service(c).WatchCredential(context.Background(), key)
	c.Assert(err, gc.ErrorMatches, "watching cloud credential with invalid key.*")
}

func (s *serviceSuite) TestCheckAndUpdateCredentialsNoModelsFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cred := credential.CloudCredentialInfo{}
	key := corecredential.Key{
		Cloud: "cirrus",
		Owner: usertesting.GenNewName(c, "bob"),
		Name:  "foobar",
	}

	s.state.EXPECT().ModelsUsingCloudCredential(gomock.Any(), key).Return(nil, coreerrors.NotFound)

	s.state.EXPECT().UpsertCloudCredential(gomock.Any(), key, cred)

	service := s.service(c)

	results, err := service.CheckAndUpdateCredential(context.Background(), key, cloud.Credential{}, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 0)
}

func (s *serviceSuite) TestCheckAndUpdateCredentialInvalidKey(c *gc.C) {
	defer s.setupMocks(c).Finish()

	key := corecredential.Key{Cloud: "cirrus", Owner: usertesting.GenNewName(c, "fred")}
	_, err := s.service(c).CheckAndUpdateCredential(context.Background(), key, cloud.Credential{}, false)
	c.Assert(err, gc.ErrorMatches, "invalid id updating cloud credential.*")
}

func (s *serviceSuite) TestCheckAndUpdateCredentialModelsError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cred := cloud.Credential{}
	key := corecredential.Key{
		Cloud: "cirrus",
		Owner: usertesting.GenNewName(c, "bob"),
		Name:  "foobar",
	}

	s.state.EXPECT().ModelsUsingCloudCredential(gomock.Any(), key).Return(nil, errors.New("cannot get models"))

	service := s.service(c)

	results, err := service.CheckAndUpdateCredential(context.Background(), key, cred, false)
	c.Assert(err, gc.ErrorMatches, "cannot get models")
	c.Assert(results, gc.HasLen, 0)
}

func (s *serviceSuite) TestCheckAndUpdateCredential(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cred := cloud.Credential{}
	key := corecredential.Key{
		Cloud: "cirrus",
		Owner: usertesting.GenNewName(c, "bob"),
		Name:  "foobar",
	}

	s.state.EXPECT().ModelsUsingCloudCredential(gomock.Any(), key).Return(map[coremodel.UUID]string{
		coremodel.UUID(jujutesting.ModelTag.Id()): "mymodel",
	}, nil)

	s.state.EXPECT().UpsertCloudCredential(gomock.Any(), key, credential.CloudCredentialInfo{})

	service := s.service(c)

	results, err := service.CheckAndUpdateCredential(context.Background(), key, cred, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, []UpdateCredentialModelResult{{
		ModelUUID: coremodel.UUID(jujutesting.ModelTag.Id()), ModelName: "mymodel",
	}})
}

func (s *serviceSuite) TestRevokeCredentialsModelsError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	key := corecredential.Key{
		Cloud: "cirrus",
		Owner: usertesting.GenNewName(c, "bob"),
		Name:  "foobar",
	}

	s.state.EXPECT().ModelsUsingCloudCredential(gomock.Any(), key).Return(nil, errors.New("cannot get models"))

	service := s.service(c)

	err := service.CheckAndRevokeCredential(context.Background(), key, false)
	c.Assert(err, gc.ErrorMatches, "cannot get models")
}

func (s *serviceSuite) TestRevokeCredentialsHasModel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	key := corecredential.Key{
		Cloud: "cirrus",
		Owner: usertesting.GenNewName(c, "bob"),
		Name:  "foobar",
	}

	s.state.EXPECT().ModelsUsingCloudCredential(gomock.Any(), key).Return(map[coremodel.UUID]string{
		coremodel.UUID(jujutesting.ModelTag.Id()): "mymodel",
	}, nil)

	service := s.service(c)

	err := service.CheckAndRevokeCredential(context.Background(), key, false)
	c.Assert(err, gc.ErrorMatches, `cannot revoke credential cirrus/bob/foobar: it is still used by 1 model`)
}

func (s *serviceSuite) TestRevokeCredentialsHasModels(c *gc.C) {
	defer s.setupMocks(c).Finish()

	key := corecredential.Key{
		Cloud: "cirrus",
		Owner: usertesting.GenNewName(c, "bob"),
		Name:  "foobar",
	}

	s.state.EXPECT().ModelsUsingCloudCredential(gomock.Any(), key).Return(map[coremodel.UUID]string{
		coremodel.UUID(jujutesting.ModelTag.Id()): "mymodel",
		"deadbeef-1bad-500d-9000-4b1d0d06f666":    "anothermodel",
	}, nil)

	service := s.service(c)

	err := service.CheckAndRevokeCredential(context.Background(), key, false)
	c.Assert(err, gc.ErrorMatches, `cannot revoke credential cirrus/bob/foobar: it is still used by 2 models`)
}

func (s *serviceSuite) TestRevokeCredentialsHasModelForce(c *gc.C) {
	defer s.setupMocks(c).Finish()

	key := corecredential.Key{
		Cloud: "cirrus",
		Owner: usertesting.GenNewName(c, "bob"),
		Name:  "foobar",
	}

	s.state.EXPECT().ModelsUsingCloudCredential(gomock.Any(), key).Return(map[coremodel.UUID]string{
		coremodel.UUID(jujutesting.ModelTag.Id()): "mymodel",
	}, nil)
	s.state.EXPECT().RemoveCloudCredential(gomock.Any(), key)

	service := s.service(c)

	err := service.CheckAndRevokeCredential(context.Background(), key, true)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestRevokeCredentialsHasModelsForce(c *gc.C) {
	defer s.setupMocks(c).Finish()

	key := corecredential.Key{
		Cloud: "cirrus",
		Owner: usertesting.GenNewName(c, "bob"),
		Name:  "foobar",
	}

	s.state.EXPECT().ModelsUsingCloudCredential(gomock.Any(), key).Return(map[coremodel.UUID]string{
		coremodel.UUID(jujutesting.ModelTag.Id()): "mymodel",
		"deadbeef-1bad-500d-9000-4b1d0d06f666":    "anothermodel",
	}, nil)
	s.state.EXPECT().RemoveCloudCredential(gomock.Any(), key)

	service := s.service(c)

	err := service.CheckAndRevokeCredential(context.Background(), key, true)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestCheckAndRevokeCredentialInvalidID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	key := corecredential.Key{Cloud: "cirrus", Owner: usertesting.GenNewName(c, "fred")}
	err := s.service(c).CheckAndRevokeCredential(context.Background(), key, false)
	c.Assert(err, gc.ErrorMatches, "invalid id revoking cloud credential.*")
}

// TestInvalidateModelCloudCredentialNotFound is asserting that if we try and
// invalidate the cloud credential for a model that doesn't exist we get back
// an error satisfying [modelerrors.NotValid].
func (s *serviceSuite) TestInvalidateModelCloudCredentialNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()
	modelUUID := modeltesting.GenModelUUID(c)

	s.state.EXPECT().InvalidateModelCloudCredential(
		gomock.Any(),
		modelUUID,
		gomock.Any(),
	).Return(modelerrors.NotFound)
	err := s.service(c).InvalidateModelCredential(
		context.Background(),
		modelUUID,
		"some reason",
	)
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)
}

// TestInvalidateModelCloudCredentialNotSet is asserting that if we try to
// invalidate the cloud credential for a model and the model has no cloud
// credential set then we get back an error satisfying
// [credentialerrors.ModelCredentialNotSet].
func (s *serviceSuite) TestInvalidateModelCloudCredentialNotSet(c *gc.C) {
	defer s.setupMocks(c).Finish()
	modelUUID := modeltesting.GenModelUUID(c)

	s.state.EXPECT().InvalidateModelCloudCredential(
		gomock.Any(),
		modelUUID,
		gomock.Any(),
	).Return(credentialerrors.ModelCredentialNotSet)
	err := s.service(c).InvalidateModelCredential(
		context.Background(),
		modelUUID,
		"some reason",
	)
	c.Check(err, jc.ErrorIs, credentialerrors.ModelCredentialNotSet)
}

// TestInvalidateModelCloudCredenntialInvalidModelUUID is asserting that if we
// try to invalidate the cloud credential associated with a model and the model
// model uuid provided is invalid no operation is performed and the error we get
// back satisfies [coreerrors.NotValid].
func (s *serviceSuite) TestInvalidateModelCloudCredenntialInvalidModelUUID(c *gc.C) {
	defer s.setupMocks(c).Finish()
	modelUUID := coremodel.UUID("invalid")

	err := s.service(c).InvalidateModelCredential(
		context.Background(),
		modelUUID,
		"some reason",
	)
	c.Check(err, jc.ErrorIs, coreerrors.NotValid)
}

// TestInvalidateModelCloudCredential asserts the happy path of invalidating the
// cloud credential associated with a model.
func (s serviceSuite) TestInvalidateModelCloudCredential(c *gc.C) {
	defer s.setupMocks(c).Finish()
	modelUUID := modeltesting.GenModelUUID(c)

	s.state.EXPECT().InvalidateModelCloudCredential(
		gomock.Any(),
		modelUUID,
		"some reason",
	).Return(nil)

	err := s.service(c).InvalidateModelCredential(
		context.Background(),
		modelUUID,
		"some reason",
	)
	c.Check(err, jc.ErrorIsNil)
}

// TestModelCredentialStatus represents a test for the happy path of getting
// the credential key and validity status of a model's credential.
func (s *serviceSuite) TestModelCredentialStatus(c *gc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	credentialKey := corecredential.Key{
		Cloud: "cirrus",
		Owner: usertesting.GenNewName(c, "bob"),
		Name:  "foobar",
	}

	s.state.EXPECT().GetModelCredentialStatus(gomock.Any(), modelUUID).Return(
		credentialKey, true, nil,
	)
	key, valid, err := s.service(c).GetModelCredentialStatus(context.Background(), modelUUID)
	c.Check(err, jc.ErrorIsNil)
	c.Check(key, gc.Equals, credentialKey)
	c.Check(valid, jc.IsTrue)

	// Check the invalid case as well to be complete.
	s.state.EXPECT().GetModelCredentialStatus(gomock.Any(), modelUUID).Return(
		credentialKey, false, nil,
	)
	key, valid, err = s.service(c).GetModelCredentialStatus(context.Background(), modelUUID)
	c.Check(err, jc.ErrorIsNil)
	c.Check(key, gc.Equals, credentialKey)
	c.Check(valid, jc.IsFalse)
}

// TestModelCredentialStatusNotFound asserts that if we ask for the credential
// and status of a model that doesn't exist the error returned satisfies
// [modelerrors.NotFound].
func (s *serviceSuite) TestModelCredentialStatusNotFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := modeltesting.GenModelUUID(c)

	s.state.EXPECT().GetModelCredentialStatus(gomock.Any(), modelUUID).Return(
		corecredential.Key{}, false, modelerrors.NotFound,
	)
	_, _, err := s.service(c).GetModelCredentialStatus(context.Background(), modelUUID)
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)
}

// TestModelCredentialStatusNotSet asserts that we ask for the credential and
// status of a model's credential but no credential has been set on the model an
// error is returned that satisfies [credentialerrors.ModelCredentialNotSet].
func (s *serviceSuite) TestModelCredentialStatusNotSet(c *gc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := modeltesting.GenModelUUID(c)

	s.state.EXPECT().GetModelCredentialStatus(gomock.Any(), modelUUID).Return(
		corecredential.Key{}, false, credentialerrors.ModelCredentialNotSet,
	)
	_, _, err := s.service(c).GetModelCredentialStatus(context.Background(), modelUUID)
	c.Check(err, jc.ErrorIs, credentialerrors.ModelCredentialNotSet)
}
