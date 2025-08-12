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
	modeltesting "github.com/juju/juju/core/model/testing"
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
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/secrets/provider"
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
	s.modelID = modeltesting.GenModelUUID(c)
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
			Accessor: SecretAccessor{
				Kind: ModelAccessor,
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

	s.state.EXPECT().GetSecretOwner(domaintesting.IsAtomicContextChecker, uri).Return(domainsecret.Owner{Kind: domainsecret.ModelOwner}, nil)
	s.state.EXPECT().CheckUserSecretLabelExists(domaintesting.IsAtomicContextChecker, "my secret").Return(labelExists, nil)
	if !labelExists {
		s.state.EXPECT().UpdateSecret(domaintesting.IsAtomicContextChecker, uri, params).
			DoAndReturn(func(domain.AtomicContext, *coresecrets.URI, domainsecret.UpsertSecretParams) error {
				if finalStepFailed {
					return errors.New("some error")
				}
				return nil
			})
	}

	err := s.service.UpdateUserSecret(c.Context(), uri, UpdateUserSecretParams{
		Accessor: SecretAccessor{
			Kind: ModelAccessor,
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

	err = s.service.CreateCharmSecret(c.Context(), uri, CreateCharmSecretParams{
		UpdateCharmSecretParams: UpdateCharmSecretParams{
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

	err = s.service.CreateCharmSecret(c.Context(), uri, CreateCharmSecretParams{
		UpdateCharmSecretParams: UpdateCharmSecretParams{
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

	appUUID, err := coreapplication.NewID()
	c.Assert(err, tc.ErrorIsNil)

	s.ensurer.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(goodToken{})

	s.state.EXPECT().GetApplicationUUID(domaintesting.IsAtomicContextChecker, "mariadb").Return(appUUID, nil)
	s.state.EXPECT().CheckApplicationSecretLabelExists(domaintesting.IsAtomicContextChecker, appUUID, "my secret").Return(false, nil)
	s.state.EXPECT().CreateCharmApplicationSecret(domaintesting.IsAtomicContextChecker, 1, uri, appUUID, gomock.AssignableToTypeOf(p)).
		DoAndReturn(func(_ domain.AtomicContext, _ int, _ *coresecrets.URI, _ coreapplication.ID, got domainsecret.UpsertSecretParams) error {
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

	err = s.service.CreateCharmSecret(c.Context(), uri, CreateCharmSecretParams{
		UpdateCharmSecretParams: UpdateCharmSecretParams{
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rollbackCalled, tc.IsFalse)
}

func (s *serviceSuite) TestCreateCharmApplicationSecretFailedLabelExists(c *tc.C) {
	defer s.setupMocks(c).Finish()

	exipreTime := s.clock.Now()
	uri := coresecrets.NewURI()

	appUUID, err := coreapplication.NewID()
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

	err = s.service.CreateCharmSecret(c.Context(), uri, CreateCharmSecretParams{
		UpdateCharmSecretParams: UpdateCharmSecretParams{
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
	c.Assert(err, tc.ErrorIs, secreterrors.SecretLabelAlreadyExists)
	c.Assert(rollbackCalled, tc.IsTrue)
}

func (s *serviceSuite) TestUpdateCharmSecretNoRotate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	exipreTime := s.clock.Now()
	uri := coresecrets.NewURI()

	unitUUID, err := coreunit.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

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
	s.state.EXPECT().GetModelUUID(gomock.Any()).Return(s.modelID, nil)
	rollbackCalled := false
	s.secretBackendState.EXPECT().AddSecretBackendReference(gomock.Any(), nil, s.modelID, s.fakeUUID.String()).Return(func() error {
		rollbackCalled = true
		return nil
	}, nil)
	s.state.EXPECT().GetSecretOwner(domaintesting.IsAtomicContextChecker, uri).Return(domainsecret.Owner{Kind: domainsecret.UnitOwner, UUID: unitUUID.String()}, nil)
	s.state.EXPECT().CheckUnitSecretLabelExists(domaintesting.IsAtomicContextChecker, unitUUID, "my secret").Return(false, nil)
	s.state.EXPECT().UpdateSecret(domaintesting.IsAtomicContextChecker, uri, p).Return(nil)

	err = s.service.UpdateCharmSecret(c.Context(), uri, UpdateCharmSecretParams{
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rollbackCalled, tc.IsFalse)
}

func (s *serviceSuite) TestUpdateCharmSecretForUnitOwned(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()

	unitUUID, err := coreunit.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

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

	s.state.EXPECT().GetSecretOwner(domaintesting.IsAtomicContextChecker, uri).Return(domainsecret.Owner{Kind: domainsecret.UnitOwner, UUID: unitUUID.String()}, nil)
	s.state.EXPECT().CheckUnitSecretLabelExists(domaintesting.IsAtomicContextChecker, unitUUID, "my secret").Return(false, nil)
	s.state.EXPECT().UpdateSecret(domaintesting.IsAtomicContextChecker, uri, gomock.Any()).DoAndReturn(func(_ domain.AtomicContext, _ *coresecrets.URI, got domainsecret.UpsertSecretParams) error {
		c.Assert(got.NextRotateTime, tc.NotNil)
		c.Assert(*got.NextRotateTime, tc.Almost, *p.NextRotateTime)
		got.NextRotateTime = nil
		want := p
		want.NextRotateTime = nil
		c.Assert(got, tc.DeepEquals, want)
		return nil
	})

	err = s.service.UpdateCharmSecret(c.Context(), uri, UpdateCharmSecretParams{
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rollbackCalled, tc.IsFalse)
}

func (s *serviceSuite) TestUpdateCharmSecretForUnitOwnedFailedLabelExists(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()

	unitUUID, err := coreunit.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

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

	s.state.EXPECT().GetSecretOwner(domaintesting.IsAtomicContextChecker, uri).Return(domainsecret.Owner{Kind: domainsecret.UnitOwner, UUID: unitUUID.String()}, nil)
	s.state.EXPECT().CheckUnitSecretLabelExists(domaintesting.IsAtomicContextChecker, unitUUID, "my secret").Return(true, nil)

	err = s.service.UpdateCharmSecret(c.Context(), uri, UpdateCharmSecretParams{
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
	c.Assert(err, tc.ErrorIs, secreterrors.SecretLabelAlreadyExists)
	c.Assert(rollbackCalled, tc.IsTrue)
}

func (s *serviceSuite) TestUpdateCharmSecretForAppOwned(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()

	appUUID, err := coreapplication.NewID()
	c.Assert(err, tc.ErrorIsNil)

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

	s.state.EXPECT().GetSecretOwner(domaintesting.IsAtomicContextChecker, uri).Return(domainsecret.Owner{Kind: domainsecret.ApplicationOwner, UUID: appUUID.String()}, nil)
	s.state.EXPECT().CheckApplicationSecretLabelExists(domaintesting.IsAtomicContextChecker, appUUID, "my secret").Return(false, nil)
	s.state.EXPECT().UpdateSecret(domaintesting.IsAtomicContextChecker, uri, gomock.Any()).DoAndReturn(func(_ domain.AtomicContext, _ *coresecrets.URI, got domainsecret.UpsertSecretParams) error {
		c.Assert(got.NextRotateTime, tc.NotNil)
		c.Assert(*got.NextRotateTime, tc.Almost, *p.NextRotateTime)
		got.NextRotateTime = nil
		want := p
		want.NextRotateTime = nil
		c.Assert(got, tc.DeepEquals, want)
		return nil
	})

	err = s.service.UpdateCharmSecret(c.Context(), uri, UpdateCharmSecretParams{
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
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rollbackCalled, tc.IsFalse)
}

func (s *serviceSuite) TestUpdateCharmSecretForAppOwnedFailedLabelExists(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()

	appUUID, err := coreapplication.NewID()
	c.Assert(err, tc.ErrorIsNil)

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

	s.state.EXPECT().GetSecretOwner(domaintesting.IsAtomicContextChecker, uri).Return(domainsecret.Owner{Kind: domainsecret.ApplicationOwner, UUID: appUUID.String()}, nil)
	s.state.EXPECT().CheckApplicationSecretLabelExists(domaintesting.IsAtomicContextChecker, appUUID, "my secret").Return(true, nil)

	err = s.service.UpdateCharmSecret(c.Context(), uri, UpdateCharmSecretParams{
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
	c.Assert(err, tc.ErrorIs, secreterrors.SecretLabelAlreadyExists)
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

	data, ref, err := s.service.GetSecretValue(c.Context(), uri, 666, SecretAccessor{
		Kind: UnitAccessor,
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
	consumer := &coresecrets.SecretConsumerMetadata{
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

	got, err := s.service.ListCharmSecretsToDrain(c.Context(), []CharmSecretOwner{{
		Kind: UnitOwner,
		ID:   "mariadb/0",
	}, {
		Kind: ApplicationOwner,
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

	owners := []CharmSecretOwner{{
		Kind: ApplicationOwner,
		ID:   "mysql",
	}, {
		Kind: UnitOwner,
		ID:   "mysql/0",
	}}
	md := []*coresecrets.SecretMetadata{{Label: "one"}}
	revs := [][]*coresecrets.SecretRevisionMetadata{{{Revision: 1}}}

	s.state.EXPECT().ListCharmSecrets(gomock.Any(), domainsecret.ApplicationOwners{"mysql"}, domainsecret.UnitOwners{"mysql/0"}).
		Return(md, revs, nil)

	gotSecrets, gotRevisions, err := s.service.ListCharmSecrets(c.Context(), owners...)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotSecrets, tc.DeepEquals, md)
	c.Assert(gotRevisions, tc.DeepEquals, revs)
}

func (s *serviceSuite) TestListCharmJustApplication(c *tc.C) {
	defer s.setupMocks(c).Finish()

	owners := []CharmSecretOwner{{
		Kind: ApplicationOwner,
		ID:   "mysql",
	}}
	md := []*coresecrets.SecretMetadata{{Label: "one"}}
	revs := [][]*coresecrets.SecretRevisionMetadata{{{Revision: 1}}}

	s.state.EXPECT().ListCharmSecrets(gomock.Any(), domainsecret.ApplicationOwners{"mysql"}, domainsecret.NilUnitOwners).
		Return(md, revs, nil)

	gotSecrets, gotRevisions, err := s.service.ListCharmSecrets(c.Context(), owners...)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotSecrets, tc.DeepEquals, md)
	c.Assert(gotRevisions, tc.DeepEquals, revs)
}

func (s *serviceSuite) TestListCharmJustUnit(c *tc.C) {
	defer s.setupMocks(c).Finish()

	owners := []CharmSecretOwner{{
		Kind: UnitOwner,
		ID:   "mysql/0",
	}}
	md := []*coresecrets.SecretMetadata{{Label: "one"}}
	revs := [][]*coresecrets.SecretRevisionMetadata{{{Revision: 1}}}

	s.state.EXPECT().ListCharmSecrets(gomock.Any(), domainsecret.NilApplicationOwners, domainsecret.UnitOwners{"mysql/0"}).
		Return(md, revs, nil)

	gotSecrets, gotRevisions, err := s.service.ListCharmSecrets(c.Context(), owners...)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotSecrets, tc.DeepEquals, md)
	c.Assert(gotRevisions, tc.DeepEquals, revs)
}

func (s *serviceSuite) TestGetURIByConsumerLabel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	s.state.EXPECT().GetURIByConsumerLabel(gomock.Any(), "my label", unittesting.GenNewName(c, "mysql/0")).Return(uri, nil)

	got, err := s.service.GetURIByConsumerLabel(c.Context(), "my label", "mysql/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.DeepEquals, uri)
}

func (s *serviceSuite) TestUpdateRemoteSecretRevision(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	s.state.EXPECT().UpdateRemoteSecretRevision(gomock.Any(), uri, 666).Return(nil)

	err := s.service.UpdateRemoteSecretRevision(c.Context(), uri, 666)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestUpdateRemoteConsumedRevision(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	s.state.EXPECT().GetSecretRemoteConsumer(gomock.Any(), uri, unittesting.GenNewName(c, "remote-app/0")).
		Return(&coresecrets.SecretConsumerMetadata{}, 666, nil)

	got, err := s.service.UpdateRemoteConsumedRevision(c.Context(), uri, unittesting.GenNewName(c, "remote-app/0"), false)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.Equals, 666)
}

func (s *serviceSuite) TestUpdateRemoteConsumedRevisionRefresh(c *tc.C) {
	defer s.setupMocks(c).Finish()

	consumer := &coresecrets.SecretConsumerMetadata{
		CurrentRevision: 666,
	}
	uri := coresecrets.NewURI()
	s.state.EXPECT().GetSecretRemoteConsumer(gomock.Any(), uri, unittesting.GenNewName(c, "remote-app/0")).
		Return(&coresecrets.SecretConsumerMetadata{}, 666, nil)
	s.state.EXPECT().SaveSecretRemoteConsumer(gomock.Any(), uri, unittesting.GenNewName(c, "remote-app/0"), consumer).Return(nil)

	got, err := s.service.UpdateRemoteConsumedRevision(c.Context(), uri, unittesting.GenNewName(c, "remote-app/0"), true)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.Equals, 666)
}

func (s *serviceSuite) TestUpdateRemoteConsumedRevisionFirstTimeRefresh(c *tc.C) {
	defer s.setupMocks(c).Finish()

	consumer := &coresecrets.SecretConsumerMetadata{
		CurrentRevision: 666,
	}
	uri := coresecrets.NewURI()
	s.state.EXPECT().GetSecretRemoteConsumer(gomock.Any(), uri, unittesting.GenNewName(c, "remote-app/0")).
		Return(nil, 666, secreterrors.SecretConsumerNotFound)
	s.state.EXPECT().SaveSecretRemoteConsumer(gomock.Any(), uri, unittesting.GenNewName(c, "remote-app/0"), consumer).Return(nil)

	got, err := s.service.UpdateRemoteConsumedRevision(c.Context(), uri, unittesting.GenNewName(c, "remote-app/0"), true)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(got, tc.Equals, 666)
}

func (s *serviceSuite) TestGrantSecretUnitAccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

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

	err := s.service.GrantSecretAccess(c.Context(), uri, SecretAccessParams{
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
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestGrantSecretApplicationAccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

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

	err := s.service.GrantSecretAccess(c.Context(), uri, SecretAccessParams{
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

	err := s.service.GrantSecretAccess(c.Context(), uri, SecretAccessParams{
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
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestGrantSecretRelationScope(c *tc.C) {
	defer s.setupMocks(c).Finish()

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

	err := s.service.GrantSecretAccess(c.Context(), uri, SecretAccessParams{
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
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestRevokeSecretUnitAccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
	}).Return("manage", nil)
	s.state.EXPECT().RevokeAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "another/0",
	}).Return(nil)

	err := s.service.RevokeSecretAccess(c.Context(), uri, SecretAccessParams{
		Accessor: SecretAccessor{
			Kind: UnitAccessor,
			ID:   "mysql/0",
		},
		Subject: SecretAccessor{
			Kind: UnitAccessor,
			ID:   "another/0",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestRevokeSecretApplicationAccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectUnit,
		SubjectID:     "mysql/0",
	}).Return("manage", nil)
	s.state.EXPECT().RevokeAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "another",
	}).Return(nil)

	err := s.service.RevokeSecretAccess(c.Context(), uri, SecretAccessParams{
		Accessor: SecretAccessor{
			Kind: UnitAccessor,
			ID:   "mysql/0",
		},
		Subject: SecretAccessor{
			Kind: ApplicationAccessor,
			ID:   "another",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *serviceSuite) TestRevokeSecretModelAccess(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	s.state.EXPECT().GetSecretAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectModel,
		SubjectID:     "model-uuid",
	}).Return("manage", nil)
	s.state.EXPECT().RevokeAccess(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
	}).Return(nil)

	err := s.service.RevokeSecretAccess(c.Context(), uri, SecretAccessParams{
		Accessor: SecretAccessor{
			Kind: ModelAccessor,
			ID:   "model-uuid",
		},
		Subject: SecretAccessor{
			Kind: ApplicationAccessor,
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

	role, err := s.service.getSecretAccess(c.Context(), uri, SecretAccessor{
		Kind: ApplicationAccessor,
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

	role, err := s.service.getSecretAccess(c.Context(), uri, SecretAccessor{
		Kind: ApplicationAccessor,
		ID:   "mysql",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(role, tc.Equals, coresecrets.RoleNone)
}

func (s *serviceSuite) TestGetSecretAccessApplicationScope(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	s.state.EXPECT().GetSecretAccessScope(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
	}).Return(&domainsecret.AccessScope{
		ScopeTypeID: domainsecret.ScopeApplication,
		ScopeID:     "mysql",
	}, nil)

	scope, err := s.service.GetSecretAccessScope(c.Context(), uri, SecretAccessor{
		Kind: ApplicationAccessor,
		ID:   "mysql",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(scope, tc.DeepEquals, SecretAccessScope{
		Kind: ApplicationAccessScope,
		ID:   "mysql",
	})
}

func (s *serviceSuite) TestGetSecretAccessRelationScope(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	s.state.EXPECT().GetSecretAccessScope(gomock.Any(), uri, domainsecret.AccessParams{
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
	}).Return(&domainsecret.AccessScope{
		ScopeTypeID: domainsecret.ScopeRelation,
		ScopeID:     "mysql:db mediawiki:db",
	}, nil)

	scope, err := s.service.GetSecretAccessScope(c.Context(), uri, SecretAccessor{
		Kind: ApplicationAccessor,
		ID:   "mysql",
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(scope, tc.DeepEquals, SecretAccessScope{
		Kind: RelationAccessScope,
		ID:   "mysql:db mediawiki:db",
	})
}

func (s *serviceSuite) TestGetSecretGrants(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()
	s.state.EXPECT().GetSecretGrants(gomock.Any(), uri, coresecrets.RoleView).Return([]domainsecret.GrantParams{{
		ScopeTypeID:   domainsecret.ScopeModel,
		ScopeID:       "model-uuid",
		SubjectTypeID: domainsecret.SubjectApplication,
		SubjectID:     "mysql",
		RoleID:        domainsecret.RoleView,
	}}, nil)

	g, err := s.service.GetSecretGrants(c.Context(), uri, coresecrets.RoleView)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(g, tc.DeepEquals, []SecretAccess{{
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
		Accessor: SecretAccessor{
			Kind: UnitAccessor,
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
		Accessor: SecretAccessor{
			Kind: UnitAccessor,
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
		Accessor: SecretAccessor{
			Kind: UnitAccessor,
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
		Accessor: SecretAccessor{
			Kind: UnitAccessor,
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
		Accessor: SecretAccessor{
			Kind: UnitAccessor,
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
		Accessor: SecretAccessor{
			Kind: UnitAccessor,
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
		Accessor: SecretAccessor{
			Kind: UnitAccessor,
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
		Accessor: SecretAccessor{
			Kind: UnitAccessor,
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
		Accessor: SecretAccessor{
			Kind: UnitAccessor,
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
	s.state.EXPECT().SaveSecretConsumer(gomock.Any(), uri, unittesting.GenNewName(c, "mariadb/0"), &coresecrets.SecretConsumerMetadata{
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
	s.state.EXPECT().SaveSecretConsumer(gomock.Any(), uri, unittesting.GenNewName(c, "mariadb/0"), &coresecrets.SecretConsumerMetadata{
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
	s.state.EXPECT().SaveSecretConsumer(gomock.Any(), uri, unittesting.GenNewName(c, "mariadb/0"), &coresecrets.SecretConsumerMetadata{
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
	s.state.EXPECT().SaveSecretConsumer(gomock.Any(), uri, unittesting.GenNewName(c, "mariadb/0"), &coresecrets.SecretConsumerMetadata{
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
	s.state.EXPECT().SaveSecretConsumer(gomock.Any(), uri, unittesting.GenNewName(c, "mariadb/0"), &coresecrets.SecretConsumerMetadata{
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

	rev, err := s.service.GetConsumedRevision(c.Context(), uri, "mariadb/0", true, false, nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rev, tc.Equals, 668)
}

func (s *serviceSuite) TestProcessCharmSecretConsumerLabelForUnitOwnedSecretUpdateLabel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	uri := coresecrets.NewURI()

	unitUUID, err := coreunit.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

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

	s.state.EXPECT().GetSecretOwner(domaintesting.IsAtomicContextChecker, uri).Return(
		domainsecret.Owner{Kind: domainsecret.UnitOwner, UUID: unitUUID.String()}, nil,
	)
	s.state.EXPECT().CheckUnitSecretLabelExists(domaintesting.IsAtomicContextChecker, unitUUID, "foo").Return(false, nil)
	s.state.EXPECT().UpdateSecret(domaintesting.IsAtomicContextChecker, uri, domainsecret.UpsertSecretParams{
		RotatePolicy: ptr(domainsecret.RotateNever),
		Label:        ptr("foo"),
	}).Return(nil)

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

	gotURI, gotLabel, err := s.service.ProcessCharmSecretConsumerLabel(c.Context(), "mariadb/0", uri, "foo")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(gotURI, tc.DeepEquals, uri)
	c.Assert(gotLabel, tc.DeepEquals, ptr("foo"))
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
// only secret change will be sent if the secret has been removed.
func (s *serviceSuite) TestWatchObsoleteMapperSendObsoleteRevisionAndRemovedURIs(c *tc.C) {
	ctrl := s.setupMocks(c)
	defer ctrl.Finish()

	appOwners := domainsecret.ApplicationOwners([]string{"mysql"})
	unitOwners := domainsecret.UnitOwners([]string{"mysql/0", "mysql/1"})

	ownedURI := coresecrets.NewURI()
	removedOwnedURI := coresecrets.NewURI()
	notOwnedURI := coresecrets.NewURI()

	s.state.EXPECT().GetRevisionIDsForObsolete(gomock.Any(),
		appOwners, unitOwners,
		"revision-uuid-3",
		"revision-uuid-1",
		"revision-uuid-2",
	).Return(
		map[string]string{
			"revision-uuid-1": ownedURI.ID + "/1",
			"revision-uuid-3": ownedURI.ID + "/3",
		}, nil,
	)

	gomock.InOrder(
		// When we receive the initial event, the removedOwnedURI is not removed yet.
		s.state.EXPECT().GetOwnedSecretIDs(gomock.Any(), appOwners, unitOwners).Return(
			[]string{ownedURI.ID, removedOwnedURI.ID}, nil,
		),

		// When we receive the event 2nd time, the removedOwnedURI is removed.
		s.state.EXPECT().GetOwnedSecretIDs(gomock.Any(), appOwners, unitOwners).Return(
			[]string{ownedURI.ID}, nil,
		),
	)

	mapper := obsoleteWatcherMapperFunc(
		loggertesting.WrapCheckLog(c),
		s.state,
		appOwners, unitOwners,
		"secret_metadata", "secret_revision_obsolete",
	)

	result, err := mapper(
		c.Context(),
		[]changestream.ChangeEvent{
			// The initial events.
			newSecretChangeEvent(ownedURI.ID),
			newSecretChangeEvent(removedOwnedURI.ID),
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 0)

	result, err = mapper(
		c.Context(),
		[]changestream.ChangeEvent{
			// Owned obsolete revision events will be sent in order.
			newObsoleteRevisionChangeEvent("revision-uuid-3"),
			newObsoleteRevisionChangeEvent("revision-uuid-1"),

			// Not owned obsolete revision will be ignored.
			newObsoleteRevisionChangeEvent("revision-uuid-2"),

			// Deletion events of the secretWatcher are sent.
			newSecretChangeEvent(removedOwnedURI.ID),
			newSecretChangeEvent(notOwnedURI.ID), // not owned by the given owners will be ignored.
		},
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 3)
	revisionChange3 := result[0]
	revisionChange1 := result[1]
	c.Assert(revisionChange3, tc.Equals, ownedURI.ID+"/3")
	c.Assert(revisionChange1, tc.Equals, ownedURI.ID+"/1")

	secretChange := result[2]
	c.Assert(secretChange, tc.Equals, removedOwnedURI.ID)
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
		appOwners, unitOwners,
		"revision-uuid-3",
		"revision-uuid-2",
		"revision-uuid-1",
		"revision-uuid-4",
	).Return(
		map[string]string{
			"revision-uuid-1": ownedURI.ID + "/1",
			"revision-uuid-2": ownedURI.ID + "/2",
			"revision-uuid-3": ownedURI.ID + "/3",
		}, nil,
	)

	mapper := obsoleteWatcherMapperFunc(
		loggertesting.WrapCheckLog(c),
		s.state,
		appOwners, unitOwners,
		"secret_metadata", "secret_revision_obsolete",
	)
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
	c.Assert(result, tc.HasLen, 3)
	revisionChange3 := result[0]
	revisionChange2 := result[1]
	revisionChange1 := result[2]
	c.Assert(revisionChange3, tc.Equals, ownedURI.ID+"/3")
	c.Assert(revisionChange2, tc.Equals, ownedURI.ID+"/2")
	c.Assert(revisionChange1, tc.Equals, ownedURI.ID+"/1")
}

// TestWatchObsoleteMapperSendRemovedURIs tests the behavior of the mapper function
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

	mapper := obsoleteWatcherMapperFunc(
		loggertesting.WrapCheckLog(c),
		s.state,
		appOwners, unitOwners,
		"secret_metadata", "secret_revision_obsolete",
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
			c.Assert(secretFilter.Namespace(), tc.Equals, "secret_metadata")
			c.Assert(secretFilter.ChangeMask(), tc.Equals, changestream.All)

			c.Assert(filters, tc.HasLen, 1)
			obsoleteRevisionFilter := filters[0]
			c.Assert(obsoleteRevisionFilter.Namespace(), tc.Equals, "secret_revision_obsolete")
			c.Assert(obsoleteRevisionFilter.ChangeMask(), tc.Equals, changestream.Changed)
			return NewMockStringsWatcher(ctrl), nil
		},
	)

	var namespaceQuery = func(context.Context, database.TxnRunner) ([]string, error) {
		return []string{}, nil
	}
	s.state.EXPECT().InitialWatchStatementForObsoleteRevision(
		domainsecret.ApplicationOwners([]string{"mysql"}),
		domainsecret.UnitOwners([]string{"mysql/0", "mysql/1"}),
	).Return("secret_revision_obsolete", namespaceQuery)
	s.state.EXPECT().InitialWatchStatementForOwnedSecrets(
		domainsecret.ApplicationOwners([]string{"mysql"}),
		domainsecret.UnitOwners([]string{"mysql/0", "mysql/1"}),
	).Return("secret_metadata", namespaceQuery)

	svc := NewWatchableService(
		s.state, s.secretBackendState, s.ensurer, mockWatcherFactory, loggertesting.WrapCheckLog(c))
	w, err := svc.WatchObsolete(c.Context(),
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
	s.state.EXPECT().InitialWatchStatementForConsumedSecretsChange(unittesting.GenNewName(c, "mysql/0")).Return("secret_revision", namespaceQuery)
	s.state.EXPECT().InitialWatchStatementForConsumedRemoteSecretsChange(unittesting.GenNewName(c, "mysql/0")).Return("secret_reference", namespaceQuery)
	mockWatcherFactory.EXPECT().NewNamespaceWatcher(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(mockStringWatcher, nil)
	mockWatcherFactory.EXPECT().NewNamespaceWatcher(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(mockStringWatcherRemote, nil)

	s.state.EXPECT().GetConsumedSecretURIsWithChanges(gomock.Any(),
		unittesting.GenNewName(c, "mysql/0"), "revision-uuid-1",
	).Return([]string{uri1.String()}, nil)
	s.state.EXPECT().GetConsumedRemoteSecretURIsWithChanges(gomock.Any(),
		unittesting.GenNewName(c, "mysql/0"), "revision-uuid-2",
	).Return([]string{uri2.String()}, nil)

	svc := NewWatchableService(
		s.state, s.secretBackendState, s.ensurer, mockWatcherFactory, loggertesting.WrapCheckLog(c))
	w, err := svc.WatchConsumedSecretsChanges(c.Context(), "mysql/0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(w, tc.NotNil)
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

func (s *serviceSuite) TestWatchRemoteConsumedSecretsChanges(c *tc.C) {
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
	mockWatcherFactory.EXPECT().NewNamespaceWatcher(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(mockStringWatcher, nil)

	s.state.EXPECT().GetRemoteConsumedSecretURIsWithChangesFromOfferingSide(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, appName string, secretIDs ...string) ([]string, error) {
		c.Assert(appName, tc.Equals, "mysql")
		c.Assert(secretIDs, tc.SameContents, []string{"revision-uuid-1", "revision-uuid-2"})
		return []string{uri1.String(), uri2.String()}, nil
	})

	svc := NewWatchableService(
		s.state, s.secretBackendState, s.ensurer, mockWatcherFactory, loggertesting.WrapCheckLog(c))
	w, err := svc.WatchRemoteConsumedSecretsChanges(c.Context(), "mysql")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(w, tc.NotNil)
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
