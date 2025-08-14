// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator

import (
	"context"
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	"github.com/juju/juju/core/credential"
	coreerrors "github.com/juju/juju/core/errors"
	coremodel "github.com/juju/juju/core/model"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/core/watcher"
	credentialerrors "github.com/juju/juju/domain/credential/errors"
	"github.com/juju/juju/rpc/params"
)

type CredentialValidatorSuite struct {
	modelUUID coremodel.UUID

	credentialService      *MockModelCredentialService
	modelCredentialWatcher *MockNotifyWatcher

	modelCredentialWatcherGetter func(ctx context.Context) (watcher.NotifyWatcher, error)
	watcherRegistry              *facademocks.MockWatcherRegistry

	api *CredentialValidatorAPI
}

func TestCredentialValidatorSuite(t *testing.T) {
	tc.Run(t, &CredentialValidatorSuite{})
}
func (s *CredentialValidatorSuite) SetupTest(c *tc.C) {
	s.modelUUID = coremodel.GenUUID(c)
}

func (s *CredentialValidatorSuite) setUpMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.credentialService = NewMockModelCredentialService(ctrl)
	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)
	s.modelCredentialWatcher = NewMockNotifyWatcher(ctrl)

	s.api = NewCredentialValidatorAPI(
		s.modelUUID,
		s.credentialService,
		s.modelCredentialWatcherGetter,
		s.watcherRegistry,
	)
	return ctrl
}

func (s *CredentialValidatorSuite) TestModelCredential(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	modelCredentialKey := credential.Key{
		Cloud: "cloud",
		Owner: usertesting.GenNewName(c, "user"),
		Name:  "credential",
	}
	s.credentialService.EXPECT().GetModelCredentialStatus(gomock.Any()).Return(
		modelCredentialKey, true, nil,
	)
	credTag := names.NewCloudCredentialTag("cloud/user/credential")

	result, err := s.api.ModelCredential(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ModelCredential{
		Model:           names.NewModelTag(s.modelUUID.String()).String(),
		Exists:          true,
		CloudCredential: credTag.String(),
		Valid:           true,
	})
}

// TestModelCredentialNotSet is testing that when no credential has been set for
// the model we get back a valid results with exists set to false.
func (s *CredentialValidatorSuite) TestModelCredentialNotSet(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	s.credentialService.EXPECT().GetModelCredentialStatus(gomock.Any()).Return(
		credential.Key{}, false, credentialerrors.ModelCredentialNotSet,
	)

	result, err := s.api.ModelCredential(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.ModelCredential{
		Model:  names.NewModelTag(s.modelUUID.String()).String(),
		Exists: false,
		Valid:  true,
	})
}

func (s *CredentialValidatorSuite) TestWatchModelCredential(c *tc.C) {
	s.modelCredentialWatcherGetter = func(ctx context.Context) (watcher.NotifyWatcher, error) {
		return s.modelCredentialWatcher, nil
	}
	defer s.setUpMocks(c).Finish()
	ch := make(chan struct{}, 1)
	ch <- struct{}{}

	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("1", nil)
	s.modelCredentialWatcher.EXPECT().Changes().Return(ch)

	result, err := s.api.WatchModelCredential(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.NotifyWatchResult{"1", nil})
}

func (s *CredentialValidatorSuite) TestWatchModelCredentialError(c *tc.C) {
	s.modelCredentialWatcherGetter = func(ctx context.Context) (watcher.NotifyWatcher, error) {
		return nil, coreerrors.NotValid
	}
	defer s.setUpMocks(c).Finish()
	_, err := s.api.WatchModelCredential(c.Context())
	c.Assert(err, tc.DeepEquals, apiservererrors.ServerError(coreerrors.NotValid))
}
