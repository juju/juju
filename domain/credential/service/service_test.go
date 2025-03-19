// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	corecredential "github.com/juju/juju/core/credential"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain/credential"
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
	s.state.EXPECT().UpsertCloudCredential(gomock.Any(), key, cred).Return(nil, nil)

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
	s.state.EXPECT().RemoveCloudCredential(gomock.Any(), key).Return(nil)

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

	key := corecredential.Key{Cloud: "cirrus", Owner: usertesting.GenNewName(c, "fred"), Name: "foo"}
	s.state.EXPECT().InvalidateCloudCredential(gomock.Any(), key, "gone bad").Return(nil)

	err := s.service(c).InvalidateCredential(context.Background(), key, "gone bad")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestInvalidateCloudCredentialInvalidID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	key := corecredential.Key{Cloud: "cirrus", Owner: usertesting.GenNewName(c, "fred")}
	err := s.service(c).InvalidateCredential(context.Background(), key, "nope")
	c.Assert(err, gc.ErrorMatches, "invalid id invalidating cloud credential.*")
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
	c.Assert(err, gc.ErrorMatches, "invalid id watching cloud credential.*")
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

	var invalid = true
	s.state.EXPECT().UpsertCloudCredential(gomock.Any(), key, cred).Return(&invalid, nil)

	var legacyUpdated bool
	service := s.service(c).
		WithLegacyUpdater(func(tag names.CloudCredentialTag) error {
			c.Assert(tag, jc.DeepEquals, names.NewCloudCredentialTag("cirrus/bob/foobar"))
			legacyUpdated = true
			return nil
		})

	results, err := service.CheckAndUpdateCredential(context.Background(), key, cloud.Credential{}, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 0)
	c.Assert(legacyUpdated, jc.IsTrue)
}

func (s *serviceSuite) TestCheckAndUpdateCredentialInvalidKey(c *gc.C) {
	defer s.setupMocks(c).Finish()

	key := corecredential.Key{Cloud: "cirrus", Owner: usertesting.GenNewName(c, "fred")}
	_, err := s.service(c).CheckAndUpdateCredential(context.Background(), key, cloud.Credential{}, false)
	c.Assert(err, gc.ErrorMatches, "invalid id updating cloud credential.*")
}

func (s *serviceSuite) TestUpdateCredentialsModelsError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cred := cloud.Credential{}
	key := corecredential.Key{
		Cloud: "cirrus",
		Owner: usertesting.GenNewName(c, "bob"),
		Name:  "foobar",
	}

	s.state.EXPECT().ModelsUsingCloudCredential(gomock.Any(), key).Return(nil, errors.New("cannot get models"))

	var legacyUpdated bool
	service := s.service(c).
		WithLegacyUpdater(func(tag names.CloudCredentialTag) error {
			return coreerrors.NotImplemented
		})

	results, err := service.CheckAndUpdateCredential(context.Background(), key, cred, false)
	c.Assert(err, gc.ErrorMatches, "cannot get models")
	c.Assert(results, gc.HasLen, 0)
	c.Assert(legacyUpdated, jc.IsFalse)
}

func (s *serviceSuite) TestUpdateCredentialsModels(c *gc.C) {
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

	var invalid = true
	s.state.EXPECT().UpsertCloudCredential(gomock.Any(), key, credential.CloudCredentialInfo{}).Return(&invalid, nil)

	var legacyUpdated bool
	service := s.service(c).
		WithLegacyUpdater(func(tag names.CloudCredentialTag) error {
			c.Assert(tag, jc.DeepEquals, names.NewCloudCredentialTag("cirrus/bob/foobar"))
			legacyUpdated = true
			return nil
		})

	results, err := service.CheckAndUpdateCredential(context.Background(), key, cred, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, []UpdateCredentialModelResult{{
		ModelUUID: coremodel.UUID(jujutesting.ModelTag.Id()), ModelName: "mymodel",
	}})
	c.Assert(legacyUpdated, jc.IsTrue)
}

func (s *serviceSuite) TestUpdateCredentialsModelFailedValidationForce(c *gc.C) {
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

	var invalid = true
	s.state.EXPECT().UpsertCloudCredential(gomock.Any(), key, credential.CloudCredentialInfo{}).Return(&invalid, nil)

	var legacyUpdated bool
	service := s.service(c).
		WithLegacyUpdater(func(tag names.CloudCredentialTag) error {
			c.Assert(tag, jc.DeepEquals, names.NewCloudCredentialTag("cirrus/bob/foobar"))
			legacyUpdated = true
			return nil
		})

	results, err := service.CheckAndUpdateCredential(context.Background(), key, cred, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, []UpdateCredentialModelResult{{
		ModelUUID: coremodel.UUID(jujutesting.ModelTag.Id()), ModelName: "mymodel",
	}})
	c.Assert(legacyUpdated, jc.IsTrue)
}

func (s *serviceSuite) TestRevokeCredentialsModelsError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	key := corecredential.Key{
		Cloud: "cirrus",
		Owner: usertesting.GenNewName(c, "bob"),
		Name:  "foobar",
	}

	s.state.EXPECT().ModelsUsingCloudCredential(gomock.Any(), key).Return(nil, errors.New("cannot get models"))

	var legacyUpdated bool
	service := s.service(c).
		WithLegacyRemover(func(tag names.CloudCredentialTag) error {
			return coreerrors.NotImplemented
		})

	err := service.CheckAndRevokeCredential(context.Background(), key, false)
	c.Assert(err, gc.ErrorMatches, "cannot get models")
	c.Assert(legacyUpdated, jc.IsFalse)
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

	var legacyUpdated bool
	service := s.service(c).
		WithLegacyRemover(func(tag names.CloudCredentialTag) error {
			c.Assert(tag, jc.DeepEquals, names.NewCloudCredentialTag("cirrus/bob/foobar"))
			legacyUpdated = true
			return nil
		})

	err := service.CheckAndRevokeCredential(context.Background(), key, false)
	c.Assert(err, gc.ErrorMatches, `cannot revoke credential cirrus/bob/foobar: it is still used by 1 model`)
	c.Assert(legacyUpdated, jc.IsFalse)
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

	var legacyUpdated bool
	service := s.service(c).
		WithLegacyRemover(func(tag names.CloudCredentialTag) error {
			c.Assert(tag, jc.DeepEquals, names.NewCloudCredentialTag("cirrus/bob/foobar"))
			legacyUpdated = true
			return nil
		})

	err := service.CheckAndRevokeCredential(context.Background(), key, false)
	c.Assert(err, gc.ErrorMatches, `cannot revoke credential cirrus/bob/foobar: it is still used by 2 models`)
	c.Assert(legacyUpdated, jc.IsFalse)
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
	s.state.EXPECT().RemoveCloudCredential(gomock.Any(), key).Return(nil)

	var legacyUpdated bool
	service := s.service(c).
		WithLegacyRemover(func(tag names.CloudCredentialTag) error {
			c.Assert(tag, jc.DeepEquals, names.NewCloudCredentialTag("cirrus/bob/foobar"))
			legacyUpdated = true
			return nil
		})

	err := service.CheckAndRevokeCredential(context.Background(), key, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(legacyUpdated, jc.IsTrue)
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
	s.state.EXPECT().RemoveCloudCredential(gomock.Any(), key).Return(nil)

	var legacyUpdated bool
	service := s.service(c).
		WithLegacyRemover(func(tag names.CloudCredentialTag) error {
			c.Assert(tag, jc.DeepEquals, names.NewCloudCredentialTag("cirrus/bob/foobar"))
			legacyUpdated = true
			return nil
		})

	err := service.CheckAndRevokeCredential(context.Background(), key, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(legacyUpdated, jc.IsTrue)
}

func (s *serviceSuite) TestCheckAndRevokeCredentialInvalidID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	key := corecredential.Key{Cloud: "cirrus", Owner: usertesting.GenNewName(c, "fred")}
	err := s.service(c).CheckAndRevokeCredential(context.Background(), key, false)
	c.Assert(err, gc.ErrorMatches, "invalid id revoking cloud credential.*")
}
