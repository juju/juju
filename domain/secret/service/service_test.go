// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	domainsecret "github.com/juju/juju/domain/secret"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/secrets/provider"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

type serviceSuite struct {
	testing.IsolationSuite

	clock                  *testclock.Clock
	modelID                coremodel.UUID
	backendConfigGetter    BackendAdminConfigGetter
	userSecretConfigGetter BackendUserSecretConfigGetter
	secretsBackend         *MockSecretsBackend
	secretsBackendProvider *MockSecretBackendProvider

	state                         *MockState
	secretBackendReferenceMutator *MockSecretBackendReferenceMutator

	service  *SecretService
	fakeUUID uuid.UUID
}

var _ = gc.Suite(&serviceSuite{})

var backendConfigs = &provider.ModelBackendConfigInfo{
	ActiveID: "backend-id",
	Configs: map[string]provider.ModelBackendConfig{
		"backend-id": {
			ControllerUUID: coretesting.ControllerTag.Id(),
			ModelUUID:      coretesting.ModelTag.Id(),
			ModelName:      "some-model",
			BackendConfig: provider.BackendConfig{
				BackendType: "active-type",
				Config:      map[string]interface{}{"foo": "active-type"},
			},
		},
		"other-backend-id": {
			ControllerUUID: coretesting.ControllerTag.Id(),
			ModelUUID:      coretesting.ModelTag.Id(),
			ModelName:      "some-model",
			BackendConfig: provider.BackendConfig{
				BackendType: "other-type",
				Config:      map[string]interface{}{"foo": "other-type"},
			},
		},
	},
}

func (s *serviceSuite) SetUpTest(c *gc.C) {
	s.modelID = modeltesting.GenModelUUID(c)
	var err error
	s.fakeUUID, err = uuid.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	s.clock = testclock.NewClock(time.Time{})
	s.backendConfigGetter = func(context.Context) (*provider.ModelBackendConfigInfo, error) {
		return backendConfigs, nil
	}
	s.userSecretConfigGetter = func(context.Context, GrantedSecretsGetter, SecretAccessor) (*provider.ModelBackendConfigInfo, error) {
		return backendConfigs, nil
	}
}

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	s.secretBackendReferenceMutator = NewMockSecretBackendReferenceMutator(ctrl)
	s.secretsBackendProvider = NewMockSecretBackendProvider(ctrl)
	s.secretsBackend = NewMockSecretsBackend(ctrl)

	s.service = &SecretService{
		secretState:                   s.state,
		secretBackendReferenceMutator: s.secretBackendReferenceMutator,
		logger:                        loggertesting.WrapCheckLog(c),
		clock:                         s.clock,
		providerGetter:                func(string) (provider.SecretBackendProvider, error) { return s.secretsBackendProvider, nil },
		adminConfigGetter:             s.backendConfigGetter,
		userSecretConfigGetter:        s.userSecretConfigGetter,
		uuidGenerator:                 func() (uuid.UUID, error) { return s.fakeUUID, nil },
	}
	return ctrl
}

func (s *serviceSuite) expectRunAtomic(ctrl *gomock.Controller, anyTimes bool) {
	if anyTimes {
		s.state.EXPECT().RunAtomic(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, f func(domain.AtomicContext) error) error {
			return f(NewMockAtomicContext(ctrl))
		}).AnyTimes()
	} else {
		s.state.EXPECT().RunAtomic(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, f func(domain.AtomicContext) error) error {
			return f(NewMockAtomicContext(ctrl))
		})
	}
}

type successfulToken struct{}

func (t successfulToken) Check() error {
	return nil
}

func (s *serviceSuite) TestCreateUserSecretURIs(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(coretesting.ModelTag.Id(), nil)

	got, err := s.service.CreateSecretURIs(context.Background(), 2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.HasLen, 2)
	c.Assert(got[0].SourceUUID, gc.Equals, coretesting.ModelTag.Id())
	c.Assert(got[1].SourceUUID, gc.Equals, coretesting.ModelTag.Id())
}

func (s *serviceSuite) TestCreateUserSecretInternal(c *gc.C) {
	s.assertCreateUserSecret(c, true, false)
}
func (s *serviceSuite) TestCreateUserSecretExternalBackend(c *gc.C) {
	s.assertCreateUserSecret(c, false, false)
}

func (s *serviceSuite) TestCreateUserSecretExternalBackendFailedAndCleanup(c *gc.C) {
	s.assertCreateUserSecret(c, false, true)
}

func (s *serviceSuite) assertCreateUserSecret(c *gc.C, isInternal, finalStepFailed bool) {
	defer s.setupMocks(c).Finish()

	params := domainsecret.UpsertSecretParams{
		Description: ptr("a secret"),
		Label:       ptr("my secret"),
		AutoPrune:   ptr(true),
		Checksum:    "checksum-1234",
		RevisionID:  ptr(s.fakeUUID.String()),
	}
	if isInternal {
		params.Data = map[string]string{"foo": "bar"}
	} else {
		params.ValueRef = &coresecrets.ValueRef{
			BackendID:  "backend-id",
			RevisionID: "rev-id",
		}
	}

	s.secretsBackendProvider.EXPECT().Type().Return("active-type").AnyTimes()
	s.secretsBackendProvider.EXPECT().NewBackend(ptr(backendConfigs.Configs["backend-id"])).DoAndReturn(func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
		return s.secretsBackend, nil
	})

	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelID.String(), nil)
	uri := coresecrets.NewURI()
	rollbackCalled := false
	s.secretBackendReferenceMutator.EXPECT().AddSecretBackendReference(gomock.Any(), params.ValueRef, s.modelID, s.fakeUUID.String()).Return(
		func() error {
			rollbackCalled = true
			return nil
		}, nil,
	)
	if isInternal {
		s.secretsBackend.EXPECT().SaveContent(gomock.Any(), uri, 1, coresecrets.NewSecretValue(map[string]string{"foo": "bar"})).
			Return("", errors.NotSupportedf("not supported"))

	} else {
		s.secretsBackend.EXPECT().SaveContent(gomock.Any(), uri, 1, coresecrets.NewSecretValue(map[string]string{"foo": "bar"})).
			Return("rev-id", nil)
	}
	if finalStepFailed && !isInternal {
		s.secretsBackend.EXPECT().DeleteContent(gomock.Any(), "rev-id").Return(nil)
	}

	s.state.EXPECT().CreateUserSecret(gomock.Any(), 1, uri, params).
		DoAndReturn(func(context.Context, int, *coresecrets.URI, domainsecret.UpsertSecretParams) error {
			if finalStepFailed {
				return errors.New("some error")
			}
			return nil
		})

	err := s.service.CreateUserSecret(context.Background(), uri, CreateUserSecretParams{
		UpdateUserSecretParams: UpdateUserSecretParams{
			Description: ptr("a secret"),
			Label:       ptr("my secret"),
			Data:        map[string]string{"foo": "bar"},
			AutoPrune:   ptr(true),
			Checksum:    "checksum-1234",
		},
		Version: 1,
	})
	if finalStepFailed {
		c.Assert(rollbackCalled, jc.IsTrue)
		c.Assert(err, gc.ErrorMatches, "cannot create user secret .*some error")
	} else {
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *serviceSuite) TestUpdateUserSecretInternal(c *gc.C) {
	s.assertUpdateUserSecret(c, true, false)
}
func (s *serviceSuite) TestUpdateUserSecretExternalBackend(c *gc.C) {
	s.assertUpdateUserSecret(c, false, false)
}

func (s *serviceSuite) TestUpdateUserSecretExternalBackendFailedAndCleanup(c *gc.C) {
	s.assertUpdateUserSecret(c, false, true)
}

func (s *serviceSuite) assertUpdateUserSecret(c *gc.C, isInternal, finalStepFailed bool) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectRunAtomic(ctrl, false)

	s.secretsBackendProvider.EXPECT().Type().Return("active-type").AnyTimes()
	s.secretsBackendProvider.EXPECT().NewBackend(ptr(backendConfigs.Configs["backend-id"])).DoAndReturn(func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
		return s.secretsBackend, nil
	})

	uri := coresecrets.NewURI()
	if isInternal {
		s.secretsBackend.EXPECT().SaveContent(gomock.Any(), uri, 3, coresecrets.NewSecretValue(map[string]string{"foo": "bar"})).
			Return("", errors.NotSupportedf("not supported"))
	} else {
		s.secretsBackend.EXPECT().SaveContent(gomock.Any(), uri, 3, coresecrets.NewSecretValue(map[string]string{"foo": "bar"})).
			Return("rev-id", nil)
	}
	if finalStepFailed && !isInternal {
		s.secretsBackend.EXPECT().DeleteContent(gomock.Any(), "rev-id").Return(nil)
	}

	params := domainsecret.UpsertSecretParams{
		Description: ptr("a secret"),
		Label:       ptr("my secret"),
		AutoPrune:   ptr(true),
		Checksum:    "checksum-1234",
		RevisionID:  ptr(s.fakeUUID.String()),
	}
	if isInternal {
		params.Data = map[string]string{"foo": "bar"}
	} else {
		params.ValueRef = &coresecrets.ValueRef{
			BackendID:  "backend-id",
			RevisionID: "rev-id",
		}
	}

	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mariadb/0",
	}).Return("manage", nil)
	s.state.EXPECT().GetLatestRevision(gomock.Any(), uri).Return(2, nil)
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelID.String(), nil)
	rollbackCalled := false
	s.secretBackendReferenceMutator.EXPECT().AddSecretBackendReference(gomock.Any(), params.ValueRef, s.modelID, s.fakeUUID.String()).Return(func() error {
		rollbackCalled = true
		return nil
	}, nil)
	s.state.EXPECT().UpdateSecret(gomock.Any(), uri, params).
		DoAndReturn(func(domain.AtomicContext, *coresecrets.URI, domainsecret.UpsertSecretParams) error {
			if finalStepFailed {
				return errors.New("some error")
			}
			return nil
		})

	err := s.service.UpdateUserSecret(context.Background(), uri, UpdateUserSecretParams{
		Accessor: SecretAccessor{
			Kind: UnitAccessor,
			ID:   "mariadb/0",
		},
		Description: ptr("a secret"),
		Label:       ptr("my secret"),
		Data:        map[string]string{"foo": "bar"},
		Checksum:    "checksum-1234",
		AutoPrune:   ptr(true),
	})
	if finalStepFailed {
		c.Assert(rollbackCalled, jc.IsTrue)
		c.Assert(err, gc.ErrorMatches, "cannot update user secret .*some error")
	} else {
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *serviceSuite) TestCreateCharmUnitSecret(c *gc.C) {
	defer s.setupMocks(c).Finish()

	exipreTime := s.clock.Now()
	rotateTime := s.clock.Now().Add(time.Hour)
	uri := coresecrets.NewURI()
	p := domainsecret.UpsertSecretParams{
		RotatePolicy:   ptr(domainsecret.RotateHourly),
		Description:    ptr("a secret"),
		Label:          ptr("my secret"),
		Data:           coresecrets.SecretData{"foo": "bar"},
		Checksum:       "checksum-1234",
		ExpireTime:     ptr(exipreTime),
		NextRotateTime: ptr(rotateTime),
		RevisionID:     ptr(s.fakeUUID.String()),
	}

	s.state.EXPECT().CreateCharmUnitSecret(gomock.Any(), 1, uri, "mariadb/0", gomock.AssignableToTypeOf(p)).
		DoAndReturn(func(_ context.Context, _ int, _ *coresecrets.URI, _ string, got domainsecret.UpsertSecretParams) error {
			c.Assert(got.NextRotateTime, gc.NotNil)
			c.Assert(*got.NextRotateTime, jc.Almost, rotateTime)
			got.NextRotateTime = nil
			want := p
			want.NextRotateTime = nil
			c.Assert(got, jc.DeepEquals, want)
			return nil
		})
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelID.String(), nil)
	rollbackCalled := false
	s.secretBackendReferenceMutator.EXPECT().AddSecretBackendReference(gomock.Any(), nil, s.modelID, s.fakeUUID.String()).Return(func() error {
		rollbackCalled = true
		return nil
	}, nil)

	err := s.service.CreateCharmSecret(context.Background(), uri, CreateCharmSecretParams{
		UpdateCharmSecretParams: UpdateCharmSecretParams{
			LeaderToken: successfulToken{},
			Accessor: SecretAccessor{
				Kind: UnitAccessor,
				ID:   "mariadb/0",
			},
			Description:  ptr("a secret"),
			Label:        ptr("my secret"),
			Data:         map[string]string{"foo": "bar"},
			Checksum:     "checksum-1234",
			ExpireTime:   ptr(exipreTime),
			RotatePolicy: ptr(coresecrets.RotateHourly),
		},
		Version: 1,
		CharmOwner: CharmSecretOwner{
			Kind: UnitOwner,
			ID:   "mariadb/0",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rollbackCalled, jc.IsFalse)
}

func (s *serviceSuite) TestCreateCharmApplicationSecret(c *gc.C) {
	defer s.setupMocks(c).Finish()

	exipreTime := s.clock.Now()
	rotateTime := s.clock.Now().Add(time.Hour)
	uri := coresecrets.NewURI()
	p := domainsecret.UpsertSecretParams{
		RotatePolicy:   ptr(domainsecret.RotateHourly),
		Description:    ptr("a secret"),
		Label:          ptr("my secret"),
		Data:           coresecrets.SecretData{"foo": "bar"},
		Checksum:       "checksum-1234",
		ExpireTime:     ptr(exipreTime),
		NextRotateTime: ptr(rotateTime),
		RevisionID:     ptr(s.fakeUUID.String()),
	}

	s.state.EXPECT().CreateCharmApplicationSecret(gomock.Any(), 1, uri, "mariadb", gomock.AssignableToTypeOf(p)).
		DoAndReturn(func(_ context.Context, _ int, _ *coresecrets.URI, _ string, got domainsecret.UpsertSecretParams) error {
			c.Assert(got.NextRotateTime, gc.NotNil)
			c.Assert(*got.NextRotateTime, jc.Almost, rotateTime)
			got.NextRotateTime = nil
			want := p
			want.NextRotateTime = nil
			c.Assert(got, jc.DeepEquals, want)
			return nil
		})
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelID.String(), nil)
	rollbackCalled := false
	s.secretBackendReferenceMutator.EXPECT().AddSecretBackendReference(gomock.Any(), nil, s.modelID, s.fakeUUID.String()).Return(func() error {
		rollbackCalled = true
		return nil
	}, nil)

	err := s.service.CreateCharmSecret(context.Background(), uri, CreateCharmSecretParams{
		UpdateCharmSecretParams: UpdateCharmSecretParams{
			LeaderToken: successfulToken{},
			Accessor: SecretAccessor{
				Kind: UnitAccessor,
				ID:   "mariadb/0",
			},
			Description:  ptr("a secret"),
			Label:        ptr("my secret"),
			Data:         map[string]string{"foo": "bar"},
			Checksum:     "checksum-1234",
			ExpireTime:   ptr(exipreTime),
			RotatePolicy: ptr(coresecrets.RotateHourly),
		},
		Version: 1,
		CharmOwner: CharmSecretOwner{
			Kind: ApplicationOwner,
			ID:   "mariadb",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rollbackCalled, jc.IsFalse)
}

func (s *serviceSuite) TestUpdateCharmSecretNoRotate(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectRunAtomic(ctrl, false)

	exipreTime := s.clock.Now()
	uri := coresecrets.NewURI()
	p := domainsecret.UpsertSecretParams{
		RotatePolicy: ptr(domainsecret.RotateNever),
		Description:  ptr("a secret"),
		Label:        ptr("my secret"),
		Data:         coresecrets.SecretData{"foo": "bar"},
		Checksum:     "checksum-1234",
		ExpireTime:   ptr(exipreTime),
		RevisionID:   ptr(s.fakeUUID.String()),
	}

	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mariadb/0",
	}).Return("manage", nil)
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelID.String(), nil)
	rollbackCalled := false
	s.secretBackendReferenceMutator.EXPECT().AddSecretBackendReference(gomock.Any(), nil, s.modelID, s.fakeUUID.String()).Return(func() error {
		rollbackCalled = true
		return nil
	}, nil)
	s.state.EXPECT().UpdateSecret(gomock.Any(), uri, p).Return(nil)

	err := s.service.UpdateCharmSecret(context.Background(), uri, UpdateCharmSecretParams{
		LeaderToken: successfulToken{},
		Accessor: SecretAccessor{
			Kind: UnitAccessor,
			ID:   "mariadb/0",
		},
		Description: ptr("a secret"),
		Label:       ptr("my secret"),
		Data:        map[string]string{"foo": "bar"},
		Checksum:    "checksum-1234",
		ExpireTime:  ptr(exipreTime),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rollbackCalled, jc.IsFalse)
}

func (s *serviceSuite) TestUpdateCharmSecret(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectRunAtomic(ctrl, false)

	uri := coresecrets.NewURI()
	p := domainsecret.UpsertSecretParams{
		RotatePolicy:   ptr(domainsecret.RotateDaily),
		Description:    ptr("a secret"),
		Label:          ptr("my secret"),
		Data:           coresecrets.SecretData{"foo": "bar"},
		Checksum:       "checksum-1234",
		NextRotateTime: ptr(s.clock.Now().AddDate(0, 0, 1)),
		RevisionID:     ptr(s.fakeUUID.String()),
	}

	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mariadb/0",
	}).Return("manage", nil)
	s.state.EXPECT().GetRotatePolicy(gomock.Any(), uri).Return(
		coresecrets.RotateNever, // No rotate policy.
		nil)

	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelID.String(), nil)
	rollbackCalled := false
	s.secretBackendReferenceMutator.EXPECT().AddSecretBackendReference(gomock.Any(), nil, s.modelID, s.fakeUUID.String()).Return(func() error {
		rollbackCalled = true
		return nil
	}, nil)
	s.state.EXPECT().UpdateSecret(gomock.Any(), uri, gomock.Any()).DoAndReturn(func(_ domain.AtomicContext, _ *coresecrets.URI, got domainsecret.UpsertSecretParams) error {
		c.Assert(got.NextRotateTime, gc.NotNil)
		c.Assert(*got.NextRotateTime, jc.Almost, *p.NextRotateTime)
		got.NextRotateTime = nil
		want := p
		want.NextRotateTime = nil
		c.Assert(got, jc.DeepEquals, want)
		return nil
	})

	err := s.service.UpdateCharmSecret(context.Background(), uri, UpdateCharmSecretParams{
		LeaderToken: successfulToken{},
		Accessor: SecretAccessor{
			Kind: UnitAccessor,
			ID:   "mariadb/0",
		},
		Description:  ptr("a secret"),
		Label:        ptr("my secret"),
		Data:         map[string]string{"foo": "bar"},
		Checksum:     "checksum-1234",
		RotatePolicy: ptr(coresecrets.RotateDaily),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rollbackCalled, jc.IsFalse)
}

func (s *serviceSuite) TestGetSecret(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	md := &coresecrets.SecretMetadata{
		URI:   uri,
		Label: "my secret",
	}

	s.state.EXPECT().GetSecret(gomock.Any(), uri).Return(md, nil)

	got, err := s.service.GetSecret(context.Background(), uri)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, md)
}

func (s *serviceSuite) TestGetSecretValue(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectRunAtomic(ctrl, false)

	uri := coresecrets.NewURI()

	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mariadb/0",
	}).Return("manage", nil)
	s.state.EXPECT().GetSecretValue(gomock.Any(), uri, 666).Return(coresecrets.SecretData{"foo": "bar"}, nil, nil)

	data, ref, err := s.service.GetSecretValue(context.Background(), uri, 666, SecretAccessor{
		Kind: UnitAccessor,
		ID:   "mariadb/0",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ref, gc.IsNil)
	c.Assert(data, jc.DeepEquals, coresecrets.NewSecretValue(map[string]string{"foo": "bar"}))
}

func (s *serviceSuite) TestGetSecretConsumer(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectRunAtomic(ctrl, false)

	uri := coresecrets.NewURI()
	consumer := &coresecrets.SecretConsumerMetadata{
		Label:           "my secret",
		CurrentRevision: 666,
	}

	s.state.EXPECT().GetSecretConsumer(gomock.Any(), uri, "mysql/0").Return(consumer, 666, nil)

	got, err := s.service.GetSecretConsumer(context.Background(), uri, "mysql/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, consumer)
}

func (s *serviceSuite) TestGetSecretConsumerAndLatest(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectRunAtomic(ctrl, false)

	uri := coresecrets.NewURI()
	consumer := &coresecrets.SecretConsumerMetadata{
		Label:           "my secret",
		CurrentRevision: 666,
	}

	s.state.EXPECT().GetSecretConsumer(gomock.Any(), uri, "mysql/0").Return(consumer, 666, nil)

	got, latest, err := s.service.GetSecretConsumerAndLatest(context.Background(), uri, "mysql/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, consumer)
	c.Assert(latest, gc.Equals, 666)
}

func (s *serviceSuite) TestSaveSecretConsumer(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectRunAtomic(ctrl, false)

	uri := coresecrets.NewURI()
	consumer := &coresecrets.SecretConsumerMetadata{
		Label:           "my secret",
		CurrentRevision: 666,
	}

	s.state.EXPECT().SaveSecretConsumer(gomock.Any(), uri, "mysql/0", consumer).Return(nil)

	err := s.service.SaveSecretConsumer(context.Background(), uri, "mysql/0", consumer)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestGetUserSecretURIByLabel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()

	s.state.EXPECT().GetUserSecretURIByLabel(gomock.Any(), "my label").Return(uri, nil)

	got, err := s.service.GetUserSecretURIByLabel(context.Background(), "my label")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, uri)
}

func (s *serviceSuite) TestListCharmSecretsToDrain(c *gc.C) {
	defer s.setupMocks(c).Finish()

	md := []*coresecrets.SecretMetadataForDrain{{
		URI: coresecrets.NewURI(),
		Revisions: []coresecrets.SecretExternalRevision{{
			Revision: 666,
			ValueRef: &coresecrets.ValueRef{
				BackendID:  "backend-id",
				RevisionID: "rev-id",
			},
		}},
	}}

	s.state.EXPECT().ListCharmSecretsToDrain(
		gomock.Any(), domainsecret.ApplicationOwners{"mariadb"}, domainsecret.UnitOwners{"mariadb/0"}).Return(md, nil)

	got, err := s.service.ListCharmSecretsToDrain(context.Background(), []CharmSecretOwner{{
		Kind: UnitOwner,
		ID:   "mariadb/0",
	}, {
		Kind: ApplicationOwner,
		ID:   "mariadb",
	}}...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, md)
}

func (s *serviceSuite) TestListUserSecretsToDrain(c *gc.C) {
	defer s.setupMocks(c).Finish()

	md := []*coresecrets.SecretMetadataForDrain{{
		URI: coresecrets.NewURI(),
		Revisions: []coresecrets.SecretExternalRevision{{
			Revision: 666,
			ValueRef: &coresecrets.ValueRef{
				BackendID:  "backend-id",
				RevisionID: "rev-id",
			},
		}},
	}}

	s.state.EXPECT().ListUserSecretsToDrain(gomock.Any()).Return(md, nil)

	got, err := s.service.ListUserSecretsToDrain(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, md)
}

func (s *serviceSuite) TestListCharmSecrets(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	owners := []CharmSecretOwner{{
		Kind: ApplicationOwner,
		ID:   "mysql",
	}, {
		Kind: UnitOwner,
		ID:   "mysql/0",
	}}
	md := []*coresecrets.SecretMetadata{{Label: "one"}}
	revs := [][]*coresecrets.SecretRevisionMetadata{{{Revision: 1}}}

	s.state.EXPECT().ListCharmSecretsWithRevisions(gomock.Any(), domainsecret.ApplicationOwners{"mysql"}, domainsecret.UnitOwners{"mysql/0"}).
		Return(md, revs, nil)

	gotSecrets, gotRevisions, err := s.service.ListCharmSecrets(context.Background(), owners...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotSecrets, jc.DeepEquals, md)
	c.Assert(gotRevisions, jc.DeepEquals, revs)
}

func (s *serviceSuite) TestListCharmJustApplication(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	owners := []CharmSecretOwner{{
		Kind: ApplicationOwner,
		ID:   "mysql",
	}}
	md := []*coresecrets.SecretMetadata{{Label: "one"}}
	revs := [][]*coresecrets.SecretRevisionMetadata{{{Revision: 1}}}

	s.state.EXPECT().ListCharmSecretsWithRevisions(gomock.Any(), domainsecret.ApplicationOwners{"mysql"}, domainsecret.NilUnitOwners).
		Return(md, revs, nil)

	gotSecrets, gotRevisions, err := s.service.ListCharmSecrets(context.Background(), owners...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotSecrets, jc.DeepEquals, md)
	c.Assert(gotRevisions, jc.DeepEquals, revs)
}

func (s *serviceSuite) TestListCharmJustUnit(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	owners := []CharmSecretOwner{{
		Kind: UnitOwner,
		ID:   "mysql/0",
	}}
	md := []*coresecrets.SecretMetadata{{Label: "one"}}
	revs := [][]*coresecrets.SecretRevisionMetadata{{{Revision: 1}}}

	s.state.EXPECT().ListCharmSecretsWithRevisions(gomock.Any(), domainsecret.NilApplicationOwners, domainsecret.UnitOwners{"mysql/0"}).
		Return(md, revs, nil)

	gotSecrets, gotRevisions, err := s.service.ListCharmSecrets(context.Background(), owners...)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotSecrets, jc.DeepEquals, md)
	c.Assert(gotRevisions, jc.DeepEquals, revs)
}

func (s *serviceSuite) TestGetURIByConsumerLabel(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	s.state.EXPECT().GetURIByConsumerLabel(gomock.Any(), "my label", "mysql/0").Return(uri, nil)

	got, err := s.service.GetURIByConsumerLabel(context.Background(), "my label", "mysql/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, jc.DeepEquals, uri)
}

func (s *serviceSuite) TestUpdateRemoteSecretRevision(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	s.state.EXPECT().UpdateRemoteSecretRevision(gomock.Any(), uri, 666).Return(nil)

	err := s.service.UpdateRemoteSecretRevision(context.Background(), uri, 666)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateRemoteConsumedRevision(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectRunAtomic(ctrl, false)

	uri := coresecrets.NewURI()
	s.state.EXPECT().GetSecretRemoteConsumer(gomock.Any(), uri, "remote-app/0").
		Return(&coresecrets.SecretConsumerMetadata{}, 666, nil)

	got, err := s.service.UpdateRemoteConsumedRevision(context.Background(), uri, "remote-app/0", false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.Equals, 666)
}

func (s *serviceSuite) TestUpdateRemoteConsumedRevisionRefresh(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectRunAtomic(ctrl, false)

	consumer := &coresecrets.SecretConsumerMetadata{
		CurrentRevision: 666,
	}
	uri := coresecrets.NewURI()
	s.state.EXPECT().GetSecretRemoteConsumer(gomock.Any(), uri, "remote-app/0").
		Return(&coresecrets.SecretConsumerMetadata{}, 666, nil)
	s.state.EXPECT().SaveSecretRemoteConsumer(gomock.Any(), uri, "remote-app/0", consumer).Return(nil)

	got, err := s.service.UpdateRemoteConsumedRevision(context.Background(), uri, "remote-app/0", true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.Equals, 666)
}

func (s *serviceSuite) TestUpdateRemoteConsumedRevisionFirstTimeRefresh(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectRunAtomic(ctrl, false)

	consumer := &coresecrets.SecretConsumerMetadata{
		CurrentRevision: 666,
	}
	uri := coresecrets.NewURI()
	s.state.EXPECT().GetSecretRemoteConsumer(gomock.Any(), uri, "remote-app/0").
		Return(nil, 666, secreterrors.SecretConsumerNotFound)
	s.state.EXPECT().SaveSecretRemoteConsumer(gomock.Any(), uri, "remote-app/0", consumer).Return(nil)

	got, err := s.service.UpdateRemoteConsumedRevision(context.Background(), uri, "remote-app/0", true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(got, gc.Equals, 666)
}

func (s *serviceSuite) TestGrantSecretUnitAccess(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectRunAtomic(ctrl, false)

	uri := coresecrets.NewURI()
	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "another/0",
	}).Return("manage", nil)
	s.state.EXPECT().GrantAccess(gomock.Any(), uri, domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeApplication,
		ScopeID:       "mysql",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
		RoleID:        domainsecret.RoleManage,
	}).Return(nil)

	err := s.service.GrantSecretAccess(context.Background(), uri, SecretAccessParams{
		LeaderToken: successfulToken{},
		Accessor: SecretAccessor{
			Kind: UnitAccessor,
			ID:   "another/0",
		},
		Scope: SecretAccessScope{
			Kind: ApplicationAccessScope,
			ID:   "mysql",
		},
		Subject: SecretAccessor{
			Kind: UnitAccessor,
			ID:   "mysql/0",
		},
		Role: "manage",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestGrantSecretApplicationAccess(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectRunAtomic(ctrl, false)

	uri := coresecrets.NewURI()
	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "another/0",
	}).Return("manage", nil)
	s.state.EXPECT().GrantAccess(gomock.Any(), uri, domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeApplication,
		ScopeID:       "mysql",
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
		RoleID:        domainsecret.RoleView,
	}).Return(nil)

	err := s.service.GrantSecretAccess(context.Background(), uri, SecretAccessParams{
		LeaderToken: successfulToken{},
		Accessor: SecretAccessor{
			Kind: UnitAccessor,
			ID:   "another/0",
		},
		Scope: SecretAccessScope{
			Kind: ApplicationAccessScope,
			ID:   "mysql",
		},
		Subject: SecretAccessor{
			Kind: ApplicationAccessor,
			ID:   "mysql",
		},
		Role: "view",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestGrantSecretModelAccess(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectRunAtomic(ctrl, false)

	uri := coresecrets.NewURI()
	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectModel,
		SubjectID:     "model-uuid",
	}).Return("manage", nil)
	s.state.EXPECT().GrantAccess(gomock.Any(), uri, domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeModel,
		SubjectTypeID: domainsecret.SubjectModel,
		RoleID:        domainsecret.RoleManage,
	}).Return(nil)

	err := s.service.GrantSecretAccess(context.Background(), uri, SecretAccessParams{
		LeaderToken: successfulToken{},
		Accessor: SecretAccessor{
			Kind: ModelAccessor,
			ID:   "model-uuid",
		},
		Scope: SecretAccessScope{
			Kind: ModelAccessScope,
		},
		Subject: SecretAccessor{
			Kind: ModelAccessor,
		},
		Role: "manage",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestGrantSecretRelationScope(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectRunAtomic(ctrl, false)

	uri := coresecrets.NewURI()
	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "another/0",
	}).Return("manage", nil)
	s.state.EXPECT().GrantAccess(gomock.Any(), uri, domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeID:       "mysql:db mediawiki:db",
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
		RoleID:        domainsecret.RoleView,
	}).Return(nil)

	err := s.service.GrantSecretAccess(context.Background(), uri, SecretAccessParams{
		LeaderToken: successfulToken{},
		Accessor: SecretAccessor{
			Kind: UnitAccessor,
			ID:   "another/0",
		},
		Scope: SecretAccessScope{
			Kind: RelationAccessScope,
			ID:   "mysql:db mediawiki:db",
		},
		Subject: SecretAccessor{
			Kind: ApplicationAccessor,
			ID:   "mysql",
		},
		Role: "view",
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestRevokeSecretUnitAccess(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectRunAtomic(ctrl, false)

	uri := coresecrets.NewURI()
	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
	}).Return("manage", nil)
	s.state.EXPECT().RevokeAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "another/0",
	}).Return(nil)

	err := s.service.RevokeSecretAccess(context.Background(), uri, SecretAccessParams{
		LeaderToken: successfulToken{},
		Accessor: SecretAccessor{
			Kind: UnitAccessor,
			ID:   "mysql/0",
		},
		Subject: SecretAccessor{
			Kind: UnitAccessor,
			ID:   "another/0",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestRevokeSecretApplicationAccess(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectRunAtomic(ctrl, false)

	uri := coresecrets.NewURI()
	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
	}).Return("manage", nil)
	s.state.EXPECT().RevokeAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "another",
	}).Return(nil)

	err := s.service.RevokeSecretAccess(context.Background(), uri, SecretAccessParams{
		LeaderToken: successfulToken{},
		Accessor: SecretAccessor{
			Kind: UnitAccessor,
			ID:   "mysql/0",
		},
		Subject: SecretAccessor{
			Kind: ApplicationAccessor,
			ID:   "another",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestRevokeSecretModelAccess(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectRunAtomic(ctrl, false)

	uri := coresecrets.NewURI()
	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectModel,
		SubjectID:     "model-uuid",
	}).Return("manage", nil)
	s.state.EXPECT().RevokeAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
	}).Return(nil)

	err := s.service.RevokeSecretAccess(context.Background(), uri, SecretAccessParams{
		LeaderToken: successfulToken{},
		Accessor: SecretAccessor{
			Kind: ModelAccessor,
			ID:   "model-uuid",
		},
		Subject: SecretAccessor{
			Kind: ApplicationAccessor,
			ID:   "mysql",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestGetSecretAccess(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
	}).Return("manage", nil)

	role, err := s.service.getSecretAccess(NewMockAtomicContext(ctrl), uri, SecretAccessor{
		Kind: ApplicationAccessor,
		ID:   "mysql",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(role, gc.Equals, coresecrets.RoleManage)
}

func (s *serviceSuite) TestGetSecretAccessNone(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
	}).Return("", nil)

	role, err := s.service.getSecretAccess(NewMockAtomicContext(ctrl), uri, SecretAccessor{
		Kind: ApplicationAccessor,
		ID:   "mysql",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(role, gc.Equals, coresecrets.RoleNone)
}

func (s *serviceSuite) TestGetSecretAccessApplicationScope(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	s.state.EXPECT().GetSecretAccessScope(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
	}).Return(&domainsecret.AccessScope{
		ScopeTypeID: domainsecret.ScopeApplication,
		ScopeID:     "mysql",
	}, nil)

	scope, err := s.service.GetSecretAccessScope(context.Background(), uri, SecretAccessor{
		Kind: ApplicationAccessor,
		ID:   "mysql",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(scope, jc.DeepEquals, SecretAccessScope{
		Kind: ApplicationAccessScope,
		ID:   "mysql",
	})
}

func (s *serviceSuite) TestGetSecretAccessRelationScope(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	s.state.EXPECT().GetSecretAccessScope(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
	}).Return(&domainsecret.AccessScope{
		ScopeTypeID: domainsecret.ScopeRelation,
		ScopeID:     "mysql:db mediawiki:db",
	}, nil)

	scope, err := s.service.GetSecretAccessScope(context.Background(), uri, SecretAccessor{
		Kind: ApplicationAccessor,
		ID:   "mysql",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(scope, jc.DeepEquals, SecretAccessScope{
		Kind: RelationAccessScope,
		ID:   "mysql:db mediawiki:db",
	})
}

func (s *serviceSuite) TestGetSecretGrants(c *gc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	s.state.EXPECT().GetSecretGrants(gomock.Any(), uri, coresecrets.RoleView).Return([]domainsecret.GrantParams{{
		ScopeTypeID:   domainsecret.ScopeModel,
		ScopeID:       "model-uuid",
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
		RoleID:        domainsecret.RoleView,
	}}, nil)

	g, err := s.service.GetSecretGrants(context.Background(), uri, coresecrets.RoleView)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(g, jc.DeepEquals, []SecretAccess{{
		Scope: SecretAccessScope{
			Kind: ModelAccessScope,
			ID:   "model-uuid",
		},
		Subject: SecretAccessor{
			Kind: ApplicationAccessor,
			ID:   "mysql",
		},
		Role: coresecrets.RoleView,
	}})
}

func (s *serviceSuite) TestChangeSecretBackendToExternalBackend(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectRunAtomic(ctrl, false)

	uri := coresecrets.NewURI()
	ctx := context.Background()
	valueRef := &coresecrets.ValueRef{
		BackendID:  "backend-id",
		RevisionID: "rev-id",
	}

	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mariadb/0",
	}).Return("manage", nil)
	s.state.EXPECT().GetSecretRevisionID(gomock.Any(), uri, 1).Return(s.fakeUUID.String(), nil)
	s.state.EXPECT().ChangeSecretBackend(gomock.Any(), s.fakeUUID, valueRef, nil).Return(nil)
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelID.String(), nil)
	rollbackCalled := false
	s.secretBackendReferenceMutator.EXPECT().UpdateSecretBackendReference(gomock.Any(), valueRef, s.modelID, s.fakeUUID.String()).Return(func() error {
		rollbackCalled = true
		return nil
	}, nil)

	err := s.service.ChangeSecretBackend(ctx, uri, 1, ChangeSecretBackendParams{
		LeaderToken: successfulToken{},
		Accessor: SecretAccessor{
			Kind: UnitAccessor,
			ID:   "mariadb/0",
		},
		ValueRef: valueRef,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rollbackCalled, jc.IsFalse)
}

func (s *serviceSuite) TestChangeSecretBackendToInternalBackend(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectRunAtomic(ctrl, false)

	uri := coresecrets.NewURI()
	ctx := context.Background()

	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mariadb/0",
	}).Return("manage", nil)
	s.state.EXPECT().GetSecretRevisionID(gomock.Any(), uri, 1).Return(s.fakeUUID.String(), nil)
	s.state.EXPECT().ChangeSecretBackend(gomock.Any(), s.fakeUUID, nil, map[string]string{"foo": "bar"}).Return(nil)
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelID.String(), nil)
	rollbackCalled := false
	s.secretBackendReferenceMutator.EXPECT().UpdateSecretBackendReference(gomock.Any(), nil, s.modelID, s.fakeUUID.String()).Return(func() error {
		rollbackCalled = true
		return nil
	}, nil)

	err := s.service.ChangeSecretBackend(ctx, uri, 1, ChangeSecretBackendParams{
		LeaderToken: successfulToken{},
		Accessor: SecretAccessor{
			Kind: UnitAccessor,
			ID:   "mariadb/0",
		},
		Data: map[string]string{"foo": "bar"},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rollbackCalled, jc.IsFalse)
}

func (s *serviceSuite) TestChangeSecretBackendFailedAndRollback(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectRunAtomic(ctrl, false)

	uri := coresecrets.NewURI()
	ctx := context.Background()

	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mariadb/0",
	}).Return("manage", nil)
	s.state.EXPECT().GetSecretRevisionID(gomock.Any(), uri, 1).Return(s.fakeUUID.String(), nil)
	s.state.EXPECT().ChangeSecretBackend(gomock.Any(), s.fakeUUID, nil, map[string]string{"foo": "bar"}).Return(errors.New("boom"))
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelID.String(), nil)
	rollbackCalled := false
	s.secretBackendReferenceMutator.EXPECT().UpdateSecretBackendReference(gomock.Any(), nil, s.modelID, s.fakeUUID.String()).Return(func() error {
		rollbackCalled = true
		return nil
	}, nil)

	err := s.service.ChangeSecretBackend(ctx, uri, 1, ChangeSecretBackendParams{
		LeaderToken: successfulToken{},
		Accessor: SecretAccessor{
			Kind: UnitAccessor,
			ID:   "mariadb/0",
		},
		Data: map[string]string{"foo": "bar"},
	})
	c.Assert(err, gc.ErrorMatches, `boom`)
	c.Assert(rollbackCalled, jc.IsTrue)
}

func (s *serviceSuite) TestChangeSecretBackendFailedPermissionDenied(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectRunAtomic(ctrl, false)

	uri := coresecrets.NewURI()
	ctx := context.Background()

	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mariadb/0",
	}).Return("view", nil)
	s.state.EXPECT().GetSecretRevisionID(gomock.Any(), uri, 1).Return(s.fakeUUID.String(), nil)
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelID.String(), nil)
	rollbackCalled := false
	s.secretBackendReferenceMutator.EXPECT().UpdateSecretBackendReference(gomock.Any(), nil, s.modelID, s.fakeUUID.String()).Return(func() error {
		rollbackCalled = true
		return nil
	}, nil)

	err := s.service.ChangeSecretBackend(ctx, uri, 1, ChangeSecretBackendParams{
		Accessor: SecretAccessor{
			Kind: UnitAccessor,
			ID:   "mariadb/0",
		},
		Data: map[string]string{"foo": "bar"},
	})
	c.Assert(err, jc.ErrorIs, secreterrors.PermissionDenied)
	c.Assert(rollbackCalled, jc.IsTrue)
}

func (s *serviceSuite) TestChangeSecretBackendFailedSecretNotFound(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectRunAtomic(ctrl, false)

	uri := coresecrets.NewURI()
	ctx := context.Background()

	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mariadb/0",
	}).Return("manage", nil)
	s.state.EXPECT().GetSecretRevisionID(gomock.Any(), uri, 1).Return(s.fakeUUID.String(), nil)
	s.state.EXPECT().ChangeSecretBackend(gomock.Any(), s.fakeUUID, nil, map[string]string{"foo": "bar"}).Return(secreterrors.SecretNotFound)
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelID.String(), nil)
	rollbackCalled := false
	s.secretBackendReferenceMutator.EXPECT().UpdateSecretBackendReference(gomock.Any(), nil, s.modelID, s.fakeUUID.String()).Return(func() error {
		rollbackCalled = true
		return nil
	}, nil)

	err := s.service.ChangeSecretBackend(ctx, uri, 1, ChangeSecretBackendParams{
		LeaderToken: successfulToken{},
		Accessor: SecretAccessor{
			Kind: UnitAccessor,
			ID:   "mariadb/0",
		},
		Data: map[string]string{"foo": "bar"},
	})
	c.Assert(err, jc.ErrorIs, secreterrors.SecretNotFound)
	c.Assert(rollbackCalled, jc.IsTrue)
}

func (s *serviceSuite) TestSecretsRotated(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectRunAtomic(ctrl, false)

	uri := coresecrets.NewURI()
	ctx := context.Background()
	nextRotateTime := s.clock.Now().Add(time.Hour)

	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mariadb/0",
	}).Return("manage", nil)
	s.state.EXPECT().SecretRotated(gomock.Any(), uri, gomock.Any()).DoAndReturn(
		func(_ domain.AtomicContext, uri *coresecrets.URI, next time.Time) error {
			c.Assert(next, jc.Almost, nextRotateTime)
			return errors.New("boom")
		})
	s.state.EXPECT().GetRotationExpiryInfo(gomock.Any(), uri).Return(&domainsecret.RotationExpiryInfo{
		RotatePolicy:   coresecrets.RotateHourly,
		LatestRevision: 667,
	}, nil)

	err := s.service.SecretRotated(ctx, uri, SecretRotatedParams{
		LeaderToken: successfulToken{},
		Accessor: SecretAccessor{
			Kind: UnitAccessor,
			ID:   "mariadb/0",
		},
		OriginalRevision: 666,
	})
	c.Assert(err, gc.ErrorMatches, `boom`)
}

func (s *serviceSuite) TestSecretsRotatedRetry(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectRunAtomic(ctrl, false)

	uri := coresecrets.NewURI()
	ctx := context.Background()
	nextRotateTime := s.clock.Now().Add(coresecrets.RotateRetryDelay)

	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mariadb/0",
	}).Return("manage", nil)
	s.state.EXPECT().SecretRotated(gomock.Any(), uri, gomock.Any()).DoAndReturn(
		func(_ domain.AtomicContext, uri *coresecrets.URI, next time.Time) error {
			c.Assert(next, jc.Almost, nextRotateTime)
			return errors.New("boom")
		})
	s.state.EXPECT().GetRotationExpiryInfo(gomock.Any(), uri).Return(&domainsecret.RotationExpiryInfo{
		RotatePolicy:   coresecrets.RotateHourly,
		LatestRevision: 666,
	}, nil)

	err := s.service.SecretRotated(ctx, uri, SecretRotatedParams{
		LeaderToken: successfulToken{},
		Accessor: SecretAccessor{
			Kind: UnitAccessor,
			ID:   "mariadb/0",
		},
		OriginalRevision: 666,
	})
	c.Assert(err, gc.ErrorMatches, `boom`)
}

func (s *serviceSuite) TestSecretsRotatedForce(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectRunAtomic(ctrl, false)

	uri := coresecrets.NewURI()
	ctx := context.Background()
	nextRotateTime := s.clock.Now().Add(coresecrets.RotateRetryDelay)

	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mariadb/0",
	}).Return("manage", nil)
	s.state.EXPECT().SecretRotated(gomock.Any(), uri, gomock.Any()).DoAndReturn(
		func(_ domain.AtomicContext, uri *coresecrets.URI, next time.Time) error {
			c.Assert(next, jc.Almost, nextRotateTime)
			return errors.New("boom")
		})
	s.state.EXPECT().GetRotationExpiryInfo(gomock.Any(), uri).Return(&domainsecret.RotationExpiryInfo{
		RotatePolicy:     coresecrets.RotateHourly,
		LatestExpireTime: ptr(s.clock.Now().Add(50 * time.Minute)),
		LatestRevision:   667,
	}, nil)

	err := s.service.SecretRotated(ctx, uri, SecretRotatedParams{
		LeaderToken: successfulToken{},
		Accessor: SecretAccessor{
			Kind: UnitAccessor,
			ID:   "mariadb/0",
		},
		OriginalRevision: 666,
	})
	c.Assert(err, gc.ErrorMatches, `boom`)
}

func (s *serviceSuite) TestSecretsRotatedThenNever(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectRunAtomic(ctrl, false)

	uri := coresecrets.NewURI()
	ctx := context.Background()

	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mariadb/0",
	}).Return("manage", nil)
	s.state.EXPECT().GetRotationExpiryInfo(gomock.Any(), uri).Return(&domainsecret.RotationExpiryInfo{
		RotatePolicy:   coresecrets.RotateNever,
		LatestRevision: 667,
	}, nil)

	err := s.service.SecretRotated(ctx, uri, SecretRotatedParams{
		LeaderToken: successfulToken{},
		Accessor: SecretAccessor{
			Kind: UnitAccessor,
			ID:   "mariadb/0",
		},
		OriginalRevision: 666,
	})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *serviceSuite) TestGetConsumedRevisionFirstTime(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectRunAtomic(ctrl, false)

	uri := coresecrets.NewURI()

	s.state.EXPECT().GetSecretConsumer(gomock.Any(), uri, "mariadb/0").Return(nil, 666, secreterrors.SecretConsumerNotFound)
	s.state.EXPECT().SaveSecretConsumer(gomock.Any(), uri, "mariadb/0", &coresecrets.SecretConsumerMetadata{
		CurrentRevision: 666,
	})

	rev, err := s.service.GetConsumedRevision(context.Background(), uri, "mariadb/0", false, false, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rev, gc.Equals, 666)
}

func (s *serviceSuite) TestGetConsumedRevisionFirstTimeUpdateLabel(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectRunAtomic(ctrl, false)

	uri := coresecrets.NewURI()

	s.state.EXPECT().GetSecretConsumer(gomock.Any(), uri, "mariadb/0").Return(nil, 666, secreterrors.SecretConsumerNotFound)
	s.state.EXPECT().SaveSecretConsumer(gomock.Any(), uri, "mariadb/0", &coresecrets.SecretConsumerMetadata{
		Label:           "label",
		CurrentRevision: 666,
	})

	rev, err := s.service.GetConsumedRevision(context.Background(), uri, "mariadb/0", false, false, ptr("label"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rev, gc.Equals, 666)
}

func (s *serviceSuite) TestGetSecretConsumedRevisionUpdateLabel(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectRunAtomic(ctrl, false)

	uri := coresecrets.NewURI()

	s.state.EXPECT().GetSecretConsumer(gomock.Any(), uri, "mariadb/0").Return(&coresecrets.SecretConsumerMetadata{
		Label:           "old-label",
		CurrentRevision: 666,
	}, 666, nil)
	s.state.EXPECT().SaveSecretConsumer(gomock.Any(), uri, "mariadb/0", &coresecrets.SecretConsumerMetadata{
		Label:           "new-label",
		CurrentRevision: 666,
	})

	rev, err := s.service.GetConsumedRevision(context.Background(), uri, "mariadb/0", false, false, ptr("new-label"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rev, gc.Equals, 666)
}

func (s *serviceSuite) TestGetSecretConsumedRevisionRefresh(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectRunAtomic(ctrl, false)

	uri := coresecrets.NewURI()

	s.state.EXPECT().GetSecretConsumer(gomock.Any(), uri, "mariadb/0").Return(&coresecrets.SecretConsumerMetadata{
		Label:           "old-label",
		CurrentRevision: 666,
	}, 668, nil)
	s.state.EXPECT().SaveSecretConsumer(gomock.Any(), uri, "mariadb/0", &coresecrets.SecretConsumerMetadata{
		Label:           "old-label",
		CurrentRevision: 668,
	})

	rev, err := s.service.GetConsumedRevision(context.Background(), uri, "mariadb/0", true, false, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rev, gc.Equals, 668)
}

func (s *serviceSuite) TestGetSecretConsumedRevisionPeek(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectRunAtomic(ctrl, false)

	uri := coresecrets.NewURI()

	s.state.EXPECT().GetSecretConsumer(gomock.Any(), uri, "mariadb/0").Return(&coresecrets.SecretConsumerMetadata{
		Label:           "old-label",
		CurrentRevision: 666,
	}, 668, nil)

	rev, err := s.service.GetConsumedRevision(context.Background(), uri, "mariadb/0", false, true, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rev, gc.Equals, 668)
}

func (s *serviceSuite) TestGetSecretConsumedRevisionSecretNotFound(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectRunAtomic(ctrl, false)

	uri := coresecrets.NewURI()

	s.state.EXPECT().GetSecretConsumer(gomock.Any(), uri, "mariadb/0").Return(&coresecrets.SecretConsumerMetadata{
		CurrentRevision: 666,
	}, 668, nil)
	s.state.EXPECT().SaveSecretConsumer(gomock.Any(), uri, "mariadb/0", &coresecrets.SecretConsumerMetadata{
		CurrentRevision: 668,
	})

	uri2 := coresecrets.NewURI()
	md := []*domainsecret.SecretMetadata{{
		URI:   uri2,
		Label: "foz",
	}}

	s.state.EXPECT().ListCharmSecrets(gomock.Any(), domainsecret.ApplicationOwners{"mariadb"}, domainsecret.UnitOwners{"mariadb/0"}).
		Return(md, nil)

	rev, err := s.service.GetConsumedRevision(context.Background(), uri, "mariadb/0", true, false, nil)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(rev, gc.Equals, 668)
}

func (s *serviceSuite) TestProcessCharmSecretConsumerLabelForUnitOwnedSecretUpdateLabel(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectRunAtomic(ctrl, true)

	uri := coresecrets.NewURI()
	md := []*domainsecret.SecretMetadata{{
		URI:   uri,
		Label: "foz",
	}}

	s.state.EXPECT().ListCharmSecrets(gomock.Any(), domainsecret.ApplicationOwners{"mariadb"}, domainsecret.UnitOwners{"mariadb/0"}).
		Return(md, nil)
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(coretesting.ModelTag.Id(), nil)
	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectID:     "mariadb/0",
		SubjectTypeID: domainsecret.SubjectUnit,
	}).Return("manage", nil)
	s.state.EXPECT().UpdateSecret(gomock.Any(), uri, domainsecret.UpsertSecretParams{
		RotatePolicy: ptr(domainsecret.RotateNever),
		Label:        ptr("foo"),
	}).Return(nil)

	gotURI, gotLabel, err := s.service.ProcessCharmSecretConsumerLabel(context.Background(), "mariadb/0", uri, "foo", successfulToken{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotURI, jc.DeepEquals, uri)
	c.Assert(gotLabel, gc.IsNil)
}

func (s *serviceSuite) TestProcessCharmSecretConsumerLabelForUnitOwnedSecretLookupURI(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectRunAtomic(ctrl, false)

	uri := coresecrets.NewURI()
	md := []*domainsecret.SecretMetadata{{
		URI:   uri,
		Label: "foo",
	}}

	s.state.EXPECT().ListCharmSecrets(gomock.Any(), domainsecret.ApplicationOwners{"mariadb"}, domainsecret.UnitOwners{"mariadb/0"}).
		Return(md, nil)
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(coretesting.ModelTag.Id(), nil)

	gotURI, gotLabel, err := s.service.ProcessCharmSecretConsumerLabel(context.Background(), "mariadb/0", nil, "foo", successfulToken{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotURI, jc.DeepEquals, uri)
	c.Assert(gotLabel, gc.IsNil)
}

func (s *serviceSuite) TestProcessCharmSecretConsumerLabelLookupURI(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectRunAtomic(ctrl, false)

	uri := coresecrets.NewURI()
	md := []*domainsecret.SecretMetadata{{
		URI:   uri,
		Label: "foz",
	}}

	s.state.EXPECT().ListCharmSecrets(gomock.Any(), domainsecret.ApplicationOwners{"mariadb"}, domainsecret.UnitOwners{"mariadb/0"}).
		Return(md, nil)
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(coretesting.ModelTag.Id(), nil)
	s.state.EXPECT().GetURIByConsumerLabel(gomock.Any(), "foo", "mariadb/0").Return(uri, nil)

	gotURI, gotLabel, err := s.service.ProcessCharmSecretConsumerLabel(context.Background(), "mariadb/0", nil, "foo", successfulToken{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotURI, jc.DeepEquals, uri)
	c.Assert(gotLabel, gc.IsNil)
}

func (s *serviceSuite) TestProcessCharmSecretConsumerLabelUpdateLabel(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	s.expectRunAtomic(ctrl, false)

	uri := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	md := []*domainsecret.SecretMetadata{{
		URI:   uri2,
		Label: "foz",
	}}

	s.state.EXPECT().ListCharmSecrets(gomock.Any(), domainsecret.ApplicationOwners{"mariadb"}, domainsecret.UnitOwners{"mariadb/0"}).
		Return(md, nil)
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(coretesting.ModelTag.Id(), nil)

	gotURI, gotLabel, err := s.service.ProcessCharmSecretConsumerLabel(context.Background(), "mariadb/0", uri, "foo", successfulToken{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(gotURI, jc.DeepEquals, uri)
	c.Assert(gotLabel, gc.DeepEquals, ptr("foo"))
}

func (s *serviceSuite) TestWatchObsolete(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	mockWatcherFactory := NewMockWatcherFactory(ctrl)

	ch := make(chan []string)
	mockStringWatcher := NewMockStringsWatcher(ctrl)
	mockStringWatcher.EXPECT().Changes().Return(ch).AnyTimes()
	mockStringWatcher.EXPECT().Wait().Return(nil).AnyTimes()
	mockStringWatcher.EXPECT().Kill().AnyTimes()

	var namespaceQuery eventsource.NamespaceQuery = func(context.Context, database.TxnRunner) ([]string, error) {
		return []string{"revision-uuid-1", "revision-uuid-2"}, nil
	}
	s.state.EXPECT().InitialWatchStatementForObsoleteRevision(
		domainsecret.ApplicationOwners([]string{"mysql"}),
		domainsecret.UnitOwners([]string{"mysql/0", "mysql/1"}),
	).Return("table", namespaceQuery)
	mockWatcherFactory.EXPECT().NewNamespaceWatcher("table", changestream.Create, gomock.Any()).Return(mockStringWatcher, nil)

	s.state.EXPECT().GetRevisionIDsForObsolete(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners, revisionUUIDs ...string) ([]string, error) {
		c.Assert(appOwners, jc.SameContents, domainsecret.ApplicationOwners([]string{"mysql"}))
		c.Assert(unitOwners, jc.SameContents, domainsecret.UnitOwners([]string{"mysql/0", "mysql/1"}))
		c.Assert(revisionUUIDs, jc.SameContents, []string{"revision-uuid-1", "revision-uuid-2"})
		return []string{"yyy/1", "yyy/2"}, nil
	})

	svc := NewWatchableService(s.state, s.secretBackendReferenceMutator, loggertesting.WrapCheckLog(c), mockWatcherFactory, SecretServiceParams{})
	w, err := svc.WatchObsolete(context.Background(),
		CharmSecretOwner{
			Kind: ApplicationOwner,
			ID:   "mysql",
		},
		CharmSecretOwner{
			Kind: UnitOwner,
			ID:   "mysql/0",
		},
		CharmSecretOwner{
			Kind: UnitOwner,
			ID:   "mysql/1",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
	defer workertest.CleanKill(c, w)
	wC := watchertest.NewStringsWatcherC(c, w)

	select {
	case ch <- []string{"revision-uuid-1", "revision-uuid-2"}:
	case <-time.After(coretesting.ShortWait):
		c.Fatalf("timed out waiting for the initial changes")
	}

	wC.AssertChange(
		"yyy/1",
		"yyy/2",
	)
	wC.AssertNoChange()
}

func (s *serviceSuite) TestWatchObsoleteUserSecretsToPrune(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	mockWatcherFactory := NewMockWatcherFactory(ctrl)

	ch1 := make(chan struct{})
	ch2 := make(chan struct{})

	go func() {
		// send the initial change.
		ch1 <- struct{}{}
		ch2 <- struct{}{}
	}()

	mockObsoleteWatcher := NewMockNotifyWatcher(ctrl)
	mockObsoleteWatcher.EXPECT().Changes().Return(ch1).AnyTimes()
	mockObsoleteWatcher.EXPECT().Wait().Return(nil).AnyTimes()
	mockObsoleteWatcher.EXPECT().Kill().AnyTimes()

	mockAutoPruneWatcher := NewMockNotifyWatcher(ctrl)
	mockAutoPruneWatcher.EXPECT().Changes().Return(ch2).AnyTimes()
	mockAutoPruneWatcher.EXPECT().Wait().Return(nil).AnyTimes()
	mockAutoPruneWatcher.EXPECT().Kill().AnyTimes()

	mockWatcherFactory.EXPECT().NewNamespaceNotifyMapperWatcher("secret_revision_obsolete", changestream.Create, gomock.Any()).Return(mockObsoleteWatcher, nil)
	mockWatcherFactory.EXPECT().NewNamespaceNotifyMapperWatcher("secret_metadata", changestream.Update, gomock.Any()).Return(mockAutoPruneWatcher, nil)

	svc := NewWatchableService(s.state, s.secretBackendReferenceMutator, loggertesting.WrapCheckLog(c), mockWatcherFactory, SecretServiceParams{})
	w, err := svc.WatchObsoleteUserSecretsToPrune(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
	defer workertest.CleanKill(c, w)
	wc := watchertest.NewNotifyWatcherC(c, w)
	// initial change.
	wc.AssertOneChange()

	select {
	case ch1 <- struct{}{}:
	case <-time.After(coretesting.ShortWait):
		c.Fatalf("timed out waiting for sending the secret revision changes")
	}
	wc.AssertOneChange()
	select {
	case ch2 <- struct{}{}:
	case <-time.After(coretesting.ShortWait):
		c.Fatalf("timed out waiting for sending the secret URI changes")
	}
	wc.AssertOneChange()
}

func (s *serviceSuite) TestWatchConsumedSecretsChanges(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	mockWatcherFactory := NewMockWatcherFactory(ctrl)

	uri1 := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()

	ch := make(chan []string)
	mockStringWatcher := NewMockStringsWatcher(ctrl)
	mockStringWatcher.EXPECT().Changes().Return(ch).AnyTimes()
	mockStringWatcher.EXPECT().Wait().Return(nil).AnyTimes()
	mockStringWatcher.EXPECT().Kill().AnyTimes()

	chRemote := make(chan []string)
	mockStringWatcherRemote := NewMockStringsWatcher(ctrl)
	mockStringWatcherRemote.EXPECT().Changes().Return(chRemote).AnyTimes()
	mockStringWatcherRemote.EXPECT().Wait().Return(nil).AnyTimes()
	mockStringWatcherRemote.EXPECT().Kill().AnyTimes()

	var namespaceQuery eventsource.NamespaceQuery = func(context.Context, database.TxnRunner) ([]string, error) {
		return nil, nil
	}
	s.state.EXPECT().InitialWatchStatementForConsumedSecretsChange("mysql/0").Return("secret_revision", namespaceQuery)
	s.state.EXPECT().InitialWatchStatementForConsumedRemoteSecretsChange("mysql/0").Return("secret_reference", namespaceQuery)
	mockWatcherFactory.EXPECT().NewNamespaceWatcher("secret_revision", changestream.Create, gomock.Any()).Return(mockStringWatcher, nil)
	mockWatcherFactory.EXPECT().NewNamespaceWatcher("secret_reference", changestream.All, gomock.Any()).Return(mockStringWatcherRemote, nil)

	s.state.EXPECT().GetConsumedSecretURIsWithChanges(gomock.Any(),
		"mysql/0", "revision-uuid-1",
	).Return([]string{uri1.String()}, nil)
	s.state.EXPECT().GetConsumedRemoteSecretURIsWithChanges(gomock.Any(),
		"mysql/0", "revision-uuid-2",
	).Return([]string{uri2.String()}, nil)

	svc := NewWatchableService(s.state, s.secretBackendReferenceMutator, loggertesting.WrapCheckLog(c), mockWatcherFactory, SecretServiceParams{})
	w, err := svc.WatchConsumedSecretsChanges(context.Background(), "mysql/0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
	defer workertest.CleanKill(c, w)
	wc := watchertest.NewStringsWatcherC(c, w)

	select {
	case ch <- []string{"revision-uuid-1"}:
	case <-time.After(coretesting.ShortWait):
		c.Fatalf("timed out waiting for the initial changes")
	}
	select {
	case chRemote <- []string{"revision-uuid-2"}:
	case <-time.After(coretesting.ShortWait):
		c.Fatalf("timed out waiting for the initial changes")
	}

	wc.AssertChange(
		uri1.String(),
		uri2.String(),
	)
	wc.AssertNoChange()
}

func (s *serviceSuite) TestWatchRemoteConsumedSecretsChanges(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	mockWatcherFactory := NewMockWatcherFactory(ctrl)

	uri1 := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()

	ch := make(chan []string)
	mockStringWatcher := NewMockStringsWatcher(ctrl)
	mockStringWatcher.EXPECT().Changes().Return(ch).AnyTimes()
	mockStringWatcher.EXPECT().Wait().Return(nil).AnyTimes()
	mockStringWatcher.EXPECT().Kill().AnyTimes()

	var namespaceQuery eventsource.NamespaceQuery = func(context.Context, database.TxnRunner) ([]string, error) {
		return nil, nil
	}
	s.state.EXPECT().InitialWatchStatementForRemoteConsumedSecretsChangesFromOfferingSide("mysql").Return("secret_revision", namespaceQuery)
	mockWatcherFactory.EXPECT().NewNamespaceWatcher("secret_revision", changestream.All, gomock.Any()).Return(mockStringWatcher, nil)

	s.state.EXPECT().GetRemoteConsumedSecretURIsWithChangesFromOfferingSide(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, appName string, secretIDs ...string) ([]string, error) {
		c.Assert(appName, gc.Equals, "mysql")
		c.Assert(secretIDs, jc.SameContents, []string{"revision-uuid-1", "revision-uuid-2"})
		return []string{uri1.String(), uri2.String()}, nil
	})

	svc := NewWatchableService(s.state, s.secretBackendReferenceMutator, loggertesting.WrapCheckLog(c), mockWatcherFactory, SecretServiceParams{})
	w, err := svc.WatchRemoteConsumedSecretsChanges(context.Background(), "mysql")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
	defer workertest.CleanKill(c, w)
	wC := watchertest.NewStringsWatcherC(c, w)

	select {
	case ch <- []string{"revision-uuid-1", "revision-uuid-2"}:
	case <-time.After(coretesting.ShortWait):
		c.Fatalf("timed out waiting for the initial changes")
	}

	wC.AssertChange(
		uri1.String(),
		uri2.String(),
	)
	wC.AssertNoChange()
}

func (s *serviceSuite) TestWatchSecretsRotationChanges(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	mockWatcherFactory := NewMockWatcherFactory(ctrl)

	uri1 := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()

	ch := make(chan []string)
	mockStringWatcher := NewMockStringsWatcher(ctrl)
	mockStringWatcher.EXPECT().Changes().Return(ch).AnyTimes()
	mockStringWatcher.EXPECT().Wait().Return(nil).AnyTimes()
	mockStringWatcher.EXPECT().Kill().AnyTimes()

	var namespaceQuery eventsource.NamespaceQuery = func(context.Context, database.TxnRunner) ([]string, error) {
		return nil, nil
	}
	s.state.EXPECT().InitialWatchStatementForSecretsRotationChanges(
		domainsecret.ApplicationOwners{"mediawiki"}, domainsecret.UnitOwners{"mysql/0", "mysql/1"},
	).Return("secret_rotation", namespaceQuery)
	mockWatcherFactory.EXPECT().NewNamespaceWatcher("secret_rotation", changestream.All, gomock.Any()).Return(mockStringWatcher, nil)

	now := s.clock.Now()
	s.state.EXPECT().GetSecretsRotationChanges(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners, secretIDs ...string) ([]domainsecret.RotationInfo, error) {
		c.Assert(appOwners, jc.SameContents, domainsecret.ApplicationOwners{"mediawiki"})
		c.Assert(unitOwners, jc.SameContents, domainsecret.UnitOwners{"mysql/0", "mysql/1"})
		c.Assert(secretIDs, jc.SameContents, []string{uri1.ID, uri2.ID})
		return []domainsecret.RotationInfo{
			{
				URI:             uri1,
				Revision:        1,
				NextTriggerTime: now,
			},
			{
				URI:             uri2,
				Revision:        2,
				NextTriggerTime: now.Add(2 * time.Hour),
			},
		}, nil
	})
	svc := NewWatchableService(s.state, s.secretBackendReferenceMutator, loggertesting.WrapCheckLog(c), mockWatcherFactory, SecretServiceParams{})
	w, err := svc.WatchSecretsRotationChanges(context.Background(),
		CharmSecretOwner{
			Kind: ApplicationOwner,
			ID:   "mediawiki",
		},
		CharmSecretOwner{
			Kind: UnitOwner,
			ID:   "mysql/0",
		},
		CharmSecretOwner{
			Kind: UnitOwner,
			ID:   "mysql/1",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
	defer workertest.CleanKill(c, w)
	wC := watchertest.NewSecretsTriggerWatcherC(c, w)

	select {
	case ch <- []string{uri1.ID, uri2.ID}:
	case <-time.After(coretesting.ShortWait):
		c.Fatalf("timed out waiting for the initial changes")
	}

	wC.AssertChange(
		watcher.SecretTriggerChange{
			URI:             uri1,
			Revision:        1,
			NextTriggerTime: now,
		},
		watcher.SecretTriggerChange{
			URI:             uri2,
			Revision:        2,
			NextTriggerTime: now.Add(2 * time.Hour),
		},
	)
	wC.AssertNoChange()
}

func (s *serviceSuite) TestWatchSecretRevisionsExpiryChanges(c *gc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	mockWatcherFactory := NewMockWatcherFactory(ctrl)

	uri1 := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()

	ch := make(chan []string)
	mockStringWatcher := NewMockStringsWatcher(ctrl)
	mockStringWatcher.EXPECT().Changes().Return(ch).AnyTimes()
	mockStringWatcher.EXPECT().Wait().Return(nil).AnyTimes()
	mockStringWatcher.EXPECT().Kill().AnyTimes()

	var namespaceQuery eventsource.NamespaceQuery = func(context.Context, database.TxnRunner) ([]string, error) {
		return nil, nil
	}
	s.state.EXPECT().InitialWatchStatementForSecretsRevisionExpiryChanges(
		domainsecret.ApplicationOwners{"mediawiki"}, domainsecret.UnitOwners{"mysql/0", "mysql/1"},
	).Return("secret_revision_expire", namespaceQuery)
	mockWatcherFactory.EXPECT().NewNamespaceWatcher("secret_revision_expire", changestream.All, gomock.Any()).Return(mockStringWatcher, nil)

	now := s.clock.Now()
	s.state.EXPECT().GetSecretsRevisionExpiryChanges(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners, revisionUUIDs ...string) ([]domainsecret.ExpiryInfo, error) {
		c.Assert(appOwners, jc.SameContents, domainsecret.ApplicationOwners{"mediawiki"})
		c.Assert(unitOwners, jc.SameContents, domainsecret.UnitOwners{"mysql/0", "mysql/1"})
		c.Assert(revisionUUIDs, jc.SameContents, []string{"revision-uuid-1", "revision-uuid-2"})
		return []domainsecret.ExpiryInfo{
			{
				URI:             uri1,
				Revision:        1,
				NextTriggerTime: now,
			},
			{
				URI:             uri2,
				Revision:        2,
				NextTriggerTime: now.Add(2 * time.Hour),
			},
		}, nil
	})
	svc := NewWatchableService(s.state, s.secretBackendReferenceMutator, loggertesting.WrapCheckLog(c), mockWatcherFactory, SecretServiceParams{})
	w, err := svc.WatchSecretRevisionsExpiryChanges(context.Background(),
		CharmSecretOwner{
			Kind: ApplicationOwner,
			ID:   "mediawiki",
		},
		CharmSecretOwner{
			Kind: UnitOwner,
			ID:   "mysql/0",
		},
		CharmSecretOwner{
			Kind: UnitOwner,
			ID:   "mysql/1",
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.NotNil)
	defer workertest.CleanKill(c, w)
	wC := watchertest.NewSecretsTriggerWatcherC(c, w)

	select {
	case ch <- []string{"revision-uuid-1", "revision-uuid-2"}:
	case <-time.After(coretesting.ShortWait):
		c.Fatalf("timed out waiting for the initial changes")
	}

	wC.AssertChange(
		watcher.SecretTriggerChange{
			URI:             uri1,
			Revision:        1,
			NextTriggerTime: now,
		},
		watcher.SecretTriggerChange{
			URI:             uri2,
			Revision:        2,
			NextTriggerTime: now.Add(2 * time.Hour),
		},
	)
	wC.AssertNoChange()
}
