// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets_test

import (
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common/secrets"
	"github.com/juju/juju/apiserver/common/secrets/mocks"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/core/model"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher/watchertest"
	secretservice "github.com/juju/juju/domain/secret/service"
	backendservice "github.com/juju/juju/domain/secretbackend/service"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type secretsDrainSuite struct {
	testhelpers.IsolationSuite

	authorizer      *facademocks.MockAuthorizer
	watcherRegistry *facademocks.MockWatcherRegistry

	leadership           *mocks.MockChecker
	token                *mocks.MockToken
	secretService        *mocks.MockSecretService
	secretBackendService *mocks.MockSecretBackendService

	authTag names.Tag

	facade *secrets.SecretsDrainAPI
}

var _ = tc.Suite(&secretsDrainSuite{})

func (s *secretsDrainSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.authTag = names.NewUnitTag("mariadb/0")
}

func (s *secretsDrainSuite) setup(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)

	s.leadership = mocks.NewMockChecker(ctrl)
	s.token = mocks.NewMockToken(ctrl)
	s.secretService = mocks.NewMockSecretService(ctrl)
	s.secretBackendService = mocks.NewMockSecretBackendService(ctrl)
	s.expectAuthUnitAgent()

	var err error
	s.facade, err = secrets.NewSecretsDrainAPI(
		s.authTag,
		s.authorizer,
		loggertesting.WrapCheckLog(c),
		s.leadership,
		model.UUID(coretesting.ModelTag.Id()),
		s.secretService,
		s.secretBackendService,
		s.watcherRegistry,
	)
	c.Assert(err, tc.ErrorIsNil)
	return ctrl
}

func (s *secretsDrainSuite) expectAuthUnitAgent() {
	s.authorizer.EXPECT().AuthUnitAgent().Return(true)
}

func (s *secretsDrainSuite) assertGetSecretsToDrain(c *tc.C, expectedRevions ...params.SecretRevision) {
	defer s.setup(c).Finish()

	s.leadership.EXPECT().LeadershipCheck("mariadb", "mariadb/0").Return(s.token)
	s.token.EXPECT().Check().Return(nil)

	uri := coresecrets.NewURI()
	revisions := []coresecrets.SecretExternalRevision{
		{
			// External backend.
			Revision: 666,
			ValueRef: &coresecrets.ValueRef{
				BackendID:  "backend-id",
				RevisionID: "rev-666",
			},
		}, {
			// Internal backend.
			Revision: 667,
		},
		{
			// k8s backend.
			Revision: 668,
			ValueRef: &coresecrets.ValueRef{
				BackendID:  coretesting.ModelTag.Id(),
				RevisionID: "rev-668",
			},
		},
	}
	s.secretService.EXPECT().ListCharmSecretsToDrain(
		gomock.Any(),
		[]secretservice.CharmSecretOwner{{
			Kind: secretservice.UnitOwner,
			ID:   "mariadb/0",
		}, {
			Kind: secretservice.ApplicationOwner,
			ID:   "mariadb",
		}}).Return([]*coresecrets.SecretMetadataForDrain{{
		URI:       uri,
		Revisions: revisions,
	}}, nil)
	revInfo := make([]backendservice.RevisionInfo, len(expectedRevions))
	for i, r := range expectedRevions {
		revInfo[i] = backendservice.RevisionInfo{
			Revision: r.Revision,
		}
		if r.ValueRef != nil {
			revInfo[i].ValueRef = &coresecrets.ValueRef{
				BackendID:  r.ValueRef.BackendID,
				RevisionID: r.ValueRef.RevisionID,
			}
		}
	}
	s.secretBackendService.EXPECT().GetRevisionsToDrain(gomock.Any(), model.UUID(coretesting.ModelTag.Id()), revisions).
		Return(revInfo, nil)

	results, err := s.facade.GetSecretsToDrain(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.SecretRevisionsToDrainResults{
		Results: []params.SecretRevisionsToDrainResult{{
			URI:       uri.String(),
			Revisions: expectedRevions,
		}},
	})
}

func (s *secretsDrainSuite) TestGetSecretsToDrainInternal(c *tc.C) {
	s.assertGetSecretsToDrain(c,
		// External backend.
		params.SecretRevision{
			Revision: 666,
			ValueRef: &params.SecretValueRef{
				BackendID:  "backend-id",
				RevisionID: "rev-666",
			},
		},
		// k8s backend.
		params.SecretRevision{
			Revision: 668,
			ValueRef: &params.SecretValueRef{
				BackendID:  coretesting.ModelTag.Id(),
				RevisionID: "rev-668",
			},
		},
	)
}

func (s *secretsDrainSuite) TestGetSecretsToDrainExternal(c *tc.C) {
	s.assertGetSecretsToDrain(c,
		// Internal backend.
		params.SecretRevision{
			Revision: 667,
		},
		// k8s backend.
		params.SecretRevision{
			Revision: 668,
			ValueRef: &params.SecretValueRef{
				BackendID:  coretesting.ModelTag.Id(),
				RevisionID: "rev-668",
			},
		},
	)
}

func (s *secretsDrainSuite) TestGetUserSecretsToDrain(c *tc.C) {
	s.authTag = names.NewModelTag(coretesting.ModelTag.Id())

	defer s.setup(c).Finish()

	uri := coresecrets.NewURI()
	revisions := []coresecrets.SecretExternalRevision{
		{
			// External backend.
			Revision: 666,
			ValueRef: &coresecrets.ValueRef{
				BackendID:  "backend-id",
				RevisionID: "rev-666",
			},
		}, {
			// Internal backend.
			Revision: 667,
		},
		{
			// k8s backend.
			Revision: 668,
			ValueRef: &coresecrets.ValueRef{
				BackendID:  coretesting.ModelTag.Id(),
				RevisionID: "rev-668",
			},
		},
	}
	expectedRevions := []params.SecretRevision{{
		Revision: 667,
	},
		// k8s backend.
		{
			Revision: 668,
			ValueRef: &params.SecretValueRef{
				BackendID:  coretesting.ModelTag.Id(),
				RevisionID: "rev-668",
			},
		},
	}
	s.secretService.EXPECT().ListUserSecretsToDrain(gomock.Any()).Return([]*coresecrets.SecretMetadataForDrain{{
		URI:       uri,
		Revisions: revisions,
	}}, nil)
	revInfo := make([]backendservice.RevisionInfo, len(expectedRevions))
	for i, r := range expectedRevions {
		revInfo[i] = backendservice.RevisionInfo{
			Revision: r.Revision,
		}
		if r.ValueRef != nil {
			revInfo[i].ValueRef = &coresecrets.ValueRef{
				BackendID:  r.ValueRef.BackendID,
				RevisionID: r.ValueRef.RevisionID,
			}
		}
	}
	s.secretBackendService.EXPECT().GetRevisionsToDrain(gomock.Any(), model.UUID(coretesting.ModelTag.Id()), revisions).
		Return(revInfo, nil)

	results, err := s.facade.GetSecretsToDrain(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results, tc.DeepEquals, params.SecretRevisionsToDrainResults{
		Results: []params.SecretRevisionsToDrainResult{{
			URI:       uri.String(),
			Revisions: expectedRevions,
		}},
	})
}

func (s *secretsDrainSuite) TestChangeSecretBackend(c *tc.C) {
	defer s.setup(c).Finish()

	uri1 := coresecrets.NewURI()
	uri2 := coresecrets.NewURI()
	s.secretService.EXPECT().ChangeSecretBackend(
		gomock.Any(),
		uri1, 666,
		secretservice.ChangeSecretBackendParams{
			Accessor: secretservice.SecretAccessor{
				Kind: secretservice.UnitAccessor,
				ID:   s.authTag.Id(),
			},
			ValueRef: &coresecrets.ValueRef{
				BackendID:  "backend-id",
				RevisionID: "rev-666",
			},
		},
	).Return(nil)
	s.secretService.EXPECT().ChangeSecretBackend(
		gomock.Any(),
		uri2, 888,
		secretservice.ChangeSecretBackendParams{
			Accessor: secretservice.SecretAccessor{
				Kind: secretservice.UnitAccessor,
				ID:   s.authTag.Id(),
			},
			Data: map[string]string{"foo": "bar"},
		},
	).Return(nil)

	result, err := s.facade.ChangeSecretBackend(c.Context(), params.ChangeSecretBackendArgs{
		Args: []params.ChangeSecretBackendArg{
			{
				URI:      uri1.String(),
				Revision: 666,
				Content: params.SecretContentParams{
					// Change to external backend.
					ValueRef: &params.SecretValueRef{
						BackendID:  "backend-id",
						RevisionID: "rev-666",
					},
				},
			},
			{
				URI:      uri2.String(),
				Revision: 888,
				Content: params.SecretContentParams{
					// Change to internal backend.
					Data: map[string]string{"foo": "bar"},
				},
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{{Error: nil}, {Error: nil}},
	})
}

func (s *secretsDrainSuite) TestWatchSecretBackendChanged(c *tc.C) {
	defer s.setup(c).Finish()

	changeChan := make(chan struct{}, 1)
	changeChan <- struct{}{}
	w := watchertest.NewMockNotifyWatcher(changeChan)
	s.secretBackendService.EXPECT().WatchModelSecretBackendChanged(gomock.Any(), model.UUID(coretesting.ModelTag.Id())).Return(w, nil)

	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("11", nil)

	result, err := s.facade.WatchSecretBackendChanged(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.NotifyWatchResult{
		NotifyWatcherId: "11",
	})
}
