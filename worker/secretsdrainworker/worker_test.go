// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsdrainworker_test

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/common/secretsdrain"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher/watchertest"
	jujusecrets "github.com/juju/juju/secrets"
	"github.com/juju/juju/secrets/provider"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/secretsdrainworker"
	"github.com/juju/juju/worker/secretsdrainworker/mocks"
)

type workerSuite struct {
	testing.IsolationSuite
	logger loggo.Logger

	facade        *mocks.MockSecretsDrainFacade
	backendClient *mocks.MockBackendsClient

	done                   chan struct{}
	notifyBackendChangedCh chan struct{}
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) getWorkerNewer(c *gc.C, calls ...*gomock.Call) (func(string), *gomock.Controller) {
	ctrl := gomock.NewController(c)
	s.logger = loggo.GetLogger("test")
	s.facade = mocks.NewMockSecretsDrainFacade(ctrl)
	s.backendClient = mocks.NewMockBackendsClient(ctrl)

	s.notifyBackendChangedCh = make(chan struct{}, 1)
	s.done = make(chan struct{})
	s.facade.EXPECT().WatchSecretBackendChanged().Return(watchertest.NewMockNotifyWatcher(s.notifyBackendChangedCh), nil)

	start := func(expectedErr string) {
		w, err := secretsdrainworker.NewWorker(secretsdrainworker.Config{
			Logger:             s.logger,
			SecretsDrainFacade: s.facade,
			SecretsBackendGetter: func() (jujusecrets.BackendsClient, error) {
				return s.backendClient, nil
			},
		})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(w, gc.NotNil)
		s.AddCleanup(func(c *gc.C) {
			if expectedErr == "" {
				workertest.CleanKill(c, w)
			} else {
				err := workertest.CheckKilled(c, w)
				c.Assert(err, gc.ErrorMatches, expectedErr)
			}
		})
		s.waitDone(c)
	}
	return start, ctrl
}

func (s *workerSuite) waitDone(c *gc.C) {
	select {
	case <-s.done:
	case <-time.After(coretesting.ShortWait):
		c.Errorf("timed out waiting for worker")
	}
}

func (s *workerSuite) TestNothingToDrain(c *gc.C) {
	start, ctrl := s.getWorkerNewer(c)
	defer ctrl.Finish()

	s.notifyBackendChangedCh <- struct{}{}
	gomock.InOrder(
		s.facade.EXPECT().GetSecretsToDrain().DoAndReturn(func() ([]coresecrets.SecretMetadataForDrain, error) {
			close(s.done)
			return []coresecrets.SecretMetadataForDrain{}, nil
		}),
	)
	start("")
}

func (s *workerSuite) TestDrainNoOPS(c *gc.C) {
	start, ctrl := s.getWorkerNewer(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	s.notifyBackendChangedCh <- struct{}{}
	gomock.InOrder(
		s.facade.EXPECT().GetSecretsToDrain().Return([]coresecrets.SecretMetadataForDrain{
			{
				Metadata: coresecrets.SecretMetadata{URI: uri},
				Revisions: []coresecrets.SecretRevisionMetadata{
					{
						Revision: 1,
						ValueRef: &coresecrets.ValueRef{BackendID: "backend-1", RevisionID: "revision-1"},
					},
				},
			},
		}, nil),
		s.backendClient.EXPECT().GetBackend(nil, true).DoAndReturn(func(*string, bool) (*provider.SecretsBackend, string, error) {
			close(s.done)
			return nil, "backend-1", nil
		}),
	)
	start("")
}

func (s *workerSuite) TestDrainBetweenExternalBackends(c *gc.C) {
	start, ctrl := s.getWorkerNewer(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	s.notifyBackendChangedCh <- struct{}{}
	secretValue := coresecrets.NewSecretValue(map[string]string{"foo": "bar"})

	oldBackend := mocks.NewMockSecretsBackend(ctrl)
	activeBackend := mocks.NewMockSecretsBackend(ctrl)

	gomock.InOrder(
		s.facade.EXPECT().GetSecretsToDrain().Return([]coresecrets.SecretMetadataForDrain{
			{
				Metadata: coresecrets.SecretMetadata{URI: uri},
				Revisions: []coresecrets.SecretRevisionMetadata{
					{
						Revision: 1,
						ValueRef: &coresecrets.ValueRef{BackendID: "backend-1", RevisionID: "revision-1"},
					},
				},
			},
		}, nil),
		s.backendClient.EXPECT().GetBackend(nil, true).Return(activeBackend, "backend-2", nil),
		s.backendClient.EXPECT().GetRevisionContent(uri, 1).Return(secretValue, nil),
		activeBackend.EXPECT().SaveContent(gomock.Any(), uri, 1, secretValue).Return("revision-1", nil),
		s.backendClient.EXPECT().GetBackend(ptr("backend-1"), true).Return(oldBackend, "", nil),
		s.facade.EXPECT().ChangeSecretBackend(
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
		oldBackend.EXPECT().DeleteContent(gomock.Any(), "revision-1").DoAndReturn(func(_ any, _ string) error {
			close(s.done)
			return nil
		}),
	)
	start("")
}

func (s *workerSuite) TestDrainFromInternalToExternal(c *gc.C) {
	start, ctrl := s.getWorkerNewer(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	s.notifyBackendChangedCh <- struct{}{}
	secretValue := coresecrets.NewSecretValue(map[string]string{"foo": "bar"})

	activeBackend := mocks.NewMockSecretsBackend(ctrl)

	gomock.InOrder(
		s.facade.EXPECT().GetSecretsToDrain().Return([]coresecrets.SecretMetadataForDrain{
			{
				Metadata:  coresecrets.SecretMetadata{URI: uri},
				Revisions: []coresecrets.SecretRevisionMetadata{{Revision: 1}},
			},
		}, nil),
		s.backendClient.EXPECT().GetBackend(nil, true).Return(activeBackend, "backend-2", nil),
		s.backendClient.EXPECT().GetRevisionContent(uri, 1).Return(secretValue, nil),
		activeBackend.EXPECT().SaveContent(gomock.Any(), uri, 1, secretValue).Return("revision-1", nil),
		s.facade.EXPECT().ChangeSecretBackend(
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
		).DoAndReturn(func([]secretsdrain.ChangeSecretBackendArg) (secretsdrain.ChangeSecretBackendResult, error) {
			close(s.done)
			return secretsdrain.ChangeSecretBackendResult{Results: []error{nil}}, nil
		}),
	)
	start("")
}

func (s *workerSuite) TestDrainFromExternalToInternal(c *gc.C) {
	start, ctrl := s.getWorkerNewer(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	s.notifyBackendChangedCh <- struct{}{}
	secretValue := coresecrets.NewSecretValue(map[string]string{"foo": "bar"})

	oldBackend := mocks.NewMockSecretsBackend(ctrl)
	activeBackend := mocks.NewMockSecretsBackend(ctrl)
	gomock.InOrder(
		s.facade.EXPECT().GetSecretsToDrain().Return([]coresecrets.SecretMetadataForDrain{
			{
				Metadata: coresecrets.SecretMetadata{URI: uri},
				Revisions: []coresecrets.SecretRevisionMetadata{
					{
						Revision: 1,
						ValueRef: &coresecrets.ValueRef{BackendID: "backend-1", RevisionID: "revision-1"},
					},
				},
			},
		}, nil),
		s.backendClient.EXPECT().GetBackend(nil, true).Return(activeBackend, "backend-2", nil),
		s.backendClient.EXPECT().GetRevisionContent(uri, 1).Return(secretValue, nil),
		activeBackend.EXPECT().SaveContent(gomock.Any(), uri, 1, secretValue).Return("", errors.NotSupportedf("")),
		s.backendClient.EXPECT().GetBackend(ptr("backend-1"), true).Return(oldBackend, "", nil),
		s.facade.EXPECT().ChangeSecretBackend(
			[]secretsdrain.ChangeSecretBackendArg{
				{
					URI:      uri,
					Revision: 1,
					Data:     secretValue.EncodedValues(),
				},
			},
		).Return(secretsdrain.ChangeSecretBackendResult{Results: []error{nil}}, nil),
		oldBackend.EXPECT().DeleteContent(gomock.Any(), "revision-1").DoAndReturn(func(_ any, _ string) error {
			close(s.done)
			return nil
		}),
	)
	start("")
}

func (s *workerSuite) TestDrainPartiallyFailed(c *gc.C) {
	// If the drain fails for one revision, it should continue to drain the rest.
	// But the agent should be restarted to retry.
	start, ctrl := s.getWorkerNewer(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	s.notifyBackendChangedCh <- struct{}{}
	secretValue := coresecrets.NewSecretValue(map[string]string{"foo": "bar"})

	oldBackend := mocks.NewMockSecretsBackend(ctrl)
	activeBackend := mocks.NewMockSecretsBackend(ctrl)
	gomock.InOrder(
		s.facade.EXPECT().GetSecretsToDrain().Return([]coresecrets.SecretMetadataForDrain{
			{
				Metadata: coresecrets.SecretMetadata{URI: uri},
				Revisions: []coresecrets.SecretRevisionMetadata{
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
		s.backendClient.EXPECT().GetBackend(nil, true).Return(activeBackend, "backend-3", nil),
		s.backendClient.EXPECT().GetRevisionContent(uri, 1).Return(secretValue, nil),
		activeBackend.EXPECT().SaveContent(gomock.Any(), uri, 1, secretValue).Return("", errors.NotSupportedf("")),
		s.backendClient.EXPECT().GetBackend(ptr("backend-1"), true).Return(oldBackend, "", nil),

		// revision 2
		s.backendClient.EXPECT().GetBackend(nil, true).Return(activeBackend, "backend-3", nil),
		s.backendClient.EXPECT().GetRevisionContent(uri, 2).Return(secretValue, nil),
		activeBackend.EXPECT().SaveContent(gomock.Any(), uri, 2, secretValue).Return("", errors.NotSupportedf("")),
		s.backendClient.EXPECT().GetBackend(ptr("backend-2"), true).Return(oldBackend, "", nil),

		s.facade.EXPECT().ChangeSecretBackend(
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
		oldBackend.EXPECT().DeleteContent(gomock.Any(), "revision-1").DoAndReturn(func(_ any, _ string) error {
			close(s.done)
			return nil
		}),
	)
	start(`failed to drain secret revisions for "secret:.*" to the active backend`)
}

func ptr[T any](v T) *T {
	return &v
}
