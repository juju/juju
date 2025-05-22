// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storageregistry

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/storage"
)

type trackedWorkerSuite struct {
	baseSuite

	states   chan string
	registry *MockProviderRegistry
	provider *MockProvider
}

func TestTrackedWorkerSuite(t *stdtesting.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &trackedWorkerSuite{})
}

func (s *trackedWorkerSuite) TestKilled(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w, err := NewTrackedWorker(s.registry)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CheckKill(c, w)

	w.Kill()
}

func (s *trackedWorkerSuite) TestStorageProviderTypesWithCommon(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.registry.EXPECT().StorageProviderTypes().Return([]storage.ProviderType{"ebs"}, nil)

	w, err := NewTrackedWorker(s.registry)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CheckKill(c, w)

	types, err := w.(*trackedWorker).StorageProviderTypes()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(types, tc.DeepEquals, []storage.ProviderType{"ebs", "loop", "rootfs", "tmpfs"})
}

func (s *trackedWorkerSuite) TestStorageProviderTypesWithEmptyProviderTypes(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.registry.EXPECT().StorageProviderTypes().Return([]storage.ProviderType{}, nil)

	w, err := NewTrackedWorker(s.registry)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CheckKill(c, w)

	types, err := w.(*trackedWorker).StorageProviderTypes()
	c.Assert(err, tc.ErrorIsNil)
	c.Check(types, tc.DeepEquals, []storage.ProviderType{"loop", "rootfs", "tmpfs"})
}

func (s *trackedWorkerSuite) TestStorageProvider(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.registry.EXPECT().StorageProvider(storage.ProviderType("rootfs")).Return(s.provider, nil)

	w, err := NewTrackedWorker(s.registry)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CheckKill(c, w)

	provider, err := w.(*trackedWorker).StorageProvider(storage.ProviderType("rootfs"))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(provider, tc.DeepEquals, s.provider)
}

func (s *trackedWorkerSuite) setupMocks(c *tc.C) *gomock.Controller {
	// Ensure we buffer the channel, this is because we might miss the
	// event if we're too quick at starting up.
	s.states = make(chan string, 1)

	ctrl := s.baseSuite.setupMocks(c)

	s.registry = NewMockProviderRegistry(ctrl)
	s.provider = NewMockProvider(ctrl)

	return ctrl
}
