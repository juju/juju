// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/common/credentialcommon"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain/credential"
	"github.com/juju/juju/domain/model"
	jujutesting "github.com/juju/juju/testing"
)

type serviceSuite struct {
	testing.IsolationSuite

	state          *MockState
	validator      *MockCredentialValidator
	watcherFactory *MockWatcherFactory
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.validator = NewMockCredentialValidator(ctrl)
	s.state = NewMockState(ctrl)
	s.watcherFactory = NewMockWatcherFactory(ctrl)

	return ctrl
}

func (s *serviceSuite) service() *Service {
	return NewService(s.state, s.watcherFactory, loggo.GetLogger("test"))
}

func (s *serviceSuite) TestUpdateCloudCredential(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewCloudCredentialTag("cirrus/fred/foo")
	cred := cloud.NewNamedCredential("foo", cloud.UserPassAuthType, map[string]string{"hello": "world"}, false)
	s.state.EXPECT().UpsertCloudCredential(gomock.Any(), "foo", "cirrus", "fred", cred).Return(nil, nil)

	err := s.service().UpdateCloudCredential(context.Background(), tag, cred)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestCloudCredentials(c *gc.C) {
	defer s.setupMocks(c).Finish()

	one := cloud.NewNamedCredential("foo", cloud.UserPassAuthType, map[string]string{"hello": "world"}, false)
	two := cloud.NewNamedCredential("foobar", cloud.UserPassAuthType, map[string]string{"hello": "world"}, false)
	s.state.EXPECT().CloudCredentials(gomock.Any(), "fred", "cirrus").Return(map[string]cloud.Credential{
		"foo":    one,
		"foobar": two,
	}, nil)

	creds, err := s.service().CloudCredentials(context.Background(), "fred", "cirrus")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(creds, jc.DeepEquals, map[string]cloud.Credential{
		"foo":    one,
		"foobar": two,
	})
}

func (s *serviceSuite) TestCloudCredential(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewCloudCredentialTag("cirrus/fred/foo")
	cred := cloud.NewNamedCredential("foo", cloud.UserPassAuthType, map[string]string{"hello": "world"}, false)
	s.state.EXPECT().CloudCredential(gomock.Any(), "foo", "cirrus", "fred").Return(cred, nil)

	result, err := s.service().CloudCredential(context.Background(), tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, cred)
}

func (s *serviceSuite) TestRemoveCloudCredential(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewCloudCredentialTag("cirrus/fred/foo")
	s.state.EXPECT().RemoveCloudCredential(gomock.Any(), "foo", "cirrus", "fred").Return(nil)

	err := s.service().RemoveCloudCredential(context.Background(), tag)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestInvalidateCloudCredential(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tag := names.NewCloudCredentialTag("cirrus/fred/foo")
	s.state.EXPECT().InvalidateCloudCredential(gomock.Any(), "foo", "cirrus", "fred", "gone bad").Return(nil)

	err := s.service().InvalidateCredential(context.Background(), tag, "gone bad")
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestAllCloudCredentials(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cred := cloud.NewNamedCredential("foo", cloud.UserPassAuthType, map[string]string{"hello": "world"}, false)
	s.state.EXPECT().AllCloudCredentials(gomock.Any(), "fred").Return([]credential.CloudCredential{{CloudName: "cirrus", Credential: cred}}, nil)

	result, err := s.service().AllCloudCredentials(context.Background(), "fred")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, []credential.CloudCredential{{CloudName: "cirrus", Credential: cred}})
}

func (s *serviceSuite) TestWatchCredential(c *gc.C) {
	defer s.setupMocks(c).Finish()

	nw := watchertest.NewMockNotifyWatcher(nil)

	tag := names.NewCloudCredentialTag("cirrus/fred/foo")
	s.state.EXPECT().WatchCredential(gomock.Any(), gomock.Any(), "foo", "cirrus", "fred").Return(nw, nil)

	w, err := s.service().WatchCredential(context.Background(), tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
}

var invalid = true

func (s *serviceSuite) TestCheckAndUpdateCredentialsNoModelsFound(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cred := cloud.Credential{}
	id := credential.ID{
		Cloud: "cirrus",
		Owner: "bob",
		Name:  "foobar",
	}

	s.state.EXPECT().ModelsUsingCloudCredential(gomock.Any(), id).Return(nil, errors.NotFound)
	s.state.EXPECT().GetCloud(gomock.Any(), "cirrus").Return(cloud.Cloud{Name: "cirrus"}, nil)
	s.state.EXPECT().UpsertCloudCredential(gomock.Any(), id.Name, id.Cloud, id.Owner, cred).Return(&invalid, nil)

	var legacyUpdated bool
	service := s.service().
		WithValidationContextGetter(func(modelUUID model.UUID) (credentialcommon.CredentialValidationContext, error) {
			return credentialcommon.CredentialValidationContext{}, errors.NotImplemented
		}).
		WithLegacyUpdater(func(tag names.CloudCredentialTag) error {
			c.Assert(tag, jc.DeepEquals, names.NewCloudCredentialTag("cirrus/bob/foobar"))
			legacyUpdated = true
			return nil
		})

	results, err := service.CheckAndUpdateCredential(context.Background(), id, cred, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 0)
	c.Assert(legacyUpdated, jc.IsTrue)
}

func (s *serviceSuite) TestUpdateCredentialsModelsError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cred := cloud.Credential{}
	id := credential.ID{
		Cloud: "cirrus",
		Owner: "bob",
		Name:  "foobar",
	}

	s.state.EXPECT().ModelsUsingCloudCredential(gomock.Any(), id).Return(nil, errors.New("cannot get models"))

	var legacyUpdated bool
	service := s.service().
		WithValidationContextGetter(func(modelUUID model.UUID) (credentialcommon.CredentialValidationContext, error) {
			return credentialcommon.CredentialValidationContext{}, errors.NotImplemented
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
	id := credential.ID{
		Cloud: "cirrus",
		Owner: "bob",
		Name:  "foobar",
	}

	s.state.EXPECT().ModelsUsingCloudCredential(gomock.Any(), id).Return(map[model.UUID]string{
		model.UUID(jujutesting.ModelTag.Id()): "mymodel",
	}, nil)
	s.state.EXPECT().GetCloud(gomock.Any(), "cirrus").Return(cloud.Cloud{Name: "cirrus"}, nil)

	contextError := errors.New("failed context")

	var legacyUpdated bool
	service := s.service().
		WithValidationContextGetter(func(modelUUID model.UUID) (credentialcommon.CredentialValidationContext, error) {
			return credentialcommon.CredentialValidationContext{}, contextError
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
		ModelUUID: model.UUID(jujutesting.ModelTag.Id()), ModelName: "mymodel",
	}})
	c.Assert(legacyUpdated, jc.IsFalse)
}

func (s *serviceSuite) TestUpdateCredentialsModelsFailedContextForce(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cred := cloud.Credential{}
	id := credential.ID{
		Cloud: "cirrus",
		Owner: "bob",
		Name:  "foobar",
	}

	s.state.EXPECT().ModelsUsingCloudCredential(gomock.Any(), id).Return(map[model.UUID]string{
		model.UUID(jujutesting.ModelTag.Id()): "mymodel",
	}, nil)
	s.state.EXPECT().GetCloud(gomock.Any(), "cirrus").Return(cloud.Cloud{Name: "cirrus"}, nil)
	s.state.EXPECT().UpsertCloudCredential(gomock.Any(), id.Name, id.Cloud, id.Owner, cred).Return(&invalid, nil)

	contextError := errors.New("failed context")

	var legacyUpdated bool
	service := s.service().
		WithValidationContextGetter(func(modelUUID model.UUID) (credentialcommon.CredentialValidationContext, error) {
			return credentialcommon.CredentialValidationContext{}, contextError
		}).
		WithLegacyUpdater(func(tag names.CloudCredentialTag) error {
			c.Assert(tag, jc.DeepEquals, names.NewCloudCredentialTag("cirrus/bob/foobar"))
			legacyUpdated = true
			return nil
		})

	results, err := service.CheckAndUpdateCredential(context.Background(), id, cred, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results, gc.HasLen, 1)
	c.Assert(results[0].Errors, gc.HasLen, 1)
	c.Assert(results[0].Errors[0], gc.ErrorMatches, "failed context")
	results[0].Errors = nil
	c.Assert(results, jc.DeepEquals, []UpdateCredentialModelResult{{
		ModelUUID: model.UUID(jujutesting.ModelTag.Id()), ModelName: "mymodel",
	}})
	c.Assert(legacyUpdated, jc.IsTrue)
}

func (s *serviceSuite) TestUpdateCredentialsModels(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cred := cloud.Credential{}
	id := credential.ID{
		Cloud: "cirrus",
		Owner: "bob",
		Name:  "foobar",
	}

	s.state.EXPECT().ModelsUsingCloudCredential(gomock.Any(), id).Return(map[model.UUID]string{
		model.UUID(jujutesting.ModelTag.Id()): "mymodel",
	}, nil)
	s.state.EXPECT().GetCloud(gomock.Any(), "cirrus").Return(cloud.Cloud{Name: "cirrus"}, nil)
	s.state.EXPECT().UpsertCloudCredential(gomock.Any(), id.Name, id.Cloud, id.Owner, cred).Return(&invalid, nil)
	s.validator.EXPECT().Validate(gomock.Any(), id, &cred, false).Return(nil, nil)

	var legacyUpdated bool
	service := s.service().
		WithValidationContextGetter(func(modelUUID model.UUID) (credentialcommon.CredentialValidationContext, error) {
			return credentialcommon.CredentialValidationContext{}, nil
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
		ModelUUID: model.UUID(jujutesting.ModelTag.Id()), ModelName: "mymodel",
	}})
	c.Assert(legacyUpdated, jc.IsTrue)
}

func (s *serviceSuite) TestUpdateCredentialsModelFailedValidationForce(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cred := cloud.Credential{}
	id := credential.ID{
		Cloud: "cirrus",
		Owner: "bob",
		Name:  "foobar",
	}

	s.state.EXPECT().ModelsUsingCloudCredential(gomock.Any(), id).Return(map[model.UUID]string{
		model.UUID(jujutesting.ModelTag.Id()): "mymodel",
	}, nil)
	s.state.EXPECT().GetCloud(gomock.Any(), "cirrus").Return(cloud.Cloud{Name: "cirrus"}, nil)
	s.state.EXPECT().UpsertCloudCredential(gomock.Any(), id.Name, id.Cloud, id.Owner, cred).Return(&invalid, nil)

	validationError := errors.New("cred error")
	s.validator.EXPECT().Validate(gomock.Any(), id, &cred, false).Return([]error{validationError}, nil)

	var legacyUpdated bool
	service := s.service().
		WithValidationContextGetter(func(modelUUID model.UUID) (credentialcommon.CredentialValidationContext, error) {
			return credentialcommon.CredentialValidationContext{}, nil
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
		ModelUUID: model.UUID(jujutesting.ModelTag.Id()), ModelName: "mymodel", Errors: []error{validationError},
	}})
	c.Assert(legacyUpdated, jc.IsTrue)
}

func (s *serviceSuite) TestUpdateCredentialsSomeModelsFailedValidation(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cred := cloud.Credential{}
	id := credential.ID{
		Cloud: "cirrus",
		Owner: "bob",
		Name:  "foobar",
	}

	s.state.EXPECT().ModelsUsingCloudCredential(gomock.Any(), id).Return(map[model.UUID]string{
		model.UUID(jujutesting.ModelTag.Id()):  "mymodel",
		"deadbeef-1bad-500d-9000-4b1d0d06f666": "anothermodel",
	}, nil)
	s.state.EXPECT().GetCloud(gomock.Any(), "cirrus").Return(cloud.Cloud{Name: "cirrus"}, nil)

	validationError := errors.New("cred error")
	calls := 0
	s.validator.EXPECT().Validate(gomock.Any(), id, &cred, false).DoAndReturn(func(
		ctx credentialcommon.CredentialValidationContext,
		credentialID credential.ID,
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
		WithValidationContextGetter(func(modelUUID model.UUID) (credentialcommon.CredentialValidationContext, error) {
			return credentialcommon.CredentialValidationContext{}, nil
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
		ModelUUID: model.UUID(jujutesting.ModelTag.Id()), ModelName: "mymodel",
	}, {
		ModelUUID: "deadbeef-1bad-500d-9000-4b1d0d06f666", ModelName: "anothermodel",
	}})
	c.Assert(legacyUpdated, jc.IsFalse)
}

func (s *serviceSuite) TestUpdateCredentialsSomeModelsFailedValidationForce(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cred := cloud.Credential{}
	id := credential.ID{
		Cloud: "cirrus",
		Owner: "bob",
		Name:  "foobar",
	}

	s.state.EXPECT().ModelsUsingCloudCredential(gomock.Any(), id).Return(map[model.UUID]string{
		model.UUID(jujutesting.ModelTag.Id()):  "mymodel",
		"deadbeef-1bad-500d-9000-4b1d0d06f666": "anothermodel",
	}, nil)
	s.state.EXPECT().GetCloud(gomock.Any(), "cirrus").Return(cloud.Cloud{Name: "cirrus"}, nil)
	s.state.EXPECT().UpsertCloudCredential(gomock.Any(), id.Name, id.Cloud, id.Owner, cred).Return(&invalid, nil)

	validationError := errors.New("cred error")
	calls := 0
	s.validator.EXPECT().Validate(gomock.Any(), id, &cred, false).DoAndReturn(func(
		ctx credentialcommon.CredentialValidationContext,
		credentialID credential.ID,
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
		WithValidationContextGetter(func(modelUUID model.UUID) (credentialcommon.CredentialValidationContext, error) {
			return credentialcommon.CredentialValidationContext{}, nil
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
		ModelUUID: model.UUID(jujutesting.ModelTag.Id()), ModelName: "mymodel",
	}, {
		ModelUUID: "deadbeef-1bad-500d-9000-4b1d0d06f666", ModelName: "anothermodel",
	}})
	c.Assert(legacyUpdated, jc.IsTrue)
}

func (s *serviceSuite) TestRevokeCredentialsModelsError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := credential.ID{
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

	id := credential.ID{
		Cloud: "cirrus",
		Owner: "bob",
		Name:  "foobar",
	}

	s.state.EXPECT().ModelsUsingCloudCredential(gomock.Any(), id).Return(map[model.UUID]string{
		model.UUID(jujutesting.ModelTag.Id()): "mymodel",
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

	id := credential.ID{
		Cloud: "cirrus",
		Owner: "bob",
		Name:  "foobar",
	}

	s.state.EXPECT().ModelsUsingCloudCredential(gomock.Any(), id).Return(map[model.UUID]string{
		model.UUID(jujutesting.ModelTag.Id()):  "mymodel",
		"deadbeef-1bad-500d-9000-4b1d0d06f666": "anothermodel",
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

	id := credential.ID{
		Cloud: "cirrus",
		Owner: "bob",
		Name:  "foobar",
	}

	s.state.EXPECT().ModelsUsingCloudCredential(gomock.Any(), id).Return(map[model.UUID]string{
		model.UUID(jujutesting.ModelTag.Id()): "mymodel",
	}, nil)
	s.state.EXPECT().RemoveCloudCredential(gomock.Any(), id.Name, id.Cloud, id.Owner).Return(nil)

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
	c.Assert(c.GetTestLog(), jc.Contains,
		" WARNING test credential cirrus/bob/foobar will be deleted but it is used by model deadbeef-0bad-400d-8000-4b1d0d06f00d")
}

func (s *serviceSuite) TestRevokeCredentialsHasModelsForce(c *gc.C) {
	defer s.setupMocks(c).Finish()

	id := credential.ID{
		Cloud: "cirrus",
		Owner: "bob",
		Name:  "foobar",
	}

	s.state.EXPECT().ModelsUsingCloudCredential(gomock.Any(), id).Return(map[model.UUID]string{
		model.UUID(jujutesting.ModelTag.Id()):  "mymodel",
		"deadbeef-1bad-500d-9000-4b1d0d06f666": "anothermodel",
	}, nil)
	s.state.EXPECT().RemoveCloudCredential(gomock.Any(), id.Name, id.Cloud, id.Owner).Return(nil)

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
	c.Assert(c.GetTestLog(), jc.Contains,
		` WARNING test credential cirrus/bob/foobar will be deleted but it is used by models:
- deadbeef-0bad-400d-8000-4b1d0d06f00d
- deadbeef-1bad-500d-9000-4b1d0d06f666`)
}
