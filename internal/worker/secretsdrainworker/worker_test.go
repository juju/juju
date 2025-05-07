// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsdrainworker_test

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/testing"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/api/common/secretsdrain"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/logger"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher/watchertest"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	jujusecrets "github.com/juju/juju/internal/secrets"
	"github.com/juju/juju/internal/secrets/provider"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/secretsdrainworker"
	"github.com/juju/juju/internal/worker/secretsdrainworker/mocks"
)

type workerSuite struct {
	testing.IsolationSuite

	logger logger.Logger

	facade            *mocks.MockSecretsDrainFacade
	backendClient     *mocks.MockBackendsClient
	leadershipTracker *mocks.MockTrackerWorker

	done                   chan struct{}
	notifyBackendChangedCh chan struct{}
}

var _ = tc.Suite(&workerSuite{})

func (s *workerSuite) getWorkerNewer(c *tc.C) (func(string), *gomock.Controller) {
	ctrl := gomock.NewController(c)
	s.logger = loggertesting.WrapCheckLog(c)
	s.facade = mocks.NewMockSecretsDrainFacade(ctrl)
	s.backendClient = mocks.NewMockBackendsClient(ctrl)
	s.leadershipTracker = mocks.NewMockTrackerWorker(ctrl)

	s.notifyBackendChangedCh = make(chan struct{}, 1)
	s.done = make(chan struct{})
	s.facade.EXPECT().WatchSecretBackendChanged(gomock.Any()).Return(watchertest.NewMockNotifyWatcher(s.notifyBackendChangedCh), nil)

	start := func(expectedErr string) {
		w, err := secretsdrainworker.NewWorker(secretsdrainworker.Config{
			Logger:             s.logger,
			SecretsDrainFacade: s.facade,
			SecretsBackendGetter: func() (jujusecrets.BackendsClient, error) {
				return s.backendClient, nil
			},
			LeadershipTrackerFunc: func() leadership.ChangeTracker { return s.leadershipTracker },
		})
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(w, tc.NotNil)
		s.AddCleanup(func(c *tc.C) {
			if expectedErr == "" {
				workertest.CleanKill(c, w)
			} else {
				err := workertest.CheckKilled(c, w)
				c.Assert(err, tc.ErrorMatches, expectedErr)
			}
		})
		s.waitDone(c)
	}
	return start, ctrl
}

func (s *workerSuite) waitDone(c *tc.C) {
	select {
	case <-s.done:
	case <-time.After(coretesting.ShortWait):
		c.Errorf("timed out waiting for worker")
	}
}

func (s *workerSuite) TestNothingToDrain(c *tc.C) {
	start, ctrl := s.getWorkerNewer(c)
	defer ctrl.Finish()

	s.leadershipTracker.EXPECT().WithStableLeadership(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
			return fn(ctx)
		})

	s.notifyBackendChangedCh <- struct{}{}
	gomock.InOrder(
		s.facade.EXPECT().GetSecretsToDrain(gomock.Any()).DoAndReturn(func(context.Context) ([]coresecrets.SecretMetadataForDrain, error) {
			close(s.done)
			return []coresecrets.SecretMetadataForDrain{}, nil
		}),
	)
	start("")
}

func (s *workerSuite) TestDrainNoOPS(c *tc.C) {
	start, ctrl := s.getWorkerNewer(c)
	defer ctrl.Finish()

	s.leadershipTracker.EXPECT().WithStableLeadership(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
			return fn(ctx)
		})

	uri := coresecrets.NewURI()
	s.notifyBackendChangedCh <- struct{}{}
	gomock.InOrder(
		s.facade.EXPECT().GetSecretsToDrain(gomock.Any()).Return([]coresecrets.SecretMetadataForDrain{
			{
				URI: uri,
				Revisions: []coresecrets.SecretExternalRevision{
					{
						Revision: 1,
						ValueRef: &coresecrets.ValueRef{BackendID: "backend-1", RevisionID: "revision-1"},
					},
				},
			},
		}, nil),
		s.backendClient.EXPECT().GetBackend(gomock.Any(), nil, true).DoAndReturn(func(context.Context, *string, bool) (provider.SecretsBackend, string, error) {
			close(s.done)
			return nil, "backend-1", nil
		}),
	)
	start("")
}

func (s *workerSuite) TestDrainBetweenExternalBackends(c *tc.C) {
	start, ctrl := s.getWorkerNewer(c)
	defer ctrl.Finish()

	s.leadershipTracker.EXPECT().WithStableLeadership(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
			return fn(ctx)
		})

	uri := coresecrets.NewURI()
	s.notifyBackendChangedCh <- struct{}{}
	secretValue := coresecrets.NewSecretValue(map[string]string{"foo": "bar"})

	oldBackend := mocks.NewMockSecretsBackend(ctrl)
	activeBackend := mocks.NewMockSecretsBackend(ctrl)

	gomock.InOrder(
		s.facade.EXPECT().GetSecretsToDrain(gomock.Any()).Return([]coresecrets.SecretMetadataForDrain{
			{
				URI: uri,
				Revisions: []coresecrets.SecretExternalRevision{
					{
						Revision: 1,
						ValueRef: &coresecrets.ValueRef{BackendID: "backend-1", RevisionID: "revision-1"},
					},
				},
			},
		}, nil),
		s.backendClient.EXPECT().GetBackend(gomock.Any(), nil, true).Return(activeBackend, "backend-2", nil),
		s.backendClient.EXPECT().GetRevisionContent(gomock.Any(), uri, 1).Return(secretValue, nil),
		activeBackend.EXPECT().SaveContent(gomock.Any(), uri, 1, secretValue).Return("revision-1", nil),
		s.backendClient.EXPECT().GetBackend(gomock.Any(), ptr("backend-1"), true).Return(oldBackend, "", nil),
		s.facade.EXPECT().ChangeSecretBackend(
			gomock.Any(),
			[]secretsdrain.ChangeSecretBackendArg{
				{
					URI:      uri,
					Revision: 1,
					ValueRef: &coresecrets.ValueRef{
						BackendID:  "backend-2",
						RevisionID: "revision-1",
					},
				},
			},
		).Return(secretsdrain.ChangeSecretBackendResult{Results: []error{nil}}, nil),
		oldBackend.EXPECT().DeleteContent(gomock.Any(), "revision-1").DoAndReturn(func(_ context.Context, _ string) error {
			close(s.done)
			return nil
		}),
	)
	start("")
}

func (s *workerSuite) TestDrainFromInternalToExternal(c *tc.C) {
	start, ctrl := s.getWorkerNewer(c)
	defer ctrl.Finish()

	s.leadershipTracker.EXPECT().WithStableLeadership(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
			return fn(ctx)
		})

	uri := coresecrets.NewURI()
	s.notifyBackendChangedCh <- struct{}{}
	secretValue := coresecrets.NewSecretValue(map[string]string{"foo": "bar"})

	activeBackend := mocks.NewMockSecretsBackend(ctrl)

	gomock.InOrder(
		s.facade.EXPECT().GetSecretsToDrain(gomock.Any()).Return([]coresecrets.SecretMetadataForDrain{
			{
				URI:       uri,
				Revisions: []coresecrets.SecretExternalRevision{{Revision: 1}},
			},
		}, nil),
		s.backendClient.EXPECT().GetBackend(gomock.Any(), nil, true).Return(activeBackend, "backend-2", nil),
		s.backendClient.EXPECT().GetRevisionContent(gomock.Any(), uri, 1).Return(secretValue, nil),
		activeBackend.EXPECT().SaveContent(gomock.Any(), uri, 1, secretValue).Return("revision-1", nil),
		s.facade.EXPECT().ChangeSecretBackend(
			gomock.Any(),
			[]secretsdrain.ChangeSecretBackendArg{
				{
					URI:      uri,
					Revision: 1,
					ValueRef: &coresecrets.ValueRef{
						BackendID:  "backend-2",
						RevisionID: "revision-1",
					},
				},
			},
		).DoAndReturn(func(context.Context, []secretsdrain.ChangeSecretBackendArg) (secretsdrain.ChangeSecretBackendResult, error) {
			close(s.done)
			return secretsdrain.ChangeSecretBackendResult{Results: []error{nil}}, nil
		}),
	)
	start("")
}

func (s *workerSuite) TestDrainFromExternalToInternal(c *tc.C) {
	start, ctrl := s.getWorkerNewer(c)
	defer ctrl.Finish()

	s.leadershipTracker.EXPECT().WithStableLeadership(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
			return fn(ctx)
		})

	uri := coresecrets.NewURI()
	s.notifyBackendChangedCh <- struct{}{}
	secretValue := coresecrets.NewSecretValue(map[string]string{"foo": "bar"})

	oldBackend := mocks.NewMockSecretsBackend(ctrl)
	activeBackend := mocks.NewMockSecretsBackend(ctrl)
	gomock.InOrder(
		s.facade.EXPECT().GetSecretsToDrain(gomock.Any()).Return([]coresecrets.SecretMetadataForDrain{
			{
				URI: uri,
				Revisions: []coresecrets.SecretExternalRevision{
					{
						Revision: 1,
						ValueRef: &coresecrets.ValueRef{BackendID: "backend-1", RevisionID: "revision-1"},
					},
				},
			},
		}, nil),
		s.backendClient.EXPECT().GetBackend(gomock.Any(), nil, true).Return(activeBackend, "backend-2", nil),
		s.backendClient.EXPECT().GetRevisionContent(gomock.Any(), uri, 1).Return(secretValue, nil),
		activeBackend.EXPECT().SaveContent(gomock.Any(), uri, 1, secretValue).Return("", errors.NotSupportedf("")),
		s.backendClient.EXPECT().GetBackend(gomock.Any(), ptr("backend-1"), true).Return(oldBackend, "", nil),
		s.facade.EXPECT().ChangeSecretBackend(
			gomock.Any(),
			[]secretsdrain.ChangeSecretBackendArg{
				{
					URI:      uri,
					Revision: 1,
					Data:     secretValue.EncodedValues(),
				},
			},
		).Return(secretsdrain.ChangeSecretBackendResult{Results: []error{nil}}, nil),
		oldBackend.EXPECT().DeleteContent(gomock.Any(), "revision-1").DoAndReturn(func(_ context.Context, _ string) error {
			close(s.done)
			return nil
		}),
	)
	start("")
}

func (s *workerSuite) TestDrainPartiallyFailed(c *tc.C) {
	// If the drain fails for one revision, it should continue to drain the rest.
	// But the agent should be restarted to retry.
	start, ctrl := s.getWorkerNewer(c)
	defer ctrl.Finish()

	s.leadershipTracker.EXPECT().WithStableLeadership(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
			return fn(ctx)
		})

	uri := coresecrets.NewURI()
	s.notifyBackendChangedCh <- struct{}{}
	secretValue := coresecrets.NewSecretValue(map[string]string{"foo": "bar"})

	oldBackend := mocks.NewMockSecretsBackend(ctrl)
	activeBackend := mocks.NewMockSecretsBackend(ctrl)
	gomock.InOrder(
		s.facade.EXPECT().GetSecretsToDrain(gomock.Any()).Return([]coresecrets.SecretMetadataForDrain{
			{
				URI: uri,
				Revisions: []coresecrets.SecretExternalRevision{
					{
						Revision: 1,
						ValueRef: &coresecrets.ValueRef{BackendID: "backend-1", RevisionID: "revision-1"},
					},
					{
						Revision: 2,
						ValueRef: &coresecrets.ValueRef{BackendID: "backend-2", RevisionID: "revision-2"},
					},
				},
			},
		}, nil),

		// revision 1
		s.backendClient.EXPECT().GetBackend(gomock.Any(), nil, true).Return(activeBackend, "backend-3", nil),
		s.backendClient.EXPECT().GetRevisionContent(gomock.Any(), uri, 1).Return(secretValue, nil),
		activeBackend.EXPECT().SaveContent(gomock.Any(), uri, 1, secretValue).Return("", errors.NotSupportedf("")),
		s.backendClient.EXPECT().GetBackend(gomock.Any(), ptr("backend-1"), true).Return(oldBackend, "", nil),

		// revision 2
		s.backendClient.EXPECT().GetBackend(gomock.Any(), nil, true).Return(activeBackend, "backend-3", nil),
		s.backendClient.EXPECT().GetRevisionContent(gomock.Any(), uri, 2).Return(secretValue, nil),
		activeBackend.EXPECT().SaveContent(gomock.Any(), uri, 2, secretValue).Return("", errors.NotSupportedf("")),
		s.backendClient.EXPECT().GetBackend(gomock.Any(), ptr("backend-2"), true).Return(oldBackend, "", nil),

		s.facade.EXPECT().ChangeSecretBackend(
			gomock.Any(),
			[]secretsdrain.ChangeSecretBackendArg{
				{
					URI:      uri,
					Revision: 1,
					Data:     secretValue.EncodedValues(),
				},
				{
					URI:      uri,
					Revision: 2,
					Data:     secretValue.EncodedValues(),
				},
			},
		).Return(secretsdrain.ChangeSecretBackendResult{Results: []error{
			nil,
			errors.New("failed"), // 2nd one failed.
		}}, nil),
		// We only delete for the 1st revision.
		oldBackend.EXPECT().DeleteContent(gomock.Any(), "revision-1").DoAndReturn(func(_ context.Context, _ string) error {
			close(s.done)
			return nil
		}),
	)
	start(`failed to drain secret revisions for "secret:.*" to the active backend`)
}

func (s *workerSuite) TestDrainLeadershipChange(c *tc.C) {
	start, ctrl := s.getWorkerNewer(c)
	defer ctrl.Finish()

	s.leadershipTracker.EXPECT().WithStableLeadership(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
			return leadership.ErrLeadershipChanged
		})
	s.leadershipTracker.EXPECT().WithStableLeadership(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(ctx context.Context) error) error {
			return fn(ctx)
		})

	uri := coresecrets.NewURI()
	s.notifyBackendChangedCh <- struct{}{}
	secretValue := coresecrets.NewSecretValue(map[string]string{"foo": "bar"})

	activeBackend := mocks.NewMockSecretsBackend(ctrl)

	gomock.InOrder(
		s.facade.EXPECT().GetSecretsToDrain(gomock.Any()).Return([]coresecrets.SecretMetadataForDrain{
			{
				URI:       uri,
				Revisions: []coresecrets.SecretExternalRevision{{Revision: 1}},
			},
		}, nil),
		s.backendClient.EXPECT().GetBackend(gomock.Any(), nil, true).Return(activeBackend, "backend-2", nil),
		s.backendClient.EXPECT().GetRevisionContent(gomock.Any(), uri, 1).Return(secretValue, nil),
		activeBackend.EXPECT().SaveContent(gomock.Any(), uri, 1, secretValue).Return("revision-1", nil),
		s.facade.EXPECT().ChangeSecretBackend(gomock.Any(),
			[]secretsdrain.ChangeSecretBackendArg{
				{
					URI:      uri,
					Revision: 1,
					ValueRef: &coresecrets.ValueRef{
						BackendID:  "backend-2",
						RevisionID: "revision-1",
					},
				},
			},
		).DoAndReturn(func(context.Context, []secretsdrain.ChangeSecretBackendArg) (secretsdrain.ChangeSecretBackendResult, error) {
			close(s.done)
			return secretsdrain.ChangeSecretBackendResult{Results: []error{nil}}, nil
		}),
	)
	start("")
}

func ptr[T any](v T) *T {
	return &v
}
