// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keyupdater

import (
	"context"
	"sync"
	"time"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type authorisedKeysSuite struct {
	authorizer        apiservertesting.FakeAuthorizer
	keyUpdaterService *MockKeyUpdaterService
	machineTag        names.MachineTag
	watcherRegistry   *facademocks.MockWatcherRegistry
}

var _ = tc.Suite(&authorisedKeysSuite{})

func (s *authorisedKeysSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.keyUpdaterService = NewMockKeyUpdaterService(ctrl)
	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)
	return ctrl
}

func (s *authorisedKeysSuite) SetUpTest(c *tc.C) {
	s.machineTag = names.NewMachineTag("0")

	// The default auth is as a controller
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.machineTag,
	}
}

func (s *authorisedKeysSuite) getCanRead(context.Context) (common.AuthFunc, error) {
	return s.authorizer.AuthOwner, nil
}

// TestWatchAuthorisedKeysNothing is asserting that it is not an error to watch
// authorised keys for zero entities.
func (s *authorisedKeysSuite) TestWatchAuthorisedKeysNothing(c *tc.C) {
	endPoint := newKeyUpdaterAPI(
		s.getCanRead, s.keyUpdaterService, s.watcherRegistry,
	)
	results, err := endPoint.WatchAuthorisedKeys(context.Background(), params.Entities{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 0)
}

// TestWatchAuthorisedKeys is asserting that for machines the caller is allowed
// to watch we get back a valid watcher id. For machines that cannot be watched
// by the caller an unauthorised error is returned.
func (s *authorisedKeysSuite) TestWatchAuthorisedKeys(c *tc.C) {
	defer s.setupMocks(c).Finish()

	endPoint := newKeyUpdaterAPI(
		s.getCanRead, s.keyUpdaterService, s.watcherRegistry,
	)

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: s.machineTag.String()},
			{Tag: "machine-40"},
			{Tag: "machine-42"},
		},
	}

	done := make(chan struct{})
	defer close(done)
	wg := sync.WaitGroup{}
	defer wg.Wait()
	ch := make(chan struct{})
	w := watchertest.NewMockNotifyWatcher(ch)

	s.keyUpdaterService.EXPECT().WatchAuthorisedKeysForMachine(
		gomock.Any(),
		coremachine.Name("0"),
	).DoAndReturn(func(_ context.Context, _ coremachine.Name) (watcher.Watcher[struct{}], error) {
		wg.Add(1)
		time.AfterFunc(testing.ShortWait, func() {
			defer wg.Done()
			// Send initial event.
			select {
			case ch <- struct{}{}:
			case <-done:
				c.ExpectFailure("watcher did not fire")
			}
		})
		return w, nil
	})
	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("1", nil)

	result, err := endPoint.WatchAuthorisedKeys(context.Background(), args)
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{NotifyWatcherId: "1"},
			{Error: &params.Error{
				Code:    params.CodeUnauthorized,
				Message: "\"machine-40\" does not have permission to read authorized keys",
			}},
			{Error: &params.Error{
				Code:    params.CodeUnauthorized,
				Message: "\"machine-42\" does not have permission to read authorized keys",
			}},
		},
	})
}

// TestAuthorisedKeysForNoone is asserting that if we ask for authorised keys
// for zero machines we back an empty result with no errors.
func (s *authorisedKeysSuite) TestAuthorisedKeysForNone(c *tc.C) {
	endPoint := newKeyUpdaterAPI(
		s.getCanRead, s.keyUpdaterService, s.watcherRegistry,
	)
	// Not an error to watch nothing
	results, err := endPoint.AuthorisedKeys(context.Background(), params.Entities{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(results.Results, tc.HasLen, 0)
}

// TestAuthorisedKeys is asserting that the caller can get back authorised keys
// for the authenticated machine. For any other machines that the caller is not
// authenticated for we back unauthorised errors.
func (s *authorisedKeysSuite) TestAuthorisedKeys(c *tc.C) {
	defer s.setupMocks(c).Finish()
	endPoint := newKeyUpdaterAPI(
		s.getCanRead, s.keyUpdaterService, s.watcherRegistry,
	)

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: s.machineTag.String()},
			{Tag: "machine-40"},
			{Tag: "machine-42"},
		},
	}

	s.keyUpdaterService.EXPECT().GetAuthorisedKeysForMachine(gomock.Any(), coremachine.Name("0")).
		Return([]string{"key1", "key2"}, nil)

	result, err := endPoint.AuthorisedKeys(context.Background(), args)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringsResults{
		Results: []params.StringsResult{
			{Result: []string{"key1", "key2"}},
			{Error: &params.Error{
				Code:    params.CodeUnauthorized,
				Message: "no permission to read authorised keys for \"machine-40\"",
			}},
			{Error: &params.Error{
				Code:    params.CodeUnauthorized,
				Message: "no permission to read authorised keys for \"machine-42\"",
			}},
		},
	})
}

// TestAuthorisedKeysForNonMachineEntity is asserting that if we try and get
// authorised keys for a non machine enitity we get back a
// [params.CodeTagKindNotSupported] error.
func (s *authorisedKeysSuite) TestAuthorisedKeysForNonMachineEntity(c *tc.C) {
	endPoint := newKeyUpdaterAPI(
		s.getCanRead, s.keyUpdaterService, s.watcherRegistry,
	)

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewUnitTag("ubuntu/1").String()},
		},
	}

	result, err := endPoint.AuthorisedKeys(context.Background(), args)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringsResults{
		Results: []params.StringsResult{
			{Error: &params.Error{
				Code:    params.CodeTagKindNotSupported,
				Message: "tag \"unit-ubuntu-1\" unsupported, can only accept tags of kind \"machine\"",
			}},
		},
	})
}

// TestWatchAuthorisedKeysForNonMachineEntity is asserting that if we try and
// watch  authorised keys for a non machine enitity we get back a
// [params.CodeTagKindNotSupported] error.
func (s *authorisedKeysSuite) TestWatchAuthorisedKeysForNonMachineEntity(c *tc.C) {
	endPoint := newKeyUpdaterAPI(
		s.getCanRead, s.keyUpdaterService, s.watcherRegistry,
	)

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewUnitTag("ubuntu/1").String()},
		},
	}

	result, err := endPoint.WatchAuthorisedKeys(context.Background(), args)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{Error: &params.Error{
				Code:    params.CodeTagKindNotSupported,
				Message: "tag \"unit-ubuntu-1\" unsupported, can only accept tags of kind \"machine\"",
			}},
		},
	})
}

// TestAuthorisedKeysForNonMachineEntity is asserting that if we try and get
// authorised keys for a machine that doesn't exist
func (s *authorisedKeysSuite) TestAuthorisedKeysForNotFoundMachine(c *tc.C) {
	defer s.setupMocks(c).Finish()
	endPoint := newKeyUpdaterAPI(
		s.getCanRead, s.keyUpdaterService, s.watcherRegistry,
	)

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: s.machineTag.String()},
		},
	}

	s.keyUpdaterService.EXPECT().GetAuthorisedKeysForMachine(
		gomock.Any(), coremachine.Name("0"),
	).Return(nil, machineerrors.MachineNotFound)

	result, err := endPoint.AuthorisedKeys(context.Background(), args)
	c.Check(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringsResults{
		Results: []params.StringsResult{
			{Error: &params.Error{
				Code:    params.CodeMachineNotFound,
				Message: "machine \"0\" does not exist",
			}},
		},
	})
}
