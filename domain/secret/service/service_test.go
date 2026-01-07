// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/relation"
	relationtesting "github.com/juju/juju/core/relation/testing"
	coresecrets "github.com/juju/juju/core/secrets"
	coreunit "github.com/juju/juju/core/unit"
	unittesting "github.com/juju/juju/core/unit/testing"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/core/watcher/watchertest"
	"github.com/juju/juju/domain"
	domainsecret "github.com/juju/juju/domain/secret"
	secreterrors "github.com/juju/juju/domain/secret/errors"
	domaintesting "github.com/juju/juju/domain/testing"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/internal/secrets/provider/juju"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/uuid"
)

type serviceSuite struct {
	clock                  *testclock.Clock
	modelID                coremodel.UUID
	secretsBackend         *MockSecretsBackend
	secretsBackendProvider *MockSecretBackendProvider
	ensurer                *MockEnsurer

	state              *MockState
	secretBackendState *MockSecretBackendState

	service  *SecretService
	fakeUUID uuid.UUID
}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) SetUpTest(c *tc.C) {
	s.modelID = tc.Must0(c, coremodel.NewUUID)
	var err error
	s.fakeUUID, err = uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
	s.clock = testclock.NewClock(time.Time{})
}

func (s *serviceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	s.secretBackendState = NewMockSecretBackendState(ctrl)
	s.secretsBackendProvider = NewMockSecretBackendProvider(ctrl)
	s.secretsBackend = NewMockSecretsBackend(ctrl)
	s.ensurer = NewMockEnsurer(ctrl)

	s.state.EXPECT().RunAtomic(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, fn func(ctx domain.AtomicContext) error) error {
		return fn(domaintesting.NewAtomicContext(ctx))
	}).AnyTimes()

	s.service = &SecretService{
		secretState:        s.state,
		secretBackendState: s.secretBackendState,
		providerGetter:     func(string) (provider.SecretBackendProvider, error) { return s.secretsBackendProvider, nil },
		leaderEnsurer:      s.ensurer,
		uuidGenerator:      func() (uuid.UUID, error) { return s.fakeUUID, nil },
		clock:              s.clock,
		logger:             loggertesting.WrapCheckLog(c),
	}
	return ctrl
}

func (s *serviceSuite) TestCreateUserSecretURIs(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(coremodel.UUID(coretesting.ModelTag.Id()), nil)

	got, err := s.service.CreateSecretURIs(c.Context(), 2)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.HasLen, 2)
	c.Assert(got[0].SourceUUID, tc.Equals, coretesting.ModelTag.Id())
	c.Assert(got[1].SourceUUID, tc.Equals, coretesting.ModelTag.Id())
}

func (s *serviceSuite) TestCreateUserSecretInternal(c *tc.C) {
	s.assertCreateUserSecret(c, true, false, false)
}
func (s *serviceSuite) TestCreateUserSecretExternalBackend(c *tc.C) {
	s.assertCreateUserSecret(c, false, false, false)
}

func (s *serviceSuite) TestCreateUserSecretExternalBackendFailedWithCleanup(c *tc.C) {
	s.assertCreateUserSecret(c, false, true, false)
}

func (s *serviceSuite) TestCreateUserSecretFailedLabelExistsWithCleanup(c *tc.C) {
	s.assertCreateUserSecret(c, false, true, true)
}

func (s *serviceSuite) assertCreateUserSecret(c *tc.C, isInternal, finalStepFailed, labelExists bool) {
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

	s.secretBackendState.EXPECT().GetActiveModelSecretBackend(gomock.Any(), s.modelID).Return(
		"backend-id",
		&provider.ModelBackendConfig{
			ControllerUUID: coretesting.ControllerTag.Id(),
			ModelUUID:      s.modelID.String(),
			ModelName:      "some-model",
			BackendConfig: provider.BackendConfig{
				BackendType: "active-type",
				Config:      map[string]any{"foo": "active-type"},
			},
		}, nil,
	)

	mBackendConfig := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      s.modelID.String(),
		ModelName:      "some-model",
		BackendConfig: provider.BackendConfig{
			BackendType: "active-type",
			Config:      map[string]any{"foo": "active-type"},
		},
	}
	s.secretsBackendProvider.EXPECT().Initialise(mBackendConfig).Return(nil)
	existingOwnedURI := coresecrets.NewURI()
	s.state.EXPECT().ListGrantedSecretsForBackend(gomock.Any(), "backend-id", []domainsecret.AccessParams{
		{
			SubjectID:     s.modelID.String(),
			SubjectTypeID: domainsecret.SubjectModel,
		},
	}, coresecrets.RoleManage).Return(
		[]*coresecrets.SecretRevisionRef{
			{
				URI:        existingOwnedURI,
				RevisionID: "rev-id",
			},
		}, nil,
	)
	ownedRevisions := provider.SecretRevisions{}
	ownedRevisions.Add(existingOwnedURI, "rev-id")
	s.secretsBackendProvider.EXPECT().RestrictedConfig(gomock.Any(), mBackendConfig, true, false, coresecrets.Accessor{
		Kind: coresecrets.ModelAccessor,
		ID:   s.modelID.String(),
	}, ownedRevisions, provider.SecretRevisions{}).Return(
		&mBackendConfig.BackendConfig, nil,
	)
	s.secretsBackendProvider.EXPECT().NewBackend(
		&provider.ModelBackendConfig{
			ControllerUUID: coretesting.ControllerTag.Id(),
			ModelUUID:      s.modelID.String(),
			ModelName:      "some-model",
			BackendConfig:  mBackendConfig.BackendConfig,
		},
	).DoAndReturn(
		func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
			return s.secretsBackend, nil
		},
	)

	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelID, nil).AnyTimes()
	uri := coresecrets.NewURI()
	rollbackCalled := false
	s.secretBackendState.EXPECT().AddSecretBackendReference(gomock.Any(), params.ValueRef, s.modelID, s.fakeUUID.String()).Return(
		func() error {
			rollbackCalled = true
			return nil
		}, nil,
	)
	if isInternal {
		s.secretsBackend.EXPECT().SaveContent(gomock.Any(), uri, 1, coresecrets.NewSecretValue(map[string]string{"foo": "bar"})).
			Return("", errors.Errorf("not supported %w", coreerrors.NotSupported))

	} else {
		s.secretsBackend.EXPECT().SaveContent(gomock.Any(), uri, 1, coresecrets.NewSecretValue(map[string]string{"foo": "bar"})).
			Return("rev-id", nil)
	}
	if (finalStepFailed || labelExists) && !isInternal {
		s.secretsBackend.EXPECT().DeleteContent(gomock.Any(), "rev-id").Return(nil)
	}

	s.state.EXPECT().CheckUserSecretLabelExists(domaintesting.IsAtomicContextChecker, "my secret").Return(labelExists, nil)
	if !labelExists {
		s.state.EXPECT().CreateUserSecret(gomock.Any(), 1, uri, params).
			DoAndReturn(func(domain.AtomicContext, int, *coresecrets.URI, domainsecret.UpsertSecretParams) error {
				if finalStepFailed {
					return errors.New("some error")
				}
				return nil
			})
	}

	err := s.service.CreateUserSecret(c.Context(), uri, CreateUserSecretParams{
		UpdateUserSecretParams: UpdateUserSecretParams{
			Accessor: domainsecret.SecretAccessor{
				Kind: domainsecret.ModelAccessor,
				ID:   s.modelID.String(),
			},
			Description: ptr("a secret"),
			Label:       ptr("my secret"),
			Data:        map[string]string{"foo": "bar"},
			AutoPrune:   ptr(true),
			Checksum:    "checksum-1234",
		},
		Version: 1,
	})
	if finalStepFailed || labelExists {
		c.Assert(rollbackCalled, tc.IsTrue)
		if labelExists {
			c.Assert(err, tc.ErrorIs, secreterrors.SecretLabelAlreadyExists)
		} else {
			c.Assert(err, tc.ErrorMatches, "creating user secret .*some error")
		}
	} else {
		c.Assert(err, tc.ErrorIsNil)
	}
}

func (s *serviceSuite) TestUpdateUserSecretInternal(c *tc.C) {
	s.assertUpdateUserSecret(c, true, false, false)
}
func (s *serviceSuite) TestUpdateUserSecretExternalBackend(c *tc.C) {
	s.assertUpdateUserSecret(c, false, false, false)
}

func (s *serviceSuite) TestUpdateUserSecretExternalBackendFailedWithCleanup(c *tc.C) {
	s.assertUpdateUserSecret(c, false, true, false)
}

func (s *serviceSuite) TestUpdateUserSecretFailedLabelExistsWithCleanup(c *tc.C) {
	s.assertUpdateUserSecret(c, false, true, true)
}

func (s *serviceSuite) assertUpdateUserSecret(c *tc.C, isInternal, finalStepFailed, labelExists bool) {
	defer s.setupMocks(c).Finish()

	s.secretBackendState.EXPECT().GetActiveModelSecretBackend(gomock.Any(), s.modelID).Return(
		"backend-id",
		&provider.ModelBackendConfig{
			ControllerUUID: coretesting.ControllerTag.Id(),
			ModelUUID:      s.modelID.String(),
			ModelName:      "some-model",
			BackendConfig: provider.BackendConfig{
				BackendType: "active-type",
				Config:      map[string]any{"foo": "active-type"},
			},
		}, nil,
	)

	mBackendConfig := &provider.ModelBackendConfig{
		ControllerUUID: coretesting.ControllerTag.Id(),
		ModelUUID:      s.modelID.String(),
		ModelName:      "some-model",
		BackendConfig: provider.BackendConfig{
			BackendType: "active-type",
			Config:      map[string]any{"foo": "active-type"},
		},
	}
	s.secretsBackendProvider.EXPECT().Initialise(mBackendConfig).Return(nil)
	existingOwnedURI := coresecrets.NewURI()
	s.state.EXPECT().ListGrantedSecretsForBackend(gomock.Any(), "backend-id", []domainsecret.AccessParams{
		{
			SubjectID:     s.modelID.String(),
			SubjectTypeID: domainsecret.SubjectModel,
		},
	}, coresecrets.RoleManage).Return(
		[]*coresecrets.SecretRevisionRef{
			{
				URI:        existingOwnedURI,
				RevisionID: "rev-id",
			},
		}, nil,
	)
	ownedRevisions := provider.SecretRevisions{}
	ownedRevisions.Add(existingOwnedURI, "rev-id")
	s.secretsBackendProvider.EXPECT().RestrictedConfig(gomock.Any(), mBackendConfig, true, false, coresecrets.Accessor{
		Kind: coresecrets.ModelAccessor,
		ID:   s.modelID.String(),
	}, ownedRevisions, provider.SecretRevisions{}).Return(
		&mBackendConfig.BackendConfig, nil,
	)
	s.secretsBackendProvider.EXPECT().NewBackend(
		&provider.ModelBackendConfig{
			ControllerUUID: coretesting.ControllerTag.Id(),
			ModelUUID:      s.modelID.String(),
			ModelName:      "some-model",
			BackendConfig:  mBackendConfig.BackendConfig,
		},
	).DoAndReturn(
		func(cfg *provider.ModelBackendConfig) (provider.SecretsBackend, error) {
			return s.secretsBackend, nil
		},
	)

	uri := coresecrets.NewURI()
	if isInternal {
		s.secretsBackend.EXPECT().SaveContent(gomock.Any(), uri, 3, coresecrets.NewSecretValue(map[string]string{"foo": "bar"})).
			Return("", errors.Errorf("not supported %w", coreerrors.NotSupported))
	} else {
		s.secretsBackend.EXPECT().SaveContent(gomock.Any(), uri, 3, coresecrets.NewSecretValue(map[string]string{"foo": "bar"})).
			Return("rev-id", nil)
	}
	if (finalStepFailed || labelExists) && !isInternal {
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
		SubjectTypeID: domainsecret.SubjectModel,
		SubjectID:     s.modelID.String(),
	}).Return("manage", nil)
	s.state.EXPECT().GetLatestRevision(gomock.Any(), uri).Return(2, nil)
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelID, nil).AnyTimes()
	rollbackCalled := false
	s.secretBackendState.EXPECT().AddSecretBackendReference(gomock.Any(), params.ValueRef, s.modelID, s.fakeUUID.String()).Return(func() error {
		rollbackCalled = true
		return nil
	}, nil)

	s.state.EXPECT().UpdateSecret(gomock.Any(), uri, params).
		DoAndReturn(func(context.Context, *coresecrets.URI, domainsecret.UpsertSecretParams) error {
			if labelExists {
				return secreterrors.SecretLabelAlreadyExists
			}
			if finalStepFailed {
				return errors.New("some error")
			}
			return nil
		})

	err := s.service.UpdateUserSecret(c.Context(), uri, UpdateUserSecretParams{
		Accessor: domainsecret.SecretAccessor{
			Kind: domainsecret.ModelAccessor,
			ID:   s.modelID.String(),
		},
		Description: ptr("a secret"),
		Label:       ptr("my secret"),
		Data:        map[string]string{"foo": "bar"},
		Checksum:    "checksum-1234",
		AutoPrune:   ptr(true),
	})
	if finalStepFailed || labelExists {
		c.Assert(rollbackCalled, tc.IsTrue)
		if labelExists {
			c.Assert(err, tc.ErrorIs, secreterrors.SecretLabelAlreadyExists)
		} else {
			c.Assert(err, tc.ErrorMatches, "updating user secret .*some error")
		}
	} else {
		c.Assert(err, tc.ErrorIsNil)
	}
}

func (s *serviceSuite) TestCreateCharmUnitSecret(c *tc.C) {
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
	unitUUID, err := coreunit.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.state.EXPECT().GetUnitUUID(domaintesting.IsAtomicContextChecker, unittesting.GenNewName(c, "mariadb/0")).Return(unitUUID, nil)
	s.state.EXPECT().CheckUnitSecretLabelExists(domaintesting.IsAtomicContextChecker, unitUUID, "my secret").Return(false, nil)
	s.state.EXPECT().CreateCharmUnitSecret(domaintesting.IsAtomicContextChecker, 1, uri, unitUUID, gomock.AssignableToTypeOf(p)).
		DoAndReturn(func(_ domain.AtomicContext, _ int, _ *coresecrets.URI, _ coreunit.UUID, got domainsecret.UpsertSecretParams) error {
			c.Assert(got.NextRotateTime, tc.NotNil)
			c.Assert(*got.NextRotateTime, tc.Almost, rotateTime)
			got.NextRotateTime = nil
			want := p
			want.NextRotateTime = nil
			c.Assert(got, tc.DeepEquals, want)
			return nil
		})
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelID, nil)
	rollbackCalled := false
	s.secretBackendState.EXPECT().AddSecretBackendReference(gomock.Any(), nil, s.modelID, s.fakeUUID.String()).Return(func() error {
		rollbackCalled = true
		return nil
	}, nil)

	err = s.service.CreateCharmSecret(c.Context(), uri, domainsecret.CreateCharmSecretParams{
		UpdateCharmSecretParams: domainsecret.UpdateCharmSecretParams{
			Accessor: domainsecret.SecretAccessor{
				Kind: domainsecret.UnitAccessor,
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
		CharmOwner: domainsecret.CharmSecretOwner{
			Kind: domainsecret.UnitCharmSecretOwner,
			ID:   "mariadb/0",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rollbackCalled, tc.IsFalse)
}

func (s *serviceSuite) TestCreateCharmUnitSecretFailedLabelAlreadyExists(c *tc.C) {
	defer s.setupMocks(c).Finish()

	exipreTime := s.clock.Now()
	uri := coresecrets.NewURI()

	unitUUID, err := coreunit.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.state.EXPECT().GetUnitUUID(domaintesting.IsAtomicContextChecker, unittesting.GenNewName(c, "mariadb/0")).Return(unitUUID, nil)
	s.state.EXPECT().CheckUnitSecretLabelExists(domaintesting.IsAtomicContextChecker, unitUUID, "my secret").Return(true, nil)
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelID, nil)
	rollbackCalled := false
	s.secretBackendState.EXPECT().AddSecretBackendReference(gomock.Any(), nil, s.modelID, s.fakeUUID.String()).Return(func() error {
		rollbackCalled = true
		return nil
	}, nil)

	err = s.service.CreateCharmSecret(c.Context(), uri, domainsecret.CreateCharmSecretParams{
		UpdateCharmSecretParams: domainsecret.UpdateCharmSecretParams{
			Accessor: domainsecret.SecretAccessor{
				Kind: domainsecret.UnitAccessor,
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
		CharmOwner: domainsecret.CharmSecretOwner{
			Kind: domainsecret.UnitCharmSecretOwner,
			ID:   "mariadb/0",
		},
	})
	c.Assert(err, tc.ErrorIs, secreterrors.SecretLabelAlreadyExists)
	c.Assert(rollbackCalled, tc.IsTrue)
}

func (s *serviceSuite) TestCreateCharmApplicationSecret(c *tc.C) {
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

	appUUID, err := coreapplication.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.ensurer.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(goodToken{})

	s.state.EXPECT().GetApplicationUUID(domaintesting.IsAtomicContextChecker, "mariadb").Return(appUUID, nil)
	s.state.EXPECT().CheckApplicationSecretLabelExists(domaintesting.IsAtomicContextChecker, appUUID, "my secret").Return(false, nil)
	s.state.EXPECT().CreateCharmApplicationSecret(domaintesting.IsAtomicContextChecker, 1, uri, appUUID, gomock.AssignableToTypeOf(p)).
		DoAndReturn(func(_ domain.AtomicContext, _ int, _ *coresecrets.URI, _ coreapplication.UUID, got domainsecret.UpsertSecretParams) error {
			c.Assert(got.NextRotateTime, tc.NotNil)
			c.Assert(*got.NextRotateTime, tc.Almost, rotateTime)
			got.NextRotateTime = nil
			want := p
			want.NextRotateTime = nil
			c.Assert(got, tc.DeepEquals, want)
			return nil
		})
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelID, nil)
	rollbackCalled := false
	s.secretBackendState.EXPECT().AddSecretBackendReference(gomock.Any(), nil, s.modelID, s.fakeUUID.String()).Return(func() error {
		rollbackCalled = true
		return nil
	}, nil)

	err = s.service.CreateCharmSecret(c.Context(), uri, domainsecret.CreateCharmSecretParams{
		UpdateCharmSecretParams: domainsecret.UpdateCharmSecretParams{
			Accessor: domainsecret.SecretAccessor{
				Kind: domainsecret.UnitAccessor,
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
		CharmOwner: domainsecret.CharmSecretOwner{
			Kind: domainsecret.ApplicationCharmSecretOwner,
			ID:   "mariadb",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rollbackCalled, tc.IsFalse)
}

func (s *serviceSuite) TestCreateCharmApplicationSecretFailedLabelExists(c *tc.C) {
	defer s.setupMocks(c).Finish()

	exipreTime := s.clock.Now()
	uri := coresecrets.NewURI()

	appUUID, err := coreapplication.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	s.ensurer.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(goodToken{})

	s.state.EXPECT().GetApplicationUUID(domaintesting.IsAtomicContextChecker, "mariadb").Return(appUUID, nil)
	s.state.EXPECT().CheckApplicationSecretLabelExists(domaintesting.IsAtomicContextChecker, appUUID, "my secret").Return(true, nil)
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelID, nil)
	rollbackCalled := false
	s.secretBackendState.EXPECT().AddSecretBackendReference(gomock.Any(), nil, s.modelID, s.fakeUUID.String()).Return(func() error {
		rollbackCalled = true
		return nil
	}, nil)

	err = s.service.CreateCharmSecret(c.Context(), uri, domainsecret.CreateCharmSecretParams{
		UpdateCharmSecretParams: domainsecret.UpdateCharmSecretParams{
			Accessor: domainsecret.SecretAccessor{
				Kind: domainsecret.UnitAccessor,
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
		CharmOwner: domainsecret.CharmSecretOwner{
			Kind: domainsecret.ApplicationCharmSecretOwner,
			ID:   "mariadb",
		},
	})
	c.Assert(err, tc.ErrorIs, secreterrors.SecretLabelAlreadyExists)
	c.Assert(rollbackCalled, tc.IsTrue)
}

func (s *serviceSuite) TestUpdateCharmSecretNoRotate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expireTime := s.clock.Now()
	uri := coresecrets.NewURI()

	p := domainsecret.UpsertSecretParams{
		RotatePolicy: ptr(domainsecret.RotateNever),
		Description:  ptr("a secret"),
		Label:        ptr("my secret"),
		Data:         coresecrets.SecretData{"foo": "bar"},
		Checksum:     "checksum-1234",
		ExpireTime:   ptr(expireTime),
		RevisionID:   ptr(s.fakeUUID.String()),
	}

	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mariadb/0",
	}).Return("manage", nil)
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelID, nil)
	rollbackCalled := false
	s.secretBackendState.EXPECT().AddSecretBackendReference(gomock.Any(), nil, s.modelID, s.fakeUUID.String()).Return(func() error {
		rollbackCalled = true
		return nil
	}, nil)

	s.state.EXPECT().UpdateSecret(gomock.Any(), uri, p).Return(nil)

	err := s.service.UpdateCharmSecret(c.Context(), uri, domainsecret.UpdateCharmSecretParams{
		Accessor: domainsecret.SecretAccessor{
			Kind: domainsecret.UnitAccessor,
			ID:   "mariadb/0",
		},
		Description: ptr("a secret"),
		Label:       ptr("my secret"),
		Data:        map[string]string{"foo": "bar"},
		Checksum:    "checksum-1234",
		ExpireTime:  ptr(expireTime),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rollbackCalled, tc.IsFalse)
}

func (s *serviceSuite) TestUpdateCharmSecretRotatePolicyTransitions(c *tc.C) {
	uri := coresecrets.NewURI()

	cases := []struct {
		name            string
		prev            coresecrets.RotatePolicy
		newPol          *coresecrets.RotatePolicy
		expectRecompute bool
	}{
		{
			name:            "RecomputeNextRotateFromNever",
			prev:            coresecrets.RotateNever,
			newPol:          ptr(coresecrets.RotateMonthly),
			expectRecompute: true,
		},
		{
			name:            "RecomputeNextRotateToNever",
			prev:            coresecrets.RotateMonthly,
			newPol:          ptr(coresecrets.RotateNever),
			expectRecompute: false, // This will be handled at the state level
		},
		{
			name:            "RecomputeNextRotateFromNeverToNever",
			prev:            coresecrets.RotateNever,
			newPol:          ptr(coresecrets.RotateNever),
			expectRecompute: false, // Edge case
		},
		{
			name:            "RecomputeNextRotateTimeIfNotMoreFrequent",
			prev:            coresecrets.RotateDaily,
			newPol:          ptr(coresecrets.RotateMonthly),
			expectRecompute: false,
		},
	}

	for _, tcse := range cases {
		c.Logf("case: %s", tcse.name)
		s.runRotatePolicyUpdateCase(c, uri, tcse.prev, tcse.newPol, tcse.expectRecompute)
	}
}

// runRotatePolicyUpdateCase encapsulates the common setup and assertions for
// UpdateCharmSecret rotate policy transition tests.
func (s *serviceSuite) runRotatePolicyUpdateCase(c *tc.C, uri *coresecrets.URI, prev coresecrets.RotatePolicy, newPol *coresecrets.RotatePolicy, expectRecompute bool) {
	defer s.setupMocks(c).Finish()

	// Build expected Upsert params.
	want := domainsecret.UpsertSecretParams{
		Label:        ptr("my secret"),
		Data:         coresecrets.SecretData{"foo": "bar"},
		RotatePolicy: ptr(domainsecret.MarshallRotatePolicy(newPol)),
		RevisionID:   ptr(s.fakeUUID.String()),
	}
	var expectedNext *time.Time
	if expectRecompute && newPol != nil && newPol.WillRotate() {
		// Compute from the suite's clock to match service usage.
		expectedNext = newPol.NextRotateTime(s.clock.Now())
		want.NextRotateTime = expectedNext
	}

	// Access check.
	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mariadb/0",
	}).Return("manage", nil)

	// Only query previous policy when the new policy will rotate.
	if newPol != nil && newPol.WillRotate() {
		s.state.EXPECT().GetRotatePolicy(gomock.Any(), uri).Return(prev, nil)
	}

	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelID, nil)
	rollbackCalled := false
	s.secretBackendState.EXPECT().AddSecretBackendReference(gomock.Any(), nil, s.modelID, s.fakeUUID.String()).Return(func() error {
		rollbackCalled = true
		return nil
	}, nil)

	s.state.EXPECT().UpdateSecret(gomock.Any(), uri, gomock.Any()).DoAndReturn(func(_ context.Context,
		_ *coresecrets.URI, got domainsecret.UpsertSecretParams) error {
		if expectRecompute {
			c.Assert(got.NextRotateTime, tc.NotNil)
			c.Assert(*got.NextRotateTime, tc.Almost, *expectedNext)
		} else {
			c.Assert(got.NextRotateTime, tc.IsNil)
		}
		// For deep equals, normalise NextRotateTime to nil on both sides if we asserted above.
		got.NextRotateTime = nil
		wantCopy := want
		wantCopy.NextRotateTime = nil
		c.Assert(got, tc.DeepEquals, wantCopy)
		return nil
	})

	err := s.service.UpdateCharmSecret(c.Context(), uri, domainsecret.UpdateCharmSecretParams{
		Accessor: domainsecret.SecretAccessor{
			Kind: domainsecret.UnitAccessor,
			ID:   "mariadb/0",
		},
		Label:        ptr("my secret"),
		Data:         map[string]string{"foo": "bar"},
		RotatePolicy: newPol,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rollbackCalled, tc.IsFalse)
}

// When updating the rotate policy to a less frequent schedule (e.g. daily -> monthly),
// we must NOT recompute nextRotateTime; it will be applied on the next rotation.
func (s *serviceSuite) TestUpdateCharmSecretDoNotRecomputeNextRotateTimeIfLessFrequent(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()

	// No nextRotateTime expected when policy becomes less frequent.
	p := domainsecret.UpsertSecretParams{
		RotatePolicy: ptr(domainsecret.RotateMonthly),
		Description:  ptr("a secret"),
		Label:        ptr("my secret"),
		Data:         coresecrets.SecretData{"foo": "bar"},
		Checksum:     "checksum-1234",
		RevisionID:   ptr(s.fakeUUID.String()),
	}

	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mariadb/0",
	}).Return("manage", nil)
	// Previous policy was daily; new policy monthly is less frequent -> do not recompute nextRotateTime.
	s.state.EXPECT().GetRotatePolicy(gomock.Any(), uri).Return(
		coresecrets.RotateDaily,
		nil)

	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelID, nil)
	rollbackCalled := false
	s.secretBackendState.EXPECT().AddSecretBackendReference(gomock.Any(), nil, s.modelID, s.fakeUUID.String()).Return(func() error {
		rollbackCalled = true
		return nil
	}, nil)

	s.state.EXPECT().UpdateSecret(gomock.Any(), uri, p).Return(nil)

	err := s.service.UpdateCharmSecret(c.Context(), uri, domainsecret.UpdateCharmSecretParams{
		Accessor: domainsecret.SecretAccessor{
			Kind: domainsecret.UnitAccessor,
			ID:   "mariadb/0",
		},
		Description:  ptr("a secret"),
		Label:        ptr("my secret"),
		Data:         map[string]string{"foo": "bar"},
		Checksum:     "checksum-1234",
		RotatePolicy: ptr(coresecrets.RotateMonthly),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rollbackCalled, tc.IsFalse)
}

func (s *serviceSuite) TestUpdateCharmSecret(c *tc.C) {
	defer s.setupMocks(c).Finish()

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

	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelID, nil)
	rollbackCalled := false
	s.secretBackendState.EXPECT().AddSecretBackendReference(gomock.Any(), nil, s.modelID, s.fakeUUID.String()).Return(func() error {
		rollbackCalled = true
		return nil
	}, nil)

	s.state.EXPECT().UpdateSecret(gomock.Any(), uri, gomock.Any()).DoAndReturn(func(_ context.Context,
		_ *coresecrets.URI, got domainsecret.UpsertSecretParams) error {
		c.Assert(got.NextRotateTime, tc.NotNil)
		c.Assert(*got.NextRotateTime, tc.Almost, *p.NextRotateTime)
		got.NextRotateTime = nil
		want := p
		want.NextRotateTime = nil
		c.Assert(got, tc.DeepEquals, want)
		return nil
	})

	err := s.service.UpdateCharmSecret(c.Context(), uri, domainsecret.UpdateCharmSecretParams{
		Accessor: domainsecret.SecretAccessor{
			Kind: domainsecret.UnitAccessor,
			ID:   "mariadb/0",
		},
		Description:  ptr("a secret"),
		Label:        ptr("my secret"),
		Data:         map[string]string{"foo": "bar"},
		Checksum:     "checksum-1234",
		RotatePolicy: ptr(coresecrets.RotateDaily),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rollbackCalled, tc.IsFalse)
}

func (s *serviceSuite) TestUpdateCharmSecretFailedStateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().GetSecretAccess(gomock.Any(), gomock.Any(), gomock.Any()).Return("manage", nil)
	s.state.EXPECT().GetRotatePolicy(gomock.Any(), gomock.Any()).Return(coresecrets.RotateNever, nil)

	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelID, nil)
	rollbackCalled := false
	s.secretBackendState.EXPECT().AddSecretBackendReference(gomock.Any(), nil, s.modelID, s.fakeUUID.String()).Return(func() error {
		rollbackCalled = true
		return nil
	}, nil)

	stateError := errors.New("boom")
	s.state.EXPECT().UpdateSecret(gomock.Any(), gomock.Any(), gomock.Any()).Return(stateError)

	err := s.service.UpdateCharmSecret(c.Context(), coresecrets.NewURI(), domainsecret.UpdateCharmSecretParams{
		Accessor: domainsecret.SecretAccessor{
			Kind: domainsecret.UnitAccessor,
			ID:   "mariadb/0",
		},
		Description:  ptr("a secret"),
		Label:        ptr("my secret"),
		Data:         map[string]string{"foo": "bar"},
		Checksum:     "checksum-1234",
		RotatePolicy: ptr(coresecrets.RotateDaily),
	})
	c.Assert(err, tc.ErrorIs, stateError)
	c.Assert(rollbackCalled, tc.IsTrue)
}

func (s *serviceSuite) TestGetSecret(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	md := &coresecrets.SecretMetadata{
		URI:   uri,
		Label: "my secret",
	}

	s.state.EXPECT().GetSecret(gomock.Any(), uri).Return(md, nil)

	got, err := s.service.GetSecret(c.Context(), uri)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, md)
}

func (s *serviceSuite) TestGetSecretValue(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()

	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mariadb/0",
	}).Return("manage", nil)
	s.state.EXPECT().GetSecretValue(gomock.Any(), uri, 666).Return(coresecrets.SecretData{"foo": "bar"}, nil, nil)

	data, ref, err := s.service.GetSecretValue(c.Context(), uri, 666, domainsecret.SecretAccessor{
		Kind: domainsecret.UnitAccessor,
		ID:   "mariadb/0",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ref, tc.IsNil)
	c.Assert(data, tc.DeepEquals, coresecrets.NewSecretValue(map[string]string{"foo": "bar"}))
}

func (s *serviceSuite) TestGetSecretConsumer(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	consumer := &coresecrets.SecretConsumerMetadata{
		Label:           "my secret",
		CurrentRevision: 666,
	}

	s.state.EXPECT().GetSecretConsumer(gomock.Any(), uri, unittesting.GenNewName(c, "mysql/0")).Return(consumer, 666, nil)

	got, err := s.service.GetSecretConsumer(c.Context(), uri, "mysql/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, consumer)
}

func (s *serviceSuite) TestGetSecretConsumerAndLatest(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	consumer := &coresecrets.SecretConsumerMetadata{
		Label:           "my secret",
		CurrentRevision: 666,
	}

	s.state.EXPECT().GetSecretConsumer(gomock.Any(), uri, unittesting.GenNewName(c, "mysql/0")).Return(consumer, 666, nil)

	got, latest, err := s.service.GetSecretConsumerAndLatest(c.Context(), uri, "mysql/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, consumer)
	c.Assert(latest, tc.Equals, 666)
}

func (s *serviceSuite) TestSaveSecretConsumer(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	consumer := coresecrets.SecretConsumerMetadata{
		Label:           "my secret",
		CurrentRevision: 666,
	}

	s.state.EXPECT().SaveSecretConsumer(gomock.Any(), uri, unittesting.GenNewName(c, "mysql/0"), consumer).Return(nil)

	err := s.service.SaveSecretConsumer(c.Context(), uri, "mysql/0", consumer)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestGetUserSecretURIByLabel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()

	s.state.EXPECT().GetUserSecretURIByLabel(gomock.Any(), "my label").Return(uri, nil)

	got, err := s.service.GetUserSecretURIByLabel(c.Context(), "my label")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, uri)
}

func (s *serviceSuite) TestListCharmSecretsToDrain(c *tc.C) {
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

	got, err := s.service.ListCharmSecretsToDrain(c.Context(), []domainsecret.CharmSecretOwner{{
		Kind: domainsecret.UnitCharmSecretOwner,
		ID:   "mariadb/0",
	}, {
		Kind: domainsecret.ApplicationCharmSecretOwner,
		ID:   "mariadb",
	}}...)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, md)
}

func (s *serviceSuite) TestListUserSecretsToDrain(c *tc.C) {
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

	got, err := s.service.ListUserSecretsToDrain(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, md)
}

func (s *serviceSuite) TestListCharmSecrets(c *tc.C) {
	defer s.setupMocks(c).Finish()

	owners := []domainsecret.CharmSecretOwner{{
		Kind: domainsecret.ApplicationCharmSecretOwner,
		ID:   "mysql",
	}, {
		Kind: domainsecret.UnitCharmSecretOwner,
		ID:   "mysql/0",
	}}
	md := []*coresecrets.SecretMetadata{{Label: "one"}}
	revs := [][]*coresecrets.SecretRevisionMetadata{{{Revision: 1}}}

	s.state.EXPECT().ListCharmSecrets(gomock.Any(), domainsecret.ApplicationOwners{"mysql"}, domainsecret.UnitOwners{"mysql/0"}).
		Return(md, revs, nil)
	s.secretBackendState.EXPECT().GetSecretBackendNamesWithUUIDs(gomock.Any()).Return(map[string]string{}, nil)

	gotSecrets, gotRevisions, err := s.service.ListCharmSecrets(c.Context(), owners...)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotSecrets, tc.DeepEquals, md)
	c.Assert(gotRevisions, tc.HasLen, 1)
}

func (s *serviceSuite) TestListSecretsErrWhenURIAndLabelsProvided(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	labels := domainsecret.Labels{"env"}

	_, _, err := s.service.ListSecrets(c.Context(), uri, nil, labels)
	c.Assert(err, tc.ErrorMatches, "cannot specify both URI and labels")
}

func (s *serviceSuite) TestListSecretsByURI(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	md := &coresecrets.SecretMetadata{URI: uri}
	revs := []*coresecrets.SecretRevisionMetadata{{Revision: 7}}

	s.secretBackendState.EXPECT().GetSecretBackendNamesWithUUIDs(gomock.Any()).Return(map[string]string{}, nil)
	s.state.EXPECT().GetSecretByURI(gomock.Any(), *uri, (*int)(nil)).Return(md, revs, nil)

	gotMDs, gotRevs, err := s.service.ListSecrets(c.Context(), uri, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotMDs, tc.HasLen, 1)
	c.Assert(gotRevs, tc.HasLen, 1)
	c.Assert(gotMDs[0], tc.DeepEquals, md)
	c.Assert(gotRevs[0], tc.DeepEquals, revs)
}

func (s *serviceSuite) TestListSecretsByURIError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()

	s.secretBackendState.EXPECT().GetSecretBackendNamesWithUUIDs(gomock.Any()).Return(map[string]string{}, nil)
	s.state.EXPECT().GetSecretByURI(gomock.Any(), *uri, (*int)(nil)).Return(nil, nil, errors.New("boom"))

	md, revs, err := s.service.ListSecrets(c.Context(), uri, nil, nil)
	c.Assert(md, tc.IsNil)
	c.Assert(revs, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf("getting secret by URI %q: boom", uri.ID))
}

func (s *serviceSuite) TestListSecretsByURIWithBackendName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	md := &coresecrets.SecretMetadata{URI: uri}

	backendUUID1 := uuid.MustNewUUID().String()
	backendUUID2 := uuid.MustNewUUID().String()
	revs := []*coresecrets.SecretRevisionMetadata{
		{
			Revision: 1,
			ValueRef: &coresecrets.ValueRef{BackendID: backendUUID1},
		}, {
			Revision: 2,
			ValueRef: &coresecrets.ValueRef{BackendID: backendUUID2},
		},
	}

	secretBackendsWithUUIDs := map[string]string{
		backendUUID1: "vault-one",
		backendUUID2: "vault-two",
	}

	s.secretBackendState.EXPECT().GetSecretBackendNamesWithUUIDs(gomock.Any()).Return(secretBackendsWithUUIDs, nil)
	s.state.EXPECT().GetSecretByURI(gomock.Any(), *uri, (*int)(nil)).Return(md, revs, nil)

	expectedMDs := []*coresecrets.SecretMetadata{md}
	expectedRes := [][]*coresecrets.SecretRevisionMetadata{revs}
	gotMDs, gotRevs, err := s.service.ListSecrets(c.Context(), uri, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotMDs, tc.DeepEquals, expectedMDs)
	c.Assert(gotRevs, tc.DeepEquals, expectedRes)

	c.Assert(gotRevs, tc.HasLen, 1)
	backend := gotRevs[0]
	c.Assert(backend, tc.HasLen, 2)

	c.Assert(backend[0].ValueRef, tc.NotNil)
	backendID1 := backend[0].ValueRef.BackendID
	c.Assert(backend[0].BackendName, tc.NotNil)
	c.Assert(*backend[0].BackendName, tc.Equals, secretBackendsWithUUIDs[backendID1])

	c.Assert(backend[1].ValueRef, tc.NotNil)
	backendID2 := backend[1].ValueRef.BackendID
	c.Assert(backend[1].BackendName, tc.NotNil)
	c.Assert(*backend[1].BackendName, tc.Equals, secretBackendsWithUUIDs[backendID2])
}

func (s *serviceSuite) TestListSecretsByURIWithoutBackendName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	md := &coresecrets.SecretMetadata{URI: uri}
	revs := []*coresecrets.SecretRevisionMetadata{
		nil,
		{
			Revision: 1,
		},
		{
			Revision: 2,
			ValueRef: &coresecrets.ValueRef{BackendID: "missing-backend"},
		},
	}

	s.secretBackendState.EXPECT().GetSecretBackendNamesWithUUIDs(gomock.Any()).Return(map[string]string{
		uuid.MustNewUUID().String(): "kubernetes",
		uuid.MustNewUUID().String(): "internal",
	}, nil)
	s.state.EXPECT().GetSecretByURI(gomock.Any(), *uri, (*int)(nil)).Return(md, revs, nil)

	_, gotRevs, err := s.service.ListSecrets(c.Context(), uri, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotRevs, tc.HasLen, 1)
	c.Assert(gotRevs[0], tc.HasLen, 3)

	// As long as the revision is not nil, we should have a backend name (either resolved or default).
	c.Assert(gotRevs[0][0], tc.IsNil)
	defaultBackendName := juju.BackendName
	c.Assert(gotRevs[0][1].BackendName, tc.DeepEquals, &defaultBackendName)
	c.Assert(gotRevs[0][2].BackendName, tc.DeepEquals, &defaultBackendName)
}

func (s *serviceSuite) TestListSecretsByURIWithRevision(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	rev := 3
	md := &coresecrets.SecretMetadata{URI: uri}

	backendUUID := uuid.MustNewUUID().String()
	revs := []*coresecrets.SecretRevisionMetadata{{
		Revision: 3,
		ValueRef: &coresecrets.ValueRef{BackendID: backendUUID},
	}}

	s.secretBackendState.EXPECT().GetSecretBackendNamesWithUUIDs(gomock.Any()).Return(map[string]string{
		backendUUID: "vault-one",
	}, nil)
	s.state.EXPECT().GetSecretByURI(gomock.Any(), *uri, &rev).Return(md, revs, nil)

	gotMDs, gotRevs, err := s.service.ListSecrets(c.Context(), uri, &rev, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotMDs, tc.HasLen, 1)
	c.Assert(gotMDs[0], tc.DeepEquals, md)
	c.Assert(gotRevs, tc.HasLen, 1)
	c.Assert(gotRevs[0], tc.HasLen, 1)
	c.Assert(gotRevs[0][0].BackendName, tc.NotNil)
	c.Assert(*gotRevs[0][0].BackendName, tc.Equals, "vault-one")
}

func (s *serviceSuite) TestListSecretsByURIWithRevisionError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	rev := 3

	s.secretBackendState.EXPECT().GetSecretBackendNamesWithUUIDs(gomock.Any()).Return(map[string]string{}, nil)
	s.state.EXPECT().GetSecretByURI(gomock.Any(), *uri, &rev).Return(nil, nil, errors.New("boom"))

	md, revs, err := s.service.ListSecrets(c.Context(), uri, &rev, nil)
	c.Assert(md, tc.IsNil)
	c.Assert(revs, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, fmt.Sprintf("getting secret by URI %q: boom", uri.ID))
}

func (s *serviceSuite) TestListSecretsByLabels(c *tc.C) {
	defer s.setupMocks(c).Finish()

	labels := domainsecret.Labels{"tier"}
	md := []*coresecrets.SecretMetadata{{Label: "one"}, {Label: "two"}}

	backendUUID1 := uuid.MustNewUUID().String()
	backendUUID2 := uuid.MustNewUUID().String()
	revs := [][]*coresecrets.SecretRevisionMetadata{
		{{
			Revision: 1,
			ValueRef: &coresecrets.ValueRef{BackendID: backendUUID1},
		}},
		{{
			Revision: 2,
			ValueRef: &coresecrets.ValueRef{BackendID: backendUUID2},
		}},
	}

	secretBackendsWithUUIDs := map[string]string{
		backendUUID1: "vault-one",
		backendUUID2: "vault-two",
	}

	s.secretBackendState.EXPECT().GetSecretBackendNamesWithUUIDs(gomock.Any()).Return(secretBackendsWithUUIDs, nil)
	s.state.EXPECT().ListSecretsByLabels(gomock.Any(), labels, (*int)(nil)).Return(md, revs, nil)

	gotMDs, gotRevs, err := s.service.ListSecrets(c.Context(), nil, nil, labels)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotMDs, tc.DeepEquals, md)
	c.Assert(gotRevs, tc.DeepEquals, revs)

	for _, backend := range gotRevs {
		c.Assert(backend, tc.HasLen, 1)
		c.Assert(backend[0].ValueRef, tc.NotNil)

		backendID := backend[0].ValueRef.BackendID
		c.Assert(backend[0].BackendName, tc.NotNil)
		c.Assert(*backend[0].BackendName, tc.Equals, secretBackendsWithUUIDs[backendID])
	}
}

func (s *serviceSuite) TestListSecretsByLabelsWithoutBackendName(c *tc.C) {
	defer s.setupMocks(c).Finish()

	labels := domainsecret.Labels{"tier"}
	md := []*coresecrets.SecretMetadata{{Label: "one"}, {Label: "two"}, {Label: "three"}}
	revs := [][]*coresecrets.SecretRevisionMetadata{
		nil,
		{{
			Revision: 1,
		}},
		{{
			Revision: 2,
			ValueRef: &coresecrets.ValueRef{BackendID: "missing-backend"},
		}},
	}

	s.secretBackendState.EXPECT().GetSecretBackendNamesWithUUIDs(gomock.Any()).Return(map[string]string{
		uuid.MustNewUUID().String(): "kubernetes",
		uuid.MustNewUUID().String(): "internal",
	}, nil)
	s.state.EXPECT().ListSecretsByLabels(gomock.Any(), labels, (*int)(nil)).Return(md, revs, nil)

	_, gotRevs, err := s.service.ListSecrets(c.Context(), nil, nil, labels)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotRevs, tc.HasLen, 3)
	defaultBackendName := juju.BackendName

	// As long as the revision is not nil, we should have a backend name (either resolved or default).
	c.Assert(gotRevs[0], tc.IsNil)
	c.Assert(gotRevs[1][0].BackendName, tc.DeepEquals, &defaultBackendName)
	c.Assert(gotRevs[2][0].BackendName, tc.DeepEquals, &defaultBackendName)
}

func (s *serviceSuite) TestListSecretsByLabelsError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	labels := domainsecret.Labels{"tier"}

	// Error getting secrets by labels from state.
	s.secretBackendState.EXPECT().GetSecretBackendNamesWithUUIDs(gomock.Any()).Return(map[string]string{}, nil)
	s.state.EXPECT().ListSecretsByLabels(gomock.Any(), labels, (*int)(nil)).Return(nil, nil, errors.New("ListSecretsByLabels err"))
	md, revs, err := s.service.ListSecrets(c.Context(), nil, nil, labels)
	c.Assert(md, tc.IsNil)
	c.Assert(revs, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "getting secrets by labels: ListSecretsByLabels err")

	// Error getting backend names with UUIDs from state.
	s.secretBackendState.EXPECT().GetSecretBackendNamesWithUUIDs(gomock.Any()).Return(nil, errors.New("GetSecretBackendNamesWithUUIDs err"))
	_, _, err = s.service.ListSecrets(c.Context(), nil, nil, labels)
	c.Assert(err, tc.ErrorMatches, "getting secret backend names with UUIDs: GetSecretBackendNamesWithUUIDs err")
}

func (s *serviceSuite) TestListSecretsByLabelsWithRevision(c *tc.C) {
	defer s.setupMocks(c).Finish()

	labels := domainsecret.Labels{"owner"}
	rev := 3
	md := []*coresecrets.SecretMetadata{{Label: "rev-3"}}

	backendUUID := uuid.MustNewUUID().String()
	revs := [][]*coresecrets.SecretRevisionMetadata{{
		{
			Revision: 3,
			ValueRef: &coresecrets.ValueRef{BackendID: backendUUID},
		},
	}}

	s.secretBackendState.EXPECT().GetSecretBackendNamesWithUUIDs(gomock.Any()).Return(map[string]string{
		backendUUID: "vault-one",
	}, nil)
	s.state.EXPECT().ListSecretsByLabels(gomock.Any(), labels, &rev).Return(md, revs, nil)

	gotMDs, gotRevs, err := s.service.ListSecrets(c.Context(), nil, &rev, labels)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotMDs, tc.DeepEquals, md)
	c.Assert(gotRevs, tc.DeepEquals, revs)
	c.Assert(gotRevs, tc.HasLen, 1)
	c.Assert(gotRevs[0], tc.HasLen, 1)
	c.Assert(gotRevs[0][0].BackendName, tc.NotNil)
	c.Assert(*gotRevs[0][0].BackendName, tc.Equals, "vault-one")
}

func (s *serviceSuite) TestListSecretsByLabelsWithRevisionError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	labels := domainsecret.Labels{"owner"}
	rev := 3

	s.secretBackendState.EXPECT().GetSecretBackendNamesWithUUIDs(gomock.Any()).Return(map[string]string{}, nil)
	s.state.EXPECT().ListSecretsByLabels(gomock.Any(), labels, &rev).Return(nil, nil, errors.New("boom"))

	md, revs, err := s.service.ListSecrets(c.Context(), nil, &rev, labels)
	c.Assert(md, tc.IsNil)
	c.Assert(revs, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "getting secrets by labels: boom")
}

func (s *serviceSuite) TestListSecretsErrOnRevisionOnly(c *tc.C) {
	defer s.setupMocks(c).Finish()

	rev := 2
	s.secretBackendState.EXPECT().GetSecretBackendNamesWithUUIDs(gomock.Any()).Return(map[string]string{}, nil)
	_, _, err := s.service.ListSecrets(c.Context(), nil, &rev, nil)
	c.Assert(err, tc.ErrorMatches, "cannot specify revision without URI or labels")
}

func (s *serviceSuite) TestListSecretsAll(c *tc.C) {
	defer s.setupMocks(c).Finish()

	md := []*coresecrets.SecretMetadata{{Label: "a"}}
	backendUUID := uuid.MustNewUUID().String()
	revs := [][]*coresecrets.SecretRevisionMetadata{{
		{
			Revision: 1,
			ValueRef: &coresecrets.ValueRef{BackendID: backendUUID},
		},
	}}

	s.secretBackendState.EXPECT().GetSecretBackendNamesWithUUIDs(gomock.Any()).Return(map[string]string{
		backendUUID: "vault-all",
	}, nil)
	s.state.EXPECT().ListAllSecrets(gomock.Any()).Return(md, revs, nil)

	gotMDs, gotRevs, err := s.service.ListSecrets(c.Context(), nil, nil, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotMDs, tc.DeepEquals, md)
	c.Assert(gotRevs, tc.DeepEquals, revs)
}

func (s *serviceSuite) TestListSecretsAllError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.secretBackendState.EXPECT().GetSecretBackendNamesWithUUIDs(gomock.Any()).Return(map[string]string{}, nil)
	s.state.EXPECT().ListAllSecrets(gomock.Any()).Return(nil, nil, errors.New("boom"))

	md, revs, err := s.service.ListSecrets(c.Context(), nil, nil, nil)
	c.Assert(md, tc.IsNil)
	c.Assert(revs, tc.IsNil)
	c.Assert(err, tc.ErrorMatches, "listing all secrets: boom")
}

func (s *serviceSuite) TestListCharmJustApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()

	owners := []domainsecret.CharmSecretOwner{{
		Kind: domainsecret.ApplicationCharmSecretOwner,
		ID:   "mysql",
	}}
	md := []*coresecrets.SecretMetadata{{Label: "one"}}
	revs := [][]*coresecrets.SecretRevisionMetadata{{{Revision: 1}}}

	s.state.EXPECT().ListCharmSecrets(gomock.Any(), domainsecret.ApplicationOwners{"mysql"}, domainsecret.NilUnitOwners).
		Return(md, revs, nil)
	s.secretBackendState.EXPECT().GetSecretBackendNamesWithUUIDs(gomock.Any()).Return(map[string]string{}, nil)

	gotSecrets, gotRevisions, err := s.service.ListCharmSecrets(c.Context(), owners...)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotSecrets, tc.DeepEquals, md)
	c.Assert(gotRevisions, tc.HasLen, 1)
}

func (s *serviceSuite) TestListCharmJustUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	owners := []domainsecret.CharmSecretOwner{{
		Kind: domainsecret.UnitCharmSecretOwner,
		ID:   "mysql/0",
	}}
	md := []*coresecrets.SecretMetadata{{Label: "one"}}
	revs := [][]*coresecrets.SecretRevisionMetadata{{{Revision: 1}}}

	s.state.EXPECT().ListCharmSecrets(gomock.Any(), domainsecret.NilApplicationOwners, domainsecret.UnitOwners{"mysql/0"}).
		Return(md, revs, nil)
	s.secretBackendState.EXPECT().GetSecretBackendNamesWithUUIDs(gomock.Any()).Return(map[string]string{}, nil)

	gotSecrets, gotRevisions, err := s.service.ListCharmSecrets(c.Context(), owners...)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotSecrets, tc.DeepEquals, md)
	c.Assert(gotRevisions, tc.HasLen, 1)
}

func (s *serviceSuite) TestGetURIByConsumerLabel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	s.state.EXPECT().GetURIByConsumerLabel(gomock.Any(), "my label", unittesting.GenNewName(c, "mysql/0")).Return(uri, nil)

	got, err := s.service.GetURIByConsumerLabel(c.Context(), "my label", "mysql/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, uri)
}

func (s *serviceSuite) TestGrantSecretUnitAccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	appUUID := tc.Must(c, coreapplication.NewUUID)
	unitUUID := unittesting.GenUnitUUID(c)
	s.state.EXPECT().GetApplicationUUID(domaintesting.IsAtomicContextChecker, "mysql").Return(appUUID, nil)
	s.state.EXPECT().GetUnitUUID(domaintesting.IsAtomicContextChecker, coreunit.Name("mysql/0")).Return(unitUUID, nil)
	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "another/0",
	}).Return("manage", nil)
	s.state.EXPECT().GrantAccess(gomock.Any(), uri, domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeApplication,
		ScopeUUID:     appUUID.String(),
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectUUID:   unitUUID.String(),
		RoleID:        domainsecret.RoleManage,
	}).Return(nil)

	err := s.service.GrantSecretAccess(c.Context(), uri, domainsecret.SecretAccessParams{
		Accessor: domainsecret.SecretAccessor{
			Kind: domainsecret.UnitAccessor,
			ID:   "another/0",
		},
		Scope: domainsecret.SecretAccessScope{
			Kind: domainsecret.ApplicationAccessScope,
			ID:   "mysql",
		},
		Subject: domainsecret.SecretAccessor{
			Kind: domainsecret.UnitAccessor,
			ID:   "mysql/0",
		},
		Role: "manage",
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestGrantSecretApplicationAccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	appUUID := tc.Must(c, coreapplication.NewUUID)
	s.state.EXPECT().GetApplicationUUID(domaintesting.IsAtomicContextChecker, "mysql").Return(appUUID, nil)
	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "another/0",
	}).Return("manage", nil)
	s.state.EXPECT().GrantAccess(gomock.Any(), uri, domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeApplication,
		ScopeUUID:     appUUID.String(),
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectUUID:   appUUID.String(),
		RoleID:        domainsecret.RoleView,
	}).Return(nil)

	err := s.service.GrantSecretAccess(c.Context(), uri, domainsecret.SecretAccessParams{
		Accessor: domainsecret.SecretAccessor{
			Kind: domainsecret.UnitAccessor,
			ID:   "another/0",
		},
		Scope: domainsecret.SecretAccessScope{
			Kind: domainsecret.ApplicationAccessScope,
			ID:   "mysql",
		},
		Subject: domainsecret.SecretAccessor{
			Kind: domainsecret.ApplicationAccessor,
			ID:   "mysql",
		},
		Role: "view",
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestGrantSecretModelAccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

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

	err := s.service.GrantSecretAccess(c.Context(), uri, domainsecret.SecretAccessParams{
		Accessor: domainsecret.SecretAccessor{
			Kind: domainsecret.ModelAccessor,
			ID:   "model-uuid",
		},
		Scope: domainsecret.SecretAccessScope{
			Kind: domainsecret.ModelAccessScope,
		},
		Subject: domainsecret.SecretAccessor{
			Kind: domainsecret.ModelAccessor,
		},
		Role: "manage",
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestGrantSecretRelationScope(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	appUUID := tc.Must(c, coreapplication.NewUUID)
	s.state.EXPECT().GetApplicationUUID(domaintesting.IsAtomicContextChecker, "mysql").Return(appUUID, nil)
	relUUID := relationtesting.GenRelationUUID(c)
	s.state.EXPECT().GetRegularRelationUUIDByEndpointIdentifiers(gomock.Any(), relation.EndpointIdentifier{
		ApplicationName: "mediawiki",
		EndpointName:    "db",
		Role:            charm.RoleRequirer,
	}, relation.EndpointIdentifier{
		ApplicationName: "mysql",
		EndpointName:    "db",
		Role:            charm.RoleProvider,
	}).Return(relUUID.String(), nil)
	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "another/0",
	}).Return("manage", nil)
	s.state.EXPECT().GrantAccess(gomock.Any(), uri, domainsecret.GrantParams{
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeUUID:     relUUID.String(),
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectUUID:   appUUID.String(),
		RoleID:        domainsecret.RoleView,
	}).Return(nil)

	err := s.service.GrantSecretAccess(c.Context(), uri, domainsecret.SecretAccessParams{
		Accessor: domainsecret.SecretAccessor{
			Kind: domainsecret.UnitAccessor,
			ID:   "another/0",
		},
		Scope: domainsecret.SecretAccessScope{
			Kind: domainsecret.RelationAccessScope,
			ID:   "mediawiki:db mysql:db",
		},
		Subject: domainsecret.SecretAccessor{
			Kind: domainsecret.ApplicationAccessor,
			ID:   "mysql",
		},
		Role: "view",
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestRevokeSecretUnitAccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	unitUUID := unittesting.GenUnitUUID(c)
	s.state.EXPECT().GetUnitUUID(domaintesting.IsAtomicContextChecker, coreunit.Name("another/0")).Return(unitUUID, nil)
	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
	}).Return("manage", nil)
	s.state.EXPECT().RevokeAccess(gomock.Any(), uri, domainsecret.RevokeParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectUUID:   unitUUID.String(),
	}).Return(nil)

	err := s.service.RevokeSecretAccess(c.Context(), uri, domainsecret.SecretAccessParams{
		Accessor: domainsecret.SecretAccessor{
			Kind: domainsecret.UnitAccessor,
			ID:   "mysql/0",
		},
		Subject: domainsecret.SecretAccessor{
			Kind: domainsecret.UnitAccessor,
			ID:   "another/0",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestRevokeSecretApplicationAccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	appUUID := tc.Must(c, coreapplication.NewUUID)
	s.state.EXPECT().GetApplicationUUID(domaintesting.IsAtomicContextChecker, "another").Return(appUUID, nil)
	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
	}).Return("manage", nil)
	s.state.EXPECT().RevokeAccess(gomock.Any(), uri, domainsecret.RevokeParams{
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectUUID:   appUUID.String(),
	}).Return(nil)

	err := s.service.RevokeSecretAccess(c.Context(), uri, domainsecret.SecretAccessParams{
		Accessor: domainsecret.SecretAccessor{
			Kind: domainsecret.UnitAccessor,
			ID:   "mysql/0",
		},
		Subject: domainsecret.SecretAccessor{
			Kind: domainsecret.ApplicationAccessor,
			ID:   "another",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestRevokeSecretModelAccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	appUUID := tc.Must(c, coreapplication.NewUUID)
	s.state.EXPECT().GetApplicationUUID(domaintesting.IsAtomicContextChecker, "mysql").Return(appUUID, nil)
	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectModel,
		SubjectID:     "model-uuid",
	}).Return("manage", nil)
	s.state.EXPECT().RevokeAccess(gomock.Any(), uri, domainsecret.RevokeParams{
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectUUID:   appUUID.String(),
	}).Return(nil)

	err := s.service.RevokeSecretAccess(c.Context(), uri, domainsecret.SecretAccessParams{
		Accessor: domainsecret.SecretAccessor{
			Kind: domainsecret.ModelAccessor,
			ID:   "model-uuid",
		},
		Subject: domainsecret.SecretAccessor{
			Kind: domainsecret.ApplicationAccessor,
			ID:   "mysql",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestGetSecretAccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
	}).Return("manage", nil)

	role, err := s.service.getSecretAccess(c.Context(), uri, domainsecret.SecretAccessor{
		Kind: domainsecret.ApplicationAccessor,
		ID:   "mysql",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(role, tc.Equals, coresecrets.RoleManage)
}

func (s *serviceSuite) TestGetSecretAccessNone(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
	}).Return("", nil)

	role, err := s.service.getSecretAccess(c.Context(), uri, domainsecret.SecretAccessor{
		Kind: domainsecret.ApplicationAccessor,
		ID:   "mysql",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(role, tc.Equals, coresecrets.RoleNone)
}

func (s *serviceSuite) TestGetSecretAccessRelationScope(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	relUUID := relationtesting.GenRelationUUID(c)
	s.state.EXPECT().GetSecretAccessRelationScope(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
	}).Return(relUUID.String(), nil)

	got, err := s.service.GetSecretAccessRelationScope(c.Context(), uri, domainsecret.SecretAccessor{
		Kind: domainsecret.ApplicationAccessor,
		ID:   "mysql",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.Equals, relUUID)
}

func (s *serviceSuite) TestGetSecretGrants(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	s.state.EXPECT().GetSecretGrants(gomock.Any(), uri, coresecrets.RoleView).Return([]domainsecret.GrantDetails{{
		ScopeTypeID:   domainsecret.ScopeModel,
		ScopeID:       "model-uuid",
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
		RoleID:        domainsecret.RoleView,
	}, {
		ScopeTypeID:   domainsecret.ScopeRelation,
		ScopeUUID:     "relation-uuid",
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mediawiki/0",
		RoleID:        domainsecret.RoleView,
	}}, nil)
	s.state.EXPECT().GetRelationEndpoints(gomock.Any(), "relation-uuid").Return([]relation.EndpointIdentifier{{
		ApplicationName: "mediawiki",
		EndpointName:    "db",
		Role:            "requirer",
	}, {
		ApplicationName: "mysql",
		EndpointName:    "db",
		Role:            "provider",
	}}, nil)

	g, err := s.service.GetSecretGrants(c.Context(), uri, coresecrets.RoleView)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(g, tc.DeepEquals, []SecretAccess{{
		Scope: domainsecret.SecretAccessScope{
			Kind: domainsecret.ModelAccessScope,
			ID:   "model-uuid",
		},
		Subject: domainsecret.SecretAccessor{
			Kind: domainsecret.ApplicationAccessor,
			ID:   "mysql",
		},
		Role: coresecrets.RoleView,
	}, {
		Scope: domainsecret.SecretAccessScope{
			Kind: domainsecret.RelationAccessScope,
			ID:   "mediawiki:db mysql:db",
		},
		Subject: domainsecret.SecretAccessor{
			Kind: domainsecret.UnitAccessor,
			ID:   "mediawiki/0",
		},
		Role: coresecrets.RoleView,
	}})
}

func (s *serviceSuite) TestChangeSecretBackendToExternalBackend(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	ctx := c.Context()

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
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelID, nil)
	rollbackCalled := false
	s.secretBackendState.EXPECT().UpdateSecretBackendReference(gomock.Any(), valueRef, s.modelID, s.fakeUUID.String()).Return(func() error {
		rollbackCalled = true
		return nil
	}, nil)

	err := s.service.ChangeSecretBackend(ctx, uri, 1, ChangeSecretBackendParams{
		Accessor: domainsecret.SecretAccessor{
			Kind: domainsecret.UnitAccessor,
			ID:   "mariadb/0",
		},
		ValueRef: valueRef,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rollbackCalled, tc.IsFalse)
}

func (s *serviceSuite) TestChangeSecretBackendToInternalBackend(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	ctx := c.Context()

	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mariadb/0",
	}).Return("manage", nil)
	s.state.EXPECT().GetSecretRevisionID(gomock.Any(), uri, 1).Return(s.fakeUUID.String(), nil)
	s.state.EXPECT().ChangeSecretBackend(gomock.Any(), s.fakeUUID, nil, map[string]string{"foo": "bar"}).Return(nil)
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelID, nil)
	rollbackCalled := false
	s.secretBackendState.EXPECT().UpdateSecretBackendReference(gomock.Any(), nil, s.modelID, s.fakeUUID.String()).Return(func() error {
		rollbackCalled = true
		return nil
	}, nil)

	err := s.service.ChangeSecretBackend(ctx, uri, 1, ChangeSecretBackendParams{
		Accessor: domainsecret.SecretAccessor{
			Kind: domainsecret.UnitAccessor,
			ID:   "mariadb/0",
		},
		Data: map[string]string{"foo": "bar"},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rollbackCalled, tc.IsFalse)
}

func (s *serviceSuite) TestChangeSecretBackendFailedAndRollback(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	ctx := c.Context()

	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mariadb/0",
	}).Return("manage", nil)
	s.state.EXPECT().GetSecretRevisionID(gomock.Any(), uri, 1).Return(s.fakeUUID.String(), nil)
	s.state.EXPECT().ChangeSecretBackend(gomock.Any(), s.fakeUUID, nil, map[string]string{"foo": "bar"}).Return(errors.New("boom"))
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelID, nil)
	rollbackCalled := false
	s.secretBackendState.EXPECT().UpdateSecretBackendReference(gomock.Any(), nil, s.modelID, s.fakeUUID.String()).Return(func() error {
		rollbackCalled = true
		return nil
	}, nil)

	err := s.service.ChangeSecretBackend(ctx, uri, 1, ChangeSecretBackendParams{
		Accessor: domainsecret.SecretAccessor{
			Kind: domainsecret.UnitAccessor,
			ID:   "mariadb/0",
		},
		Data: map[string]string{"foo": "bar"},
	})
	c.Assert(err, tc.ErrorMatches, `boom`)
	c.Assert(rollbackCalled, tc.IsTrue)
}

func (s *serviceSuite) TestChangeSecretBackendFailedPermissionDenied(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	ctx := c.Context()

	s.ensurer.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(badToken{})

	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mariadb/0",
	}).Return("view", nil)

	err := s.service.ChangeSecretBackend(ctx, uri, 1, ChangeSecretBackendParams{
		Accessor: domainsecret.SecretAccessor{
			Kind: domainsecret.UnitAccessor,
			ID:   "mariadb/0",
		},
		Data: map[string]string{"foo": "bar"},
	})
	c.Assert(err, tc.ErrorIs, secreterrors.PermissionDenied)
}

func (s *serviceSuite) TestChangeSecretBackendFailedSecretNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	ctx := c.Context()

	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mariadb/0",
	}).Return("manage", nil)
	s.state.EXPECT().GetSecretRevisionID(gomock.Any(), uri, 1).Return(s.fakeUUID.String(), nil)
	s.state.EXPECT().ChangeSecretBackend(gomock.Any(), s.fakeUUID, nil, map[string]string{"foo": "bar"}).Return(secreterrors.SecretNotFound)
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelID, nil)
	rollbackCalled := false
	s.secretBackendState.EXPECT().UpdateSecretBackendReference(gomock.Any(), nil, s.modelID, s.fakeUUID.String()).Return(func() error {
		rollbackCalled = true
		return nil
	}, nil)

	err := s.service.ChangeSecretBackend(ctx, uri, 1, ChangeSecretBackendParams{
		Accessor: domainsecret.SecretAccessor{
			Kind: domainsecret.UnitAccessor,
			ID:   "mariadb/0",
		},
		Data: map[string]string{"foo": "bar"},
	})
	c.Assert(err, tc.ErrorIs, secreterrors.SecretNotFound)
	c.Assert(rollbackCalled, tc.IsTrue)
}

func (s *serviceSuite) TestSecretsRotated(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	ctx := c.Context()
	nextRotateTime := s.clock.Now().Add(time.Hour)

	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mariadb/0",
	}).Return("manage", nil)
	s.state.EXPECT().SecretRotated(gomock.Any(), uri, gomock.Any()).DoAndReturn(
		func(ctx context.Context, uri *coresecrets.URI, next time.Time) error {
			c.Assert(next, tc.Almost, nextRotateTime)
			return errors.New("boom")
		})
	s.state.EXPECT().GetRotationExpiryInfo(gomock.Any(), uri).Return(&domainsecret.RotationExpiryInfo{
		RotatePolicy:   coresecrets.RotateHourly,
		LatestRevision: 667,
	}, nil)

	err := s.service.SecretRotated(ctx, uri, SecretRotatedParams{
		Accessor: domainsecret.SecretAccessor{
			Kind: domainsecret.UnitAccessor,
			ID:   "mariadb/0",
		},
		OriginalRevision: 666,
	})
	c.Assert(err, tc.ErrorMatches, `boom`)
}

func (s *serviceSuite) TestSecretsRotatedRetry(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	ctx := c.Context()
	nextRotateTime := s.clock.Now().Add(coresecrets.RotateRetryDelay)

	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mariadb/0",
	}).Return("manage", nil)
	s.state.EXPECT().SecretRotated(gomock.Any(), uri, gomock.Any()).DoAndReturn(
		func(ctx context.Context, uri *coresecrets.URI, next time.Time) error {
			c.Assert(next, tc.Almost, nextRotateTime)
			return errors.New("boom")
		})
	s.state.EXPECT().GetRotationExpiryInfo(gomock.Any(), uri).Return(&domainsecret.RotationExpiryInfo{
		RotatePolicy:   coresecrets.RotateHourly,
		LatestRevision: 666,
	}, nil)

	err := s.service.SecretRotated(ctx, uri, SecretRotatedParams{
		Accessor: domainsecret.SecretAccessor{
			Kind: domainsecret.UnitAccessor,
			ID:   "mariadb/0",
		},
		OriginalRevision: 666,
	})
	c.Assert(err, tc.ErrorMatches, `boom`)
}

func (s *serviceSuite) TestSecretsRotatedForce(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	ctx := c.Context()
	nextRotateTime := s.clock.Now().Add(coresecrets.RotateRetryDelay)

	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mariadb/0",
	}).Return("manage", nil)
	s.state.EXPECT().SecretRotated(gomock.Any(), uri, gomock.Any()).DoAndReturn(
		func(ctx context.Context, uri *coresecrets.URI, next time.Time) error {
			c.Assert(next, tc.Almost, nextRotateTime)
			return errors.New("boom")
		})
	s.state.EXPECT().GetRotationExpiryInfo(gomock.Any(), uri).Return(&domainsecret.RotationExpiryInfo{
		RotatePolicy:     coresecrets.RotateHourly,
		LatestExpireTime: ptr(s.clock.Now().Add(50 * time.Minute)),
		LatestRevision:   667,
	}, nil)

	err := s.service.SecretRotated(ctx, uri, SecretRotatedParams{
		Accessor: domainsecret.SecretAccessor{
			Kind: domainsecret.UnitAccessor,
			ID:   "mariadb/0",
		},
		OriginalRevision: 666,
	})
	c.Assert(err, tc.ErrorMatches, `boom`)
}

func (s *serviceSuite) TestSecretsRotatedThenNever(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	ctx := c.Context()

	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mariadb/0",
	}).Return("manage", nil)
	s.state.EXPECT().GetRotationExpiryInfo(gomock.Any(), uri).Return(&domainsecret.RotationExpiryInfo{
		RotatePolicy:   coresecrets.RotateNever,
		LatestRevision: 667,
	}, nil)

	err := s.service.SecretRotated(ctx, uri, SecretRotatedParams{
		Accessor: domainsecret.SecretAccessor{
			Kind: domainsecret.UnitAccessor,
			ID:   "mariadb/0",
		},
		OriginalRevision: 666,
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestGetConsumedRevisionFirstTime(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()

	s.state.EXPECT().GetSecretConsumer(gomock.Any(), uri, unittesting.GenNewName(c, "mariadb/0")).Return(nil, 666, secreterrors.SecretConsumerNotFound)
	s.state.EXPECT().SaveSecretConsumer(gomock.Any(), uri, unittesting.GenNewName(c, "mariadb/0"), coresecrets.SecretConsumerMetadata{
		CurrentRevision: 666,
	})

	rev, err := s.service.GetConsumedRevision(c.Context(), uri, "mariadb/0", false, false, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rev, tc.Equals, 666)
}

func (s *serviceSuite) TestGetConsumedRevisionFirstTimeUpdateLabel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()

	s.state.EXPECT().GetSecretConsumer(gomock.Any(), uri, unittesting.GenNewName(c, "mariadb/0")).Return(nil, 666, secreterrors.SecretConsumerNotFound)
	s.state.EXPECT().SaveSecretConsumer(gomock.Any(), uri, unittesting.GenNewName(c, "mariadb/0"), coresecrets.SecretConsumerMetadata{
		Label:           "label",
		CurrentRevision: 666,
	})

	rev, err := s.service.GetConsumedRevision(c.Context(), uri, "mariadb/0", false, false, ptr("label"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rev, tc.Equals, 666)
}

func (s *serviceSuite) TestGetSecretConsumedRevisionUpdateLabel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()

	s.state.EXPECT().GetSecretConsumer(gomock.Any(), uri, unittesting.GenNewName(c, "mariadb/0")).Return(&coresecrets.SecretConsumerMetadata{
		Label:           "old-label",
		CurrentRevision: 666,
	}, 666, nil)
	s.state.EXPECT().SaveSecretConsumer(gomock.Any(), uri, unittesting.GenNewName(c, "mariadb/0"), coresecrets.SecretConsumerMetadata{
		Label:           "new-label",
		CurrentRevision: 666,
	})

	rev, err := s.service.GetConsumedRevision(c.Context(), uri, "mariadb/0", false, false, ptr("new-label"))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rev, tc.Equals, 666)
}

func (s *serviceSuite) TestGetSecretConsumedRevisionRefresh(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()

	s.state.EXPECT().GetSecretConsumer(gomock.Any(), uri, unittesting.GenNewName(c, "mariadb/0")).Return(&coresecrets.SecretConsumerMetadata{
		Label:           "old-label",
		CurrentRevision: 666,
	}, 668, nil)
	s.state.EXPECT().SaveSecretConsumer(gomock.Any(), uri, unittesting.GenNewName(c, "mariadb/0"), coresecrets.SecretConsumerMetadata{
		Label:           "old-label",
		CurrentRevision: 668,
	})

	rev, err := s.service.GetConsumedRevision(c.Context(), uri, "mariadb/0", true, false, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rev, tc.Equals, 668)
}

func (s *serviceSuite) TestGetSecretConsumedRevisionPeek(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()

	s.state.EXPECT().GetSecretConsumer(gomock.Any(), uri, unittesting.GenNewName(c, "mariadb/0")).Return(&coresecrets.SecretConsumerMetadata{
		Label:           "old-label",
		CurrentRevision: 666,
	}, 668, nil)

	rev, err := s.service.GetConsumedRevision(c.Context(), uri, "mariadb/0", false, true, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rev, tc.Equals, 668)
}

func (s *serviceSuite) TestGetSecretConsumedRevisionSecretNotFound(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()

	s.state.EXPECT().GetSecretConsumer(gomock.Any(), uri, unittesting.GenNewName(c, "mariadb/0")).Return(&coresecrets.SecretConsumerMetadata{
		CurrentRevision: 666,
	}, 668, nil)
	s.state.EXPECT().SaveSecretConsumer(gomock.Any(), uri, unittesting.GenNewName(c, "mariadb/0"), coresecrets.SecretConsumerMetadata{
		CurrentRevision: 668,
	})

	uri2 := coresecrets.NewURI()
	md := []*coresecrets.SecretMetadata{{
		URI:            uri2,
		LatestRevision: 668,
		Label:          "foz",
	}}
	revs := [][]*coresecrets.SecretRevisionMetadata{{{Revision: 1}}}

	s.state.EXPECT().ListCharmSecrets(gomock.Any(), domainsecret.ApplicationOwners{"mariadb"}, domainsecret.UnitOwners{"mariadb/0"}).
		Return(md, revs, nil)
	s.secretBackendState.EXPECT().GetSecretBackendNamesWithUUIDs(gomock.Any()).Return(map[string]string{}, nil)

	rev, err := s.service.GetConsumedRevision(c.Context(), uri, "mariadb/0", true, false, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rev, tc.Equals, 668)
}

func (s *serviceSuite) TestProcessCharmSecretConsumerLabelSecretUpdateLabel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()

	md := []*coresecrets.SecretMetadata{{
		URI:            uri,
		LatestRevision: 668,
		Label:          "foz",
		Owner: coresecrets.Owner{
			Kind: "unit",
			ID:   "mariadb/0",
		},
	}}
	revs := [][]*coresecrets.SecretRevisionMetadata{{{Revision: 1}}}

	s.state.EXPECT().ListCharmSecrets(gomock.Any(), domainsecret.ApplicationOwners{"mariadb"}, domainsecret.UnitOwners{"mariadb/0"}).
		Return(md, revs, nil)
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(coremodel.UUID(coretesting.ModelTag.Id()), nil)
	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectID:     "mariadb/0",
		SubjectTypeID: domainsecret.SubjectUnit,
	}).Return("manage", nil)

	s.state.EXPECT().UpdateSecret(gomock.Any(), uri, domainsecret.UpsertSecretParams{
		RotatePolicy: ptr(domainsecret.RotateNever),
		Label:        ptr("foo"),
	}).Return(nil)
	s.secretBackendState.EXPECT().GetSecretBackendNamesWithUUIDs(gomock.Any()).Return(nil, nil)

	gotURI, gotLabel, err := s.service.ProcessCharmSecretConsumerLabel(c.Context(), "mariadb/0", uri, "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotURI, tc.DeepEquals, uri)
	c.Assert(gotLabel, tc.IsNil)
}

func (s *serviceSuite) TestProcessCharmSecretConsumerLabelForUnitOwnedSecretLookupURI(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	md := []*coresecrets.SecretMetadata{{
		URI:            uri,
		LatestRevision: 668,
		Label:          "foo",
	}}
	revs := [][]*coresecrets.SecretRevisionMetadata{{{Revision: 1}}}

	s.state.EXPECT().ListCharmSecrets(gomock.Any(), domainsecret.ApplicationOwners{"mariadb"}, domainsecret.UnitOwners{"mariadb/0"}).
		Return(md, revs, nil)
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(coremodel.UUID(coretesting.ModelTag.Id()), nil)
	s.secretBackendState.EXPECT().GetSecretBackendNamesWithUUIDs(gomock.Any()).Return(nil, nil)

	gotURI, gotLabel, err := s.service.ProcessCharmSecretConsumerLabel(c.Context(), "mariadb/0", nil, "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotURI, tc.DeepEquals, uri)
	c.Assert(gotLabel, tc.IsNil)
}

func (s *serviceSuite) TestProcessCharmSecretConsumerLabelLookupURI(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	md := []*coresecrets.SecretMetadata{{
		URI:            uri,
		LatestRevision: 668,
		Label:          "foz",
	}}
	revs := [][]*coresecrets.SecretRevisionMetadata{{{Revision: 1}}}

	s.state.EXPECT().ListCharmSecrets(gomock.Any(), domainsecret.ApplicationOwners{"mariadb"}, domainsecret.UnitOwners{"mariadb/0"}).
		Return(md, revs, nil)
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(coremodel.UUID(coretesting.ModelTag.Id()), nil)
	s.state.EXPECT().GetURIByConsumerLabel(gomock.Any(), "foo", unittesting.GenNewName(c, "mariadb/0")).Return(uri, nil)
	s.secretBackendState.EXPECT().GetSecretBackendNamesWithUUIDs(gomock.Any()).Return(nil, nil)

	gotURI, gotLabel, err := s.service.ProcessCharmSecretConsumerLabel(c.Context(), "mariadb/0", nil, "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotURI, tc.DeepEquals, uri)
	c.Assert(gotLabel, tc.IsNil)
}

func (s *serviceSuite) TestProcessCharmSecretConsumerLabelUpdateLabel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	md := []*coresecrets.SecretMetadata{{
		URI:            uri2,
		LatestRevision: 668,
		Label:          "foz",
	}}
	revs := [][]*coresecrets.SecretRevisionMetadata{{{Revision: 1}}}

	s.state.EXPECT().ListCharmSecrets(gomock.Any(), domainsecret.ApplicationOwners{"mariadb"}, domainsecret.UnitOwners{"mariadb/0"}).
		Return(md, revs, nil)
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(coremodel.UUID(coretesting.ModelTag.Id()), nil)
	s.secretBackendState.EXPECT().GetSecretBackendNamesWithUUIDs(gomock.Any()).Return(nil, nil)

	gotURI, gotLabel, err := s.service.ProcessCharmSecretConsumerLabel(c.Context(), "mariadb/0", uri, "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotURI, tc.DeepEquals, uri)
	c.Assert(gotLabel, tc.DeepEquals, ptr("foo"))
}

func (s *serviceSuite) TestGetLatestRevisions(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uris := []*coresecrets.URI{
		coresecrets.NewURI(),
		coresecrets.NewURI(),
	}
	ctx := c.Context()

	s.state.EXPECT().GetLatestRevisions(gomock.Any(), uris).Return(map[string]int{
		uris[0].ID: 666,
		uris[1].ID: 667,
	}, nil)

	latest, err := s.service.GetLatestRevisions(ctx, uris)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(latest, tc.DeepEquals, map[string]int{
		uris[0].ID: 666,
		uris[1].ID: 667,
	})
}

type changeEvent struct {
	changed    string
	namespace  string
	changeType changestream.ChangeType
}

func newSecretChangeEvent(changed string) *changeEvent {
	return &changeEvent{
		changed:   changed,
		namespace: "secret_metadata",
		// changeType is not been used, we just set it to 2(update).
		changeType: 2,
	}
}

func newObsoleteRevisionChangeEvent(changed string) *changeEvent {
	return &changeEvent{
		changed:   changed,
		namespace: "secret_revision_obsolete",
		// changeType is not been used, we just set it to 2(update).
		changeType: 2,
	}
}

func (c *changeEvent) String() string {
	return fmt.Sprintf("%s: %s", c.namespace, c.changed)
}

func (c *changeEvent) Changed() string {
	return c.changed
}
func (c *changeEvent) Namespace() string {
	return c.namespace
}

func (c *changeEvent) Type() changestream.ChangeType {
	return c.changeType
}

// TestWatchObsoleteMapperSendObsoleteRevisionAndRemovedURIs tests the behavior of the mapper function
// when it receives obsolete revision events and secret change events.
// Only owned secret events and owned obsolete revision events will be processed.
// When secret change event and its corresponding obsolete revision event are received together,
// the secret obsolete event will be omitted if the secret has been removed.
func (s *serviceSuite) TestWatchObsoleteMapperSendObsoleteRevisionAndRemovedURIs(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	appOwners := domainsecret.ApplicationOwners([]string{"mysql"})
	unitOwners := domainsecret.UnitOwners([]string{"mysql/0", "mysql/1"})

	ownedURI := coresecrets.NewURI()

	s.state.EXPECT().GetRevisionIDsForObsolete(gomock.Any(),
		appOwners, unitOwners,
		[]string{
			"revision-uuid-3",
			"revision-uuid-1",
			"revision-uuid-2",
		},
	).Return(
		[]string{
			ownedURI.ID + "/1",
			ownedURI.ID + "/3",
		}, nil,
	)

	mapper := s.service.obsoleteWatcherMapperFunc(appOwners, unitOwners)
	result, err := mapper(
		c.Context(),
		[]changestream.ChangeEvent{
			// Owned obsolete revision events will be sent in order.
			newObsoleteRevisionChangeEvent("revision-uuid-3"),
			newObsoleteRevisionChangeEvent("revision-uuid-1"),

			// Not owned obsolete revision will be ignored.
			newObsoleteRevisionChangeEvent("revision-uuid-2"),
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.SameContents, []string{
		ownedURI.ID + "/3",
		ownedURI.ID + "/1",
	})
}

// TestWatchObsoleteMapperSendObsoleteRevisions tests the behavior of the mapper function
// when it receives obsolete revision events.
// Only owned obsolete revision events will be processed.
func (s *serviceSuite) TestWatchObsoleteMapperSendObsoleteRevisions(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	appOwners := domainsecret.ApplicationOwners([]string{"mysql"})
	unitOwners := domainsecret.UnitOwners([]string{"mysql/0", "mysql/1"})

	ownedURI := coresecrets.NewURI()

	s.state.EXPECT().GetRevisionIDsForObsolete(gomock.Any(),
		appOwners, unitOwners, []string{
			"revision-uuid-3",
			"revision-uuid-2",
			"revision-uuid-1",
			"revision-uuid-4",
		},
	).Return(
		[]string{
			ownedURI.ID + "/1",
			ownedURI.ID + "/2",
			ownedURI.ID + "/3",
		}, nil,
	)

	mapper := s.service.obsoleteWatcherMapperFunc(appOwners, unitOwners)
	result, err := mapper(
		c.Context(),
		[]changestream.ChangeEvent{
			// Owned obsolete revision events will be sent in order.
			newObsoleteRevisionChangeEvent("revision-uuid-3"),
			newObsoleteRevisionChangeEvent("revision-uuid-2"),
			newObsoleteRevisionChangeEvent("revision-uuid-1"),

			// Not owned obsolete revision will be ignored.
			newObsoleteRevisionChangeEvent("revision-uuid-4"),
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.SameContents, []string{
		ownedURI.ID + "/3",
		ownedURI.ID + "/2",
		ownedURI.ID + "/1",
	})
}

// TestWatchDeletedMapperSendRemovedURIs tests the behavior of the mapper function
// when it receives secret change events.
// Only owned secret change events will be processed if the secret has been removed.
func (s *serviceSuite) TestWatchObsoleteMapperSendRemovedURIs(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	appOwners := domainsecret.ApplicationOwners([]string{"mysql"})
	unitOwners := domainsecret.UnitOwners([]string{"mysql/0", "mysql/1"})

	removedOwnedURI1 := coresecrets.NewURI()
	removedOwnedURI2 := coresecrets.NewURI()
	notOwnedURI := coresecrets.NewURI()

	gomock.InOrder(
		// When we receive the initial event, the secrets are not removed yet.
		s.state.EXPECT().GetOwnedSecretIDs(gomock.Any(), appOwners, unitOwners).Return(
			[]string{removedOwnedURI1.ID, removedOwnedURI2.ID}, nil,
		),
		// When we receive the event 2nd time, the secrets have been removed.
		s.state.EXPECT().GetOwnedSecretIDs(gomock.Any(), appOwners, unitOwners).Return(
			[]string{}, nil,
		),
	)

	mapper := deletedWatcherMapperFunc(
		loggertesting.WrapCheckLog(c),
		s.state,
		[]string{removedOwnedURI1.ID, removedOwnedURI2.ID},
		appOwners, unitOwners,
		"secret_metadata", "custom_deleted_secret_revision_by_id",
	)
	result, err := mapper(
		c.Context(),
		[]changestream.ChangeEvent{
			// The initial events.
			newSecretChangeEvent(removedOwnedURI1.ID),
			newSecretChangeEvent(removedOwnedURI2.ID),
			newSecretChangeEvent(notOwnedURI.ID),
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 0)
	result, err = mapper(
		c.Context(),
		[]changestream.ChangeEvent{
			// Deletion events of the secretWatcher are sent in order.
			newSecretChangeEvent(removedOwnedURI2.ID),
			newSecretChangeEvent(removedOwnedURI1.ID),

			// Not owned secret change event will be ignored.
			newSecretChangeEvent(notOwnedURI.ID),
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 2)
	secretChange2 := result[0]
	secretChange1 := result[1]
	c.Assert(secretChange2, tc.Equals, removedOwnedURI2.ID)
	c.Assert(secretChange1, tc.Equals, removedOwnedURI1.ID)
}

func (s *serviceSuite) TestWatchObsolete(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	mockWatcherFactory := NewMockWatcherFactory(ctrl)
	mockWatcherFactory.EXPECT().NewNamespaceMapperWatcher(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(
			_ context.Context,
			_ eventsource.NamespaceQuery,
			_ string,
			_ eventsource.Mapper,
			secretFilter eventsource.FilterOption, filters ...eventsource.FilterOption,
		) (watcher.Watcher[[]string], error) {
			c.Assert(secretFilter.Namespace(), tc.Equals, "secret_revision_obsolete")
			c.Assert(secretFilter.ChangeMask(), tc.Equals, changestream.Changed)
			c.Assert(filters, tc.HasLen, 0)
			return NewMockStringsWatcher(ctrl), nil
		},
	)

	var namespaceQuery = func(context.Context, database.TxnRunner) ([]string, error) {
		return []string{}, nil
	}
	appUUID := tc.Must(c, uuid.NewUUID).String()
	unit1UUID := tc.Must(c, uuid.NewUUID).String()
	unit2UUID := tc.Must(c, uuid.NewUUID).String()
	s.state.EXPECT().GetApplicationUUIDsForNames(gomock.Any(), domainsecret.ApplicationOwners{"mysql"}).
		Return([]string{appUUID}, nil)
	s.state.EXPECT().GetUnitUUIDsForNames(gomock.Any(), domainsecret.UnitOwners{"mysql/0", "mysql/1"}).
		Return([]string{unit1UUID, unit2UUID}, nil)
	s.state.EXPECT().InitialWatchStatementForObsoleteRevision(
		domainsecret.ApplicationOwners{appUUID},
		domainsecret.UnitOwners{unit1UUID, unit2UUID},
	).Return("secret_revision_obsolete", namespaceQuery)

	svc := NewWatchableService(
		s.state, s.secretBackendState, s.ensurer, mockWatcherFactory, loggertesting.WrapCheckLog(c))
	w, err := svc.WatchObsoleteSecrets(c.Context(),
		domainsecret.CharmSecretOwner{
			Kind: domainsecret.ApplicationCharmSecretOwner,
			ID:   "mysql",
		},
		domainsecret.CharmSecretOwner{
			Kind: domainsecret.UnitCharmSecretOwner,
			ID:   "mysql/0",
		},
		domainsecret.CharmSecretOwner{
			Kind: domainsecret.UnitCharmSecretOwner,
			ID:   "mysql/1",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(w, tc.NotNil)
}

func (s *serviceSuite) TestWatchObsoleteUserSecretsToPrune(c *tc.C) {
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

	s.state.EXPECT().NamespaceForWatchSecretRevisionObsolete().Return("secret_revision_obsolete")
	s.state.EXPECT().NamespaceForWatchSecretMetadata().Return("secret_metadata")

	mockWatcherFactory.EXPECT().NewNotifyMapperWatcher(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, _ eventsource.Mapper, fo eventsource.FilterOption, _ ...eventsource.FilterOption) (watcher.Watcher[struct{}], error) {
			c.Assert(fo.Namespace(), tc.Equals, "secret_revision_obsolete")
			return mockObsoleteWatcher, nil
		},
	)
	mockWatcherFactory.EXPECT().NewNotifyMapperWatcher(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ string, _ eventsource.Mapper, fo eventsource.FilterOption, _ ...eventsource.FilterOption) (watcher.Watcher[struct{}], error) {
			c.Assert(fo.Namespace(), tc.Equals, "secret_metadata")
			return mockAutoPruneWatcher, nil
		},
	)

	svc := NewWatchableService(
		s.state, s.secretBackendState, s.ensurer, mockWatcherFactory, loggertesting.WrapCheckLog(c))
	w, err := svc.WatchObsoleteUserSecretsToPrune(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(w, tc.NotNil)
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

func (s *serviceSuite) TestWatchConsumedSecretsChanges(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	mockWatcherFactory := NewMockWatcherFactory(ctrl)
	expectedWatcher := NewMockStringsWatcher(ctrl)

	var namespaceQuery eventsource.NamespaceQuery = func(context.Context, database.TxnRunner) ([]string, error) {
		return nil, nil
	}
	s.state.EXPECT().InitialWatchStatementForConsumedSecretsChange(unittesting.GenNewName(c, "mysql/0")).Return("secret_revision", namespaceQuery)
	s.state.EXPECT().InitialWatchStatementForConsumedRemoteSecretsChange(unittesting.GenNewName(c, "mysql/0")).Return("secret_reference", namespaceQuery)
	mockWatcherFactory.EXPECT().NewNamespaceMapperWatcher(
		gomock.Any(), gomock.Any(),
		"consumed secrets watcher",
		gomock.Any(),
		tc.Bind(tc.DeepEquals, eventsource.NamespaceFilter("secret_revision", changestream.Changed)),
		tc.Bind(tc.DeepEquals, eventsource.NamespaceFilter("secret_reference", changestream.All)),
	).Return(expectedWatcher, nil)

	svc := NewWatchableService(
		s.state, s.secretBackendState, s.ensurer, mockWatcherFactory, loggertesting.WrapCheckLog(c))
	w, err := svc.WatchConsumedSecretsChanges(c.Context(), "mysql/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(w, tc.Equals, expectedWatcher)
}

func (s *serviceSuite) TestWatchSecretsRotationChanges(c *tc.C) {
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
	mockWatcherFactory.EXPECT().NewNamespaceWatcher(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(mockStringWatcher, nil)

	now := s.clock.Now()
	s.state.EXPECT().GetSecretsRotationChanges(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners, secretIDs ...string) ([]domainsecret.RotationInfo, error) {
			c.Assert(appOwners, tc.SameContents, domainsecret.ApplicationOwners{"mediawiki"})
			c.Assert(unitOwners, tc.SameContents, domainsecret.UnitOwners{"mysql/0", "mysql/1"})
			c.Assert(secretIDs, tc.SameContents, []string{uri1.ID, uri2.ID})
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
		},
	)

	svc := NewWatchableService(
		s.state, s.secretBackendState, s.ensurer, mockWatcherFactory, loggertesting.WrapCheckLog(c))
	w, err := svc.WatchSecretsRotationChanges(c.Context(),
		domainsecret.CharmSecretOwner{
			Kind: domainsecret.ApplicationCharmSecretOwner,
			ID:   "mediawiki",
		},
		domainsecret.CharmSecretOwner{
			Kind: domainsecret.UnitCharmSecretOwner,
			ID:   "mysql/0",
		},
		domainsecret.CharmSecretOwner{
			Kind: domainsecret.UnitCharmSecretOwner,
			ID:   "mysql/1",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(w, tc.NotNil)
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

func (s *serviceSuite) TestWatchSecretRevisionsExpiryChanges(c *tc.C) {
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
	mockWatcherFactory.EXPECT().NewNamespaceWatcher(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(mockStringWatcher, nil)

	now := s.clock.Now()
	s.state.EXPECT().GetSecretsRevisionExpiryChanges(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, appOwners domainsecret.ApplicationOwners, unitOwners domainsecret.UnitOwners, revisionUUIDs ...string) ([]domainsecret.ExpiryInfo, error) {
			c.Assert(appOwners, tc.SameContents, domainsecret.ApplicationOwners{"mediawiki"})
			c.Assert(unitOwners, tc.SameContents, domainsecret.UnitOwners{"mysql/0", "mysql/1"})
			c.Assert(revisionUUIDs, tc.SameContents, []string{"revision-uuid-1", "revision-uuid-2"})
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
		},
	)

	svc := NewWatchableService(
		s.state, s.secretBackendState, s.ensurer, mockWatcherFactory, loggertesting.WrapCheckLog(c))
	w, err := svc.WatchSecretRevisionsExpiryChanges(c.Context(),
		domainsecret.CharmSecretOwner{
			Kind: domainsecret.ApplicationCharmSecretOwner,
			ID:   "mediawiki",
		},
		domainsecret.CharmSecretOwner{
			Kind: domainsecret.UnitCharmSecretOwner,
			ID:   "mysql/0",
		},
		domainsecret.CharmSecretOwner{
			Kind: domainsecret.UnitCharmSecretOwner,
			ID:   "mysql/1",
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(w, tc.NotNil)
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
