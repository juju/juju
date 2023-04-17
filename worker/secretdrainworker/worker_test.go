// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretdrainworker_test

import (
	"time"

	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"

	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/core/watcher/watchertest"
	jujusecrets "github.com/juju/juju/secrets"
	"github.com/juju/juju/secrets/provider"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/secretdrainworker"
	"github.com/juju/juju/worker/secretdrainworker/mocks"
)

type workerSuite struct {
	testing.IsolationSuite
	logger loggo.Logger

	facade        *mocks.MockFacade
	backendClient *mocks.MockBackendsClient

	done                   chan struct{}
	notifyBackendChangedCh chan struct{}
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) getWorkerNewer(c *gc.C, calls ...*gomock.Call) (func(), *gomock.Controller) {
	ctrl := gomock.NewController(c)
	s.logger = loggo.GetLogger("test")
	s.facade = mocks.NewMockFacade(ctrl)
	s.backendClient = mocks.NewMockBackendsClient(ctrl)

	s.notifyBackendChangedCh = make(chan struct{}, 1)
	s.done = make(chan struct{})
	s.facade.EXPECT().WatchSecretBackendChanged().Return(watchertest.NewMockNotifyWatcher(s.notifyBackendChangedCh), nil)

	start := func() {
		w, err := secretdrainworker.NewWorker(secretdrainworker.Config{
			Logger:             s.logger,
			SecretsDrainFacade: s.facade,
			SecretsBackendGetter: func() (jujusecrets.BackendsClient, error) {
				return s.backendClient, nil
			},
		})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(w, gc.NotNil)
		s.AddCleanup(func(c *gc.C) {
			workertest.CleanKill(c, w)
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
	start()
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
		s.backendClient.EXPECT().GetBackend(nil).DoAndReturn(func(_ *string) (*provider.SecretsBackend, string, error) {
			close(s.done)
			return nil, "backend-1", nil
		}),
	)
	start()
}

func (s *workerSuite) TestDrainBetweenExternalBackends(c *gc.C) {
	start, ctrl := s.getWorkerNewer(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	s.notifyBackendChangedCh <- struct{}{}
	secretValue := coresecrets.NewSecretValue(map[string]string{"foo": "bar"})

	oldBackend := mocks.NewMockSecretsBackend(ctrl)
	newVauleRef := coresecrets.ValueRef{
		BackendID:  "backend-2",
		RevisionID: "revision-1",
	}
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
		s.backendClient.EXPECT().GetBackend(nil).DoAndReturn(func(_ *string) (*provider.SecretsBackend, string, error) {
			return nil, "backend-2", nil
		}),
		s.backendClient.EXPECT().GetRevisionContent(uri, 1).Return(secretValue, nil),
		s.backendClient.EXPECT().SaveContent(uri, 1, secretValue).Return(newVauleRef, nil),
		s.backendClient.EXPECT().GetBackend(ptr("backend-1")).Return(oldBackend, "", nil),
		s.facade.EXPECT().ChangeSecretBackend(uri, 1, &newVauleRef, nil).Return(nil),
		oldBackend.EXPECT().DeleteContent(gomock.Any(), "revision-1").DoAndReturn(func(_ any, _ string) error {
			close(s.done)
			return nil
		}),
	)
	start()
}

func (s *workerSuite) TestDrainFromInternalToExternal(c *gc.C) {
	start, ctrl := s.getWorkerNewer(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	s.notifyBackendChangedCh <- struct{}{}
	secretValue := coresecrets.NewSecretValue(map[string]string{"foo": "bar"})

	newVauleRef := coresecrets.ValueRef{
		BackendID:  "backend-2",
		RevisionID: "revision-1",
	}
	gomock.InOrder(
		s.facade.EXPECT().GetSecretsToDrain().Return([]coresecrets.SecretMetadataForDrain{
			{
				Metadata:  coresecrets.SecretMetadata{URI: uri},
				Revisions: []coresecrets.SecretRevisionMetadata{{Revision: 1}},
			},
		}, nil),
		s.backendClient.EXPECT().GetBackend(nil).DoAndReturn(func(_ *string) (*provider.SecretsBackend, string, error) {
			return nil, "backend-2", nil
		}),
		s.backendClient.EXPECT().GetRevisionContent(uri, 1).Return(secretValue, nil),
		s.backendClient.EXPECT().SaveContent(uri, 1, secretValue).Return(newVauleRef, nil),
		s.facade.EXPECT().ChangeSecretBackend(uri, 1, &newVauleRef, nil).DoAndReturn(func(*coresecrets.URI, int, *coresecrets.ValueRef, coresecrets.SecretData) error {
			close(s.done)
			return nil
		}),
	)
	start()
}

func (s *workerSuite) TestDrainFromExternalToInternal(c *gc.C) {
	start, ctrl := s.getWorkerNewer(c)
	defer ctrl.Finish()

	uri := coresecrets.NewURI()
	s.notifyBackendChangedCh <- struct{}{}
	secretValue := coresecrets.NewSecretValue(map[string]string{"foo": "bar"})

	oldBackend := mocks.NewMockSecretsBackend(ctrl)
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
		s.backendClient.EXPECT().GetBackend(nil).DoAndReturn(func(_ *string) (*provider.SecretsBackend, string, error) {
			return nil, "backend-2", nil
		}),
		s.backendClient.EXPECT().GetRevisionContent(uri, 1).Return(secretValue, nil),
		s.backendClient.EXPECT().SaveContent(uri, 1, secretValue).Return(coresecrets.ValueRef{}, errors.NotSupportedf("")),
		s.backendClient.EXPECT().GetBackend(ptr("backend-1")).Return(oldBackend, "", nil),
		s.facade.EXPECT().ChangeSecretBackend(uri, 1, nil, secretValue.EncodedValues()).Return(nil),
		oldBackend.EXPECT().DeleteContent(gomock.Any(), "revision-1").DoAndReturn(func(_ any, _ string) error {
			close(s.done)
			return nil
		}),
	)
	start()
}

func ptr[T any](v T) *T {
	return &v
}
