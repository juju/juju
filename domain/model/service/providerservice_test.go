// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	corecloud "github.com/juju/juju/core/cloud"
	cloudtesting "github.com/juju/juju/core/cloud/testing"
	"github.com/juju/juju/core/credential"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/watcher/watchertest"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/uuid"
)

type dummyProviderState struct {
	model          *coremodel.ModelInfo
	cloudUUID      corecloud.UUID
	credentialUUID credential.UUID
}

func (d *dummyProviderState) GetModelCloudAndCredential(ctx context.Context, modelUUID coremodel.UUID) (corecloud.UUID, credential.UUID, error) {
	if d.model == nil {
		return "", "", modelerrors.NotFound
	}
	return d.cloudUUID, d.credentialUUID, nil
}

func (d *dummyProviderState) GetModel(ctx context.Context) (coremodel.ModelInfo, error) {
	if d.model != nil {
		return *d.model, nil
	}
	return coremodel.ModelInfo{}, modelerrors.NotFound
}

type providerServiceSuite struct {
	testhelpers.IsolationSuite

	state *dummyProviderState

	mockControllerState *MockState
	mockWatcherFactory  *MockWatcherFactory
}

func TestProviderServiceSuite(t *stdtesting.T) { tc.Run(t, &providerServiceSuite{}) }
func (s *providerServiceSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.mockControllerState = NewMockState(ctrl)
	s.mockWatcherFactory = NewMockWatcherFactory(ctrl)

	return ctrl
}

func (s *providerServiceSuite) SetUpTest(c *tc.C) {
	s.state = &dummyProviderState{
		cloudUUID:      cloudtesting.GenCloudUUID(c),
		credentialUUID: credential.UUID(uuid.MustNewUUID().String()),
	}
}

func (s *providerServiceSuite) TestModel(c *tc.C) {
	svc := NewProviderService(s.state, s.state, nil)

	id := modeltesting.GenModelUUID(c)
	model := coremodel.ModelInfo{
		UUID:        id,
		Name:        "my-awesome-model",
		Cloud:       "aws",
		CloudRegion: "myregion",
		Type:        coremodel.IAAS,
	}
	s.state.model = &model

	got, err := svc.Model(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(got, tc.Equals, model)
}

func (s *providerServiceSuite) TestWatchModelCloudCredential(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := modeltesting.GenModelUUID(c)
	cloudUUID := cloudtesting.GenCloudUUID(c)
	credentialUUID := credential.UUID(uuid.MustNewUUID().String())
	s.mockControllerState.EXPECT().GetModelCloudAndCredential(gomock.Any(), modelUUID).Return(cloudUUID, credentialUUID, nil)

	ch := make(chan struct{}, 1)
	watcher := watchertest.NewMockNotifyWatcher(ch)
	s.mockWatcherFactory.EXPECT().NewNotifyMapperWatcher(
		gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(watcher, nil)

	svc := NewProviderService(
		s.mockControllerState,
		&dummyProviderState{},
		s.mockWatcherFactory,
	)
	w, err := svc.WatchModelCloudCredential(c.Context(), modelUUID)
	c.Assert(err, tc.ErrorIsNil)

	select {
	case ch <- struct{}{}:
	case <-time.After(testhelpers.LongWait):
		c.Fatalf("failed to send changes to channel")
	}

	select {
	case <-w.Changes():
	case <-time.After(testhelpers.LongWait):
		c.Fatalf("failed to receive changes from watcher")
	}
}
