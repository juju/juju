// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	corecredential "github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain/credential"
	jujutesting "github.com/juju/juju/testing"
)

type baseSuite struct {
	testing.IsolationSuite

	state          *MockState
	validator      *MockCredentialValidator
	watcherFactory *MockWatcherFactory
}

func (s *baseSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.validator = NewMockCredentialValidator(ctrl)
	s.state = NewMockState(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)

	return ctrl
}

type serviceSuite struct {
	baseSuite
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) service() *WatchableService {
	return NewWatchableService(s.state, s.watcherFactory, loggo.GetLogger("test"))
}

func (s *serviceSuite) TestUpdateCloudCredential(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := corecredential.ID{Cloud: "cirrus", Owner: "fred", Name: "foo"}
	cred := credential.CloudCredentialInfo{
		AuthType: string(cloud.UserPassAuthType),
		Attributes: map[string]string{
			"hello": "world",
		},
		Label: "foo",
	}
	s.state.EXPECT().UpsertCloudCredential(gomock.Any(), id, cred).Return(nil, nil)

	err := s.service().UpdateCloudCredential(
		context.Background(), id,
		cloud.NewNamedCredential("foo", cloud.UserPassAuthType, map[string]string{"hello": "world"}, false))
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateCloudCredentialInvalidID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := corecredential.ID{Cloud: "cirrus", Owner: "fred"}
	err := s.service().UpdateCloudCredential(context.Background(), id, cloud.Credential{})
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
	s.state.EXPECT().CloudCredentialsForOwner(gomock.Any(), "fred", "cirrus").Return(map[string]credential.CloudCredentialResult{
		"foo":    one,
		"foobar": two,
	}, nil)

	creds, err := s.service().CloudCredentialsForOwner(context.Background(), "fred", "cirrus")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(creds, jc.DeepEquals, map[string]cloud.Credential{
		"foo":    cloud.NewNamedCredential("foo", cloud.UserPassAuthType, map[string]string{"hello": "world"}, false),
		"foobar": cloud.NewNamedCredential("foobar", cloud.UserPassAuthType, map[string]string{"hello": "world"}, false),
	})
}

func (s *serviceSuite) TestCloudCredential(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := corecredential.ID{Cloud: "cirrus", Owner: "fred", Name: "foo"}
	cred := credential.CloudCredentialResult{
		CloudCredentialInfo: credential.CloudCredentialInfo{
			AuthType: string(cloud.UserPassAuthType),
			Attributes: map[string]string{
				"hello": "world",
			},
			Label: "foo",
		},
	}
	s.state.EXPECT().CloudCredential(gomock.Any(), id).Return(cred, nil)

	result, err := s.service().CloudCredential(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, cloud.NewNamedCredential("foo", cloud.UserPassAuthType, map[string]string{"hello": "world"}, false))
}

func (s *serviceSuite) TestCloudCredentialInvalidID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := corecredential.ID{Cloud: "cirrus", Owner: "fred"}
	_, err := s.service().CloudCredential(context.Background(), id)
	c.Assert(err, gc.ErrorMatches, "invalid id getting cloud credential.*")
}

func (s *serviceSuite) TestRemoveCloudCredential(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := corecredential.ID{Cloud: "cirrus", Owner: "fred", Name: "foo"}
	s.state.EXPECT().RemoveCloudCredential(gomock.Any(), id).Return(nil)

	err := s.service().RemoveCloudCredential(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestRemoveCloudCredentialInvalidID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := corecredential.ID{Cloud: "cirrus", Owner: "fred"}
	err := s.service().RemoveCloudCredential(context.Background(), id)
	c.Assert(err, gc.ErrorMatches, "invalid id removing cloud credential.*")
}

func (s *serviceSuite) TestInvalidateCloudCredential(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := corecredential.ID{Cloud: "cirrus", Owner: "fred", Name: "foo"}
	s.state.EXPECT().InvalidateCloudCredential(gomock.Any(), id, "gone bad").Return(nil)

	err := s.service().InvalidateCredential(context.Background(), id, "gone bad")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestInvalidateCloudCredentialInvalidID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := corecredential.ID{Cloud: "cirrus", Owner: "fred"}
	err := s.service().InvalidateCredential(context.Background(), id, "nope")
	c.Assert(err, gc.ErrorMatches, "invalid id invalidating cloud credential.*")
}

func (s *serviceSuite) TestAllCloudCredentials(c *gc.C) {
	defer s.setupMocks(c).Finish()

	credId := corecredential.ID{Cloud: "cirrus", Owner: "fred", Name: "foo"}
	credInfoResult := credential.CloudCredentialResult{
		CloudCredentialInfo: credential.CloudCredentialInfo{
			Label:      "foo",
			AuthType:   string(cloud.UserPassAuthType),
			Attributes: map[string]string{"hello": "world"},
		},
		CloudName: "cirrus",
	}
	s.state.EXPECT().AllCloudCredentialsForOwner(gomock.Any(), "fred").Return(
		map[corecredential.ID]credential.CloudCredentialResult{credId: credInfoResult}, nil)

	result, err := s.service().AllCloudCredentialsForOwner(context.Background(), "fred")
	c.Assert(err, jc.ErrorIsNil)
	cred := cloud.NewNamedCredential("foo", cloud.UserPassAuthType, map[string]string{"hello": "world"}, false)
	c.Assert(result, jc.DeepEquals, map[corecredential.ID]cloud.Credential{credId: cred})
}

func (s *serviceSuite) TestWatchCredential(c *gc.C) {
	defer s.setupMocks(c).Finish()

	nw := watchertest.NewMockNotifyWatcher(nil)

	id := corecredential.ID{Cloud: "cirrus", Owner: "fred", Name: "foo"}
	s.state.EXPECT().WatchCredential(gomock.Any(), gomock.Any(), id).Return(nw, nil)

	w, err := s.service().WatchCredential(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
}

func (s *serviceSuite) TestWatchCredentialInvalidID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := corecredential.ID{Cloud: "cirrus", Owner: "fred"}
	_, err := s.service().WatchCredential(context.Background(), id)
	c.Assert(err, gc.ErrorMatches, "invalid id watching cloud credential.*")
}

func (s *serviceSuite) TestCheckAndUpdateCredentialsNoModelsFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cred := credential.CloudCredentialInfo{}
	id := corecredential.ID{
		Cloud: "cirrus",
		Owner: "bob",
		Name:  "foobar",
	}

	s.state.EXPECT().ModelsUsingCloudCredential(gomock.Any(), id).Return(nil, errors.NotFound)

	var invalid = true
	s.state.EXPECT().UpsertCloudCredential(gomock.Any(), id, cred).Return(&invalid, nil)

	var legacyUpdated bool
	service := s.service().
		WithValidationContextGetter(func(_ context.Context, modelUUID coremodel.UUID) (CredentialValidationContext, error) {
			return CredentialValidationContext{}, errors.NotImplemented
		}).
		WithLegacyUpdater(func(tag names.CloudCredentialTag) error {
			c.Assert(tag, jc.DeepEquals, names.NewCloudCredentialTag("cirrus/bob/foobar"))
			legacyUpdated = true
			return nil
		})

	results, err := service.CheckAndUpdateCredential(context.Background(), id, cloud.Credential{}, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 0)
	c.Assert(legacyUpdated, jc.IsTrue)
}

func (s *serviceSuite) TestCheckAndUpdateCredentialInvalidID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := corecredential.ID{Cloud: "cirrus", Owner: "fred"}
	_, err := s.service().CheckAndUpdateCredential(context.Background(), id, cloud.Credential{}, false)
	c.Assert(err, gc.ErrorMatches, "invalid id updating cloud credential.*")
}

func (s *serviceSuite) TestUpdateCredentialsModelsError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cred := cloud.Credential{}
	id := corecredential.ID{
		Cloud: "cirrus",
		Owner: "bob",
		Name:  "foobar",
	}

	s.state.EXPECT().ModelsUsingCloudCredential(gomock.Any(), id).Return(nil, errors.New("cannot get models"))

	var legacyUpdated bool
	service := s.service().
		WithValidationContextGetter(func(_ context.Context, modelUUID coremodel.UUID) (CredentialValidationContext, error) {
			return CredentialValidationContext{}, errors.NotImplemented
		}).
		WithLegacyUpdater(func(tag names.CloudCredentialTag) error {
			return errors.NotImplemented
		})

	results, err := service.CheckAndUpdateCredential(context.Background(), id, cred, false)
	c.Assert(err, gc.ErrorMatches, "cannot get models")
	c.Assert(results, gc.HasLen, 0)
	c.Assert(legacyUpdated, jc.IsFalse)
}

func (s *serviceSuite) TestUpdateCredentialsModelsFailedContext(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cred := cloud.Credential{}
	id := corecredential.ID{
		Cloud: "cirrus",
		Owner: "bob",
		Name:  "foobar",
	}

	s.state.EXPECT().ModelsUsingCloudCredential(gomock.Any(), id).Return(map[coremodel.UUID]string{
		coremodel.UUID(jujutesting.ModelTag.Id()): "mymodel",
	}, nil)

	contextError := errors.New("failed context")

	var legacyUpdated bool
	service := s.service().
		WithValidationContextGetter(func(_ context.Context, modelUUID coremodel.UUID) (CredentialValidationContext, error) {
			return CredentialValidationContext{}, contextError
		}).
		WithLegacyUpdater(func(tag names.CloudCredentialTag) error {
			c.Assert(tag, jc.DeepEquals, names.NewCloudCredentialTag("cirrus/bob/foobar"))
			legacyUpdated = true
			return nil
		})

	results, err := service.CheckAndUpdateCredential(context.Background(), id, cred, false)
	c.Assert(err, gc.ErrorMatches, "credential is not valid for one or more models")
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Errors, gc.HasLen, 1)
	c.Assert(results[0].Errors[0], gc.ErrorMatches, "failed context")
	results[0].Errors = nil
	c.Assert(results, jc.DeepEquals, []UpdateCredentialModelResult{{
		ModelUUID: coremodel.UUID(jujutesting.ModelTag.Id()), ModelName: "mymodel",
	}})
	c.Assert(legacyUpdated, jc.IsFalse)
}

func (s *serviceSuite) TestUpdateCredentialsModelsFailedContextForce(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := corecredential.ID{
		Cloud: "cirrus",
		Owner: "bob",
		Name:  "foobar",
	}

	s.state.EXPECT().ModelsUsingCloudCredential(gomock.Any(), id).Return(map[coremodel.UUID]string{
		coremodel.UUID(jujutesting.ModelTag.Id()): "mymodel",
	}, nil)

	var invalid = true
	s.state.EXPECT().UpsertCloudCredential(gomock.Any(), id, credential.CloudCredentialInfo{}).Return(&invalid, nil)

	contextError := errors.New("failed context")

	var legacyUpdated bool
	service := s.service().
		WithValidationContextGetter(func(_ context.Context, modelUUID coremodel.UUID) (CredentialValidationContext, error) {
			return CredentialValidationContext{}, contextError
		}).
		WithLegacyUpdater(func(tag names.CloudCredentialTag) error {
			c.Assert(tag, jc.DeepEquals, names.NewCloudCredentialTag("cirrus/bob/foobar"))
			legacyUpdated = true
			return nil
		})

	results, err := service.CheckAndUpdateCredential(context.Background(), id, cloud.Credential{}, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Errors, gc.HasLen, 1)
	c.Assert(results[0].Errors[0], gc.ErrorMatches, "failed context")
	results[0].Errors = nil
	c.Assert(results, jc.DeepEquals, []UpdateCredentialModelResult{{
		ModelUUID: coremodel.UUID(jujutesting.ModelTag.Id()), ModelName: "mymodel",
	}})
	c.Assert(legacyUpdated, jc.IsTrue)
}

func (s *serviceSuite) TestUpdateCredentialsModels(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cred := cloud.Credential{}
	id := corecredential.ID{
		Cloud: "cirrus",
		Owner: "bob",
		Name:  "foobar",
	}

	s.state.EXPECT().ModelsUsingCloudCredential(gomock.Any(), id).Return(map[coremodel.UUID]string{
		coremodel.UUID(jujutesting.ModelTag.Id()): "mymodel",
	}, nil)

	var invalid = true
	s.state.EXPECT().UpsertCloudCredential(gomock.Any(), id, credential.CloudCredentialInfo{}).Return(&invalid, nil)
	s.validator.EXPECT().Validate(gomock.Any(), gomock.Any(), id, &cred, false).Return(nil, nil)

	var legacyUpdated bool
	service := s.service().
		WithValidationContextGetter(func(_ context.Context, modelUUID coremodel.UUID) (CredentialValidationContext, error) {
			return CredentialValidationContext{}, nil
		}).
		WithCredentialValidator(s.validator).
		WithLegacyUpdater(func(tag names.CloudCredentialTag) error {
			c.Assert(tag, jc.DeepEquals, names.NewCloudCredentialTag("cirrus/bob/foobar"))
			legacyUpdated = true
			return nil
		})

	results, err := service.CheckAndUpdateCredential(context.Background(), id, cred, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, []UpdateCredentialModelResult{{
		ModelUUID: coremodel.UUID(jujutesting.ModelTag.Id()), ModelName: "mymodel",
	}})
	c.Assert(legacyUpdated, jc.IsTrue)
}

func (s *serviceSuite) TestUpdateCredentialsModelFailedValidationForce(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cred := cloud.Credential{}
	id := corecredential.ID{
		Cloud: "cirrus",
		Owner: "bob",
		Name:  "foobar",
	}

	s.state.EXPECT().ModelsUsingCloudCredential(gomock.Any(), id).Return(map[coremodel.UUID]string{
		coremodel.UUID(jujutesting.ModelTag.Id()): "mymodel",
	}, nil)

	var invalid = true
	s.state.EXPECT().UpsertCloudCredential(gomock.Any(), id, credential.CloudCredentialInfo{}).Return(&invalid, nil)

	validationError := errors.New("cred error")
	s.validator.EXPECT().Validate(gomock.Any(), gomock.Any(), id, &cred, false).Return([]error{validationError}, nil)

	var legacyUpdated bool
	service := s.service().
		WithValidationContextGetter(func(_ context.Context, modelUUID coremodel.UUID) (CredentialValidationContext, error) {
			return CredentialValidationContext{}, nil
		}).
		WithCredentialValidator(s.validator).
		WithLegacyUpdater(func(tag names.CloudCredentialTag) error {
			c.Assert(tag, jc.DeepEquals, names.NewCloudCredentialTag("cirrus/bob/foobar"))
			legacyUpdated = true
			return nil
		})

	results, err := service.CheckAndUpdateCredential(context.Background(), id, cred, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, jc.DeepEquals, []UpdateCredentialModelResult{{
		ModelUUID: coremodel.UUID(jujutesting.ModelTag.Id()), ModelName: "mymodel", Errors: []error{validationError},
	}})
	c.Assert(legacyUpdated, jc.IsTrue)
}

func (s *serviceSuite) TestUpdateCredentialsSomeModelsFailedValidation(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cred := cloud.Credential{}
	id := corecredential.ID{
		Cloud: "cirrus",
		Owner: "bob",
		Name:  "foobar",
	}

	s.state.EXPECT().ModelsUsingCloudCredential(gomock.Any(), id).Return(map[coremodel.UUID]string{
		coremodel.UUID(jujutesting.ModelTag.Id()): "mymodel",
		"deadbeef-1bad-500d-9000-4b1d0d06f666":    "anothermodel",
	}, nil)

	validationError := errors.New("cred error")
	calls := 0
	s.validator.EXPECT().Validate(gomock.Any(), gomock.Any(), id, &cred, false).DoAndReturn(
		func(
			stdCtx context.Context,
			ctx CredentialValidationContext,
			credentialID corecredential.ID,
			credential *cloud.Credential,
			checkCloudInstances bool,
		) ([]error, error) {
			calls++
			if calls == 1 {
				return []error{validationError}, nil
			}
			return nil, nil
		}).Times(2)

	var legacyUpdated bool
	service := s.service().
		WithValidationContextGetter(func(_ context.Context, modelUUID coremodel.UUID) (CredentialValidationContext, error) {
			return CredentialValidationContext{}, nil
		}).
		WithCredentialValidator(s.validator).
		WithLegacyUpdater(func(tag names.CloudCredentialTag) error {
			c.Assert(tag, jc.DeepEquals, names.NewCloudCredentialTag("cirrus/bob/foobar"))
			legacyUpdated = true
			return nil
		})

	results, err := service.CheckAndUpdateCredential(context.Background(), id, cred, false)
	c.Assert(err, gc.ErrorMatches, "credential is not valid for one or more models")
	c.Assert(results, gc.HasLen, 2)
	gotErrors := 0
	for i := 0; i < 2; i++ {
		if len(results[i].Errors) == 0 {
			continue
		}
		gotErrors++
		c.Assert(results[i].Errors, gc.HasLen, 1)
		c.Assert(results[i].Errors[0], gc.ErrorMatches, "cred error")
		results[i].Errors = nil
	}
	c.Assert(gotErrors, gc.Equals, 1)
	c.Assert(results, jc.DeepEquals, []UpdateCredentialModelResult{{
		ModelUUID: coremodel.UUID(jujutesting.ModelTag.Id()), ModelName: "mymodel",
	}, {
		ModelUUID: "deadbeef-1bad-500d-9000-4b1d0d06f666", ModelName: "anothermodel",
	}})
	c.Assert(legacyUpdated, jc.IsFalse)
}

func (s *serviceSuite) TestUpdateCredentialsSomeModelsFailedValidationForce(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cred := cloud.Credential{}
	id := corecredential.ID{
		Cloud: "cirrus",
		Owner: "bob",
		Name:  "foobar",
	}

	s.state.EXPECT().ModelsUsingCloudCredential(gomock.Any(), id).Return(map[coremodel.UUID]string{
		coremodel.UUID(jujutesting.ModelTag.Id()): "mymodel",
		"deadbeef-1bad-500d-9000-4b1d0d06f666":    "anothermodel",
	}, nil)

	var invalid = true
	s.state.EXPECT().UpsertCloudCredential(gomock.Any(), id, credential.CloudCredentialInfo{}).Return(&invalid, nil)

	validationError := errors.New("cred error")
	calls := 0
	s.validator.EXPECT().Validate(gomock.Any(), gomock.Any(), id, &cred, false).DoAndReturn(
		func(
			stdCtx context.Context,
			ctx CredentialValidationContext,
			credentialID corecredential.ID,
			credential *cloud.Credential,
			checkCloudInstances bool,
		) ([]error, error) {
			calls++
			if calls == 1 {
				return []error{validationError}, nil
			}
			return nil, nil
		}).Times(2)

	var legacyUpdated bool
	service := s.service().
		WithValidationContextGetter(func(_ context.Context, modelUUID coremodel.UUID) (CredentialValidationContext, error) {
			return CredentialValidationContext{}, nil
		}).
		WithCredentialValidator(s.validator).
		WithLegacyUpdater(func(tag names.CloudCredentialTag) error {
			c.Assert(tag, jc.DeepEquals, names.NewCloudCredentialTag("cirrus/bob/foobar"))
			legacyUpdated = true
			return nil
		})

	results, err := service.CheckAndUpdateCredential(context.Background(), id, cred, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 2)
	gotErrors := 0
	for i := 0; i < 2; i++ {
		if len(results[i].Errors) == 0 {
			continue
		}
		gotErrors++
		c.Assert(results[i].Errors, gc.HasLen, 1)
		c.Assert(results[i].Errors[0], gc.ErrorMatches, "cred error")
		results[i].Errors = nil
	}
	c.Assert(gotErrors, gc.Equals, 1)
	c.Assert(results, jc.DeepEquals, []UpdateCredentialModelResult{{
		ModelUUID: coremodel.UUID(jujutesting.ModelTag.Id()), ModelName: "mymodel",
	}, {
		ModelUUID: "deadbeef-1bad-500d-9000-4b1d0d06f666", ModelName: "anothermodel",
	}})
	c.Assert(legacyUpdated, jc.IsTrue)
}

func (s *serviceSuite) TestRevokeCredentialsModelsError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := corecredential.ID{
		Cloud: "cirrus",
		Owner: "bob",
		Name:  "foobar",
	}

	s.state.EXPECT().ModelsUsingCloudCredential(gomock.Any(), id).Return(nil, errors.New("cannot get models"))

	var legacyUpdated bool
	service := s.service().
		WithLegacyRemover(func(tag names.CloudCredentialTag) error {
			return errors.NotImplemented
		})

	err := service.CheckAndRevokeCredential(context.Background(), id, false)
	c.Assert(err, gc.ErrorMatches, "cannot get models")
	c.Assert(legacyUpdated, jc.IsFalse)
}

func (s *serviceSuite) TestRevokeCredentialsHasModel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := corecredential.ID{
		Cloud: "cirrus",
		Owner: "bob",
		Name:  "foobar",
	}

	s.state.EXPECT().ModelsUsingCloudCredential(gomock.Any(), id).Return(map[coremodel.UUID]string{
		coremodel.UUID(jujutesting.ModelTag.Id()): "mymodel",
	}, nil)

	var legacyUpdated bool
	service := s.service().
		WithLegacyRemover(func(tag names.CloudCredentialTag) error {
			c.Assert(tag, jc.DeepEquals, names.NewCloudCredentialTag("cirrus/bob/foobar"))
			legacyUpdated = true
			return nil
		})

	err := service.CheckAndRevokeCredential(context.Background(), id, false)
	c.Assert(err, gc.ErrorMatches, `cannot revoke credential cirrus/bob/foobar: it is still used by 1 model`)
	c.Assert(legacyUpdated, jc.IsFalse)
}

func (s *serviceSuite) TestRevokeCredentialsHasModels(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := corecredential.ID{
		Cloud: "cirrus",
		Owner: "bob",
		Name:  "foobar",
	}

	s.state.EXPECT().ModelsUsingCloudCredential(gomock.Any(), id).Return(map[coremodel.UUID]string{
		coremodel.UUID(jujutesting.ModelTag.Id()): "mymodel",
		"deadbeef-1bad-500d-9000-4b1d0d06f666":    "anothermodel",
	}, nil)

	var legacyUpdated bool
	service := s.service().
		WithLegacyRemover(func(tag names.CloudCredentialTag) error {
			c.Assert(tag, jc.DeepEquals, names.NewCloudCredentialTag("cirrus/bob/foobar"))
			legacyUpdated = true
			return nil
		})

	err := service.CheckAndRevokeCredential(context.Background(), id, false)
	c.Assert(err, gc.ErrorMatches, `cannot revoke credential cirrus/bob/foobar: it is still used by 2 models`)
	c.Assert(legacyUpdated, jc.IsFalse)
}

func (s *serviceSuite) TestRevokeCredentialsHasModelForce(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := corecredential.ID{
		Cloud: "cirrus",
		Owner: "bob",
		Name:  "foobar",
	}

	s.state.EXPECT().ModelsUsingCloudCredential(gomock.Any(), id).Return(map[coremodel.UUID]string{
		coremodel.UUID(jujutesting.ModelTag.Id()): "mymodel",
	}, nil)
	s.state.EXPECT().RemoveCloudCredential(gomock.Any(), id).Return(nil)

	var legacyUpdated bool
	service := s.service().
		WithLegacyRemover(func(tag names.CloudCredentialTag) error {
			c.Assert(tag, jc.DeepEquals, names.NewCloudCredentialTag("cirrus/bob/foobar"))
			legacyUpdated = true
			return nil
		})

	err := service.CheckAndRevokeCredential(context.Background(), id, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(legacyUpdated, jc.IsTrue)
}

func (s *serviceSuite) TestRevokeCredentialsHasModelsForce(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := corecredential.ID{
		Cloud: "cirrus",
		Owner: "bob",
		Name:  "foobar",
	}

	s.state.EXPECT().ModelsUsingCloudCredential(gomock.Any(), id).Return(map[coremodel.UUID]string{
		coremodel.UUID(jujutesting.ModelTag.Id()): "mymodel",
		"deadbeef-1bad-500d-9000-4b1d0d06f666":    "anothermodel",
	}, nil)
	s.state.EXPECT().RemoveCloudCredential(gomock.Any(), id).Return(nil)

	var legacyUpdated bool
	service := s.service().
		WithLegacyRemover(func(tag names.CloudCredentialTag) error {
			c.Assert(tag, jc.DeepEquals, names.NewCloudCredentialTag("cirrus/bob/foobar"))
			legacyUpdated = true
			return nil
		})

	err := service.CheckAndRevokeCredential(context.Background(), id, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(legacyUpdated, jc.IsTrue)
}

func (s *serviceSuite) TestCheckAndRevokeCredentialInvalidID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := corecredential.ID{Cloud: "cirrus", Owner: "fred"}
	err := s.service().CheckAndRevokeCredential(context.Background(), id, false)
	c.Assert(err, gc.ErrorMatches, "invalid id revoking cloud credential.*")
}

type providerServiceSuite struct {
	baseSuite
}

var _ = gc.Suite(&providerServiceSuite{})

func (s *providerServiceSuite) service() *WatchableProviderService {
	return NewWatchableProviderService(s.state, s.watcherFactory)
}

func (s *providerServiceSuite) TestCloudCredential(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := credential.ID{Cloud: "cirrus", Owner: "fred", Name: "foo"}
	cred := credential.CloudCredentialResult{
		CloudCredentialInfo: credential.CloudCredentialInfo{
			AuthType: string(cloud.UserPassAuthType),
			Attributes: map[string]string{
				"hello": "world",
			},
			Label: "foo",
		},
	}
	s.state.EXPECT().CloudCredential(gomock.Any(), id).Return(cred, nil)

	result, err := s.service().CloudCredential(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, cloud.NewNamedCredential("foo", cloud.UserPassAuthType, map[string]string{"hello": "world"}, false))
}

func (s *providerServiceSuite) TestCloudCredentialInvalidID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := credential.ID{Cloud: "cirrus", Owner: "fred"}
	_, err := s.service().CloudCredential(context.Background(), id)
	c.Assert(err, gc.ErrorMatches, "invalid id getting cloud credential.*")
}

func (s *providerServiceSuite) TestWatchCredential(c *gc.C) {
	defer s.setupMocks(c).Finish()

	nw := watchertest.NewMockNotifyWatcher(nil)

	id := credential.ID{Cloud: "cirrus", Owner: "fred", Name: "foo"}
	s.state.EXPECT().WatchCredential(gomock.Any(), gomock.Any(), id).Return(nw, nil)

	w, err := s.service().WatchCredential(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
}

func (s *providerServiceSuite) TestWatchCredentialInvalidID(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := credential.ID{Cloud: "cirrus", Owner: "fred"}
	_, err := s.service().WatchCredential(context.Background(), id)
	c.Assert(err, gc.ErrorMatches, "invalid id watching cloud credential.*")
}
