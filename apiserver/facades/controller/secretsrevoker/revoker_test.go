// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsrevoker

import (
	"time"

	"github.com/google/uuid"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/rpc/params"
	secretsprovider "github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher/watchertest"
)

type facadeSuite struct {
	testing.IsolationSuite

	authorizer *facademocks.MockAuthorizer
	resources  *facademocks.MockResources
	state      *MockSecretsState
	getters    *MockGetters
	provider   *MockSecretBackendProvider
	facade     *SecretsRevokerAPI
}

var _ = gc.Suite(&facadeSuite{})

func (s *facadeSuite) setup(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.authorizer = facademocks.NewMockAuthorizer(ctrl)
	s.resources = facademocks.NewMockResources(ctrl)
	s.state = NewMockSecretsState(ctrl)
	s.getters = NewMockGetters(ctrl)
	s.provider = NewMockSecretBackendProvider(ctrl)

	s.facade = &SecretsRevokerAPI{
		state:     s.state,
		resources: s.resources,

		backendConfigGetter: s.getters.BackendConfigGetter,
		providerGetter:      s.getters.ProviderGetter,
	}

	s.authorizer.EXPECT().AuthController().Return(true).AnyTimes()
	s.getters.EXPECT().ProviderGetter(
		"my-backend-type").Return(s.provider, nil).AnyTimes()

	return ctrl
}

func (s *facadeSuite) TestRevokeIssuedTokens(c *gc.C) {
	defer s.setup(c).Finish()

	now := time.Now()
	next := now.Add(time.Hour)
	uuids := []string{uuid.NewString(), uuid.NewString()}
	tokens := []state.SecretBackendIssuedToken{{
		UUID:       uuids[0],
		ExpireTime: now.Add(-time.Second),
		BackendID:  "some-backend",
		Consumer:   names.NewUnitTag("app/0"),
	}, {
		UUID:       uuids[1],
		ExpireTime: now.Add(-time.Second),
		BackendID:  "some-backend",
		Consumer:   names.NewUnitTag("app/0"),
	}}
	s.state.EXPECT().ListSecretBackendIssuedTokenUntil(now).Return(tokens, nil)
	s.state.EXPECT().NextSecretBackendIssuedTokenExpiry().Return(next, nil)

	backends := &secretsprovider.ModelBackendConfigInfo{
		ActiveID: "some-backend",
		Configs: map[string]secretsprovider.ModelBackendConfig{
			"some-backend": {
				BackendConfig: secretsprovider.BackendConfig{
					BackendType: "my-backend-type",
				},
			},
		},
	}
	s.getters.EXPECT().BackendConfigGetter().Return(backends, nil)

	s.provider.EXPECT().CleanupIssuedTokens(
		gomock.Any(), uuids).Return(uuids[:1], nil)
	s.state.EXPECT().RemoveSecretBackendIssuedTokens(uuids[:1]).Return(nil)

	res, err := s.facade.RevokeIssuedTokens(now)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.RevokeIssuedTokensResult{
		Next: next,
	})
}

func (s *facadeSuite) TestWatchIssuedTokenExpiry(c *gc.C) {
	defer s.setup(c).Finish()

	ch := make(chan []string, 1)
	ch <- []string{"something"}
	w := watchertest.NewStringsWatcher(ch)
	defer w.Kill()

	s.state.EXPECT().WatchSecretBackendIssuedTokenExpiry().Return(w)
	s.resources.EXPECT().Register(w).Return("abc")

	res, err := s.facade.WatchIssuedTokenExpiry()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.DeepEquals, params.StringsWatchResult{
		StringsWatcherId: "abc",
		Changes:          []string{"something"},
	})
}
