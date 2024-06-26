// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package keyupdater

import (
	"context"
	"sync"
	"time"

	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	facademocks "github.com/juju/juju/apiserver/facade/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/watchertest"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/testing"
)

type authorisedKeysSuite struct {
	authorizer        apiservertesting.FakeAuthorizer
	keyUpdaterService *MockKeyUpdaterService
	machineTag        names.MachineTag
	watcherRegistry   *facademocks.MockWatcherRegistry
}

var _ = gc.Suite(&authorisedKeysSuite{})

func (s *authorisedKeysSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.keyUpdaterService = NewMockKeyUpdaterService(ctrl)
	s.watcherRegistry = facademocks.NewMockWatcherRegistry(ctrl)
	return ctrl
}

func (s *authorisedKeysSuite) SetUpTest(c *gc.C) {
	s.machineTag = names.NewMachineTag("0")

	// The default auth is as a controller
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: s.machineTag,
	}
}

// TestNewKeyUpdaterAPIRefusesNonMachineAgent is checking that if we try and
// setup the facade with a non machine entity the facade fails to construct with
// [apiservererrors.ErrPerm] error.
func (s *authorisedKeysSuite) TestNewKeyUpdaterAPIRefusesNonMachineAgent(c *gc.C) {
	anAuthoriser := s.authorizer
	anAuthoriser.Tag = names.NewUnitTag("ubuntu/1")
	endPoint, err := newKeyUpdaterAPI(
		anAuthoriser, s.keyUpdaterService, s.watcherRegistry,
	)
	c.Check(endPoint, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "permission denied")
	c.Check(err, jc.ErrorIs, apiservererrors.ErrPerm)
}

// TestWatchAuthorisedKeysNothing is asserting that it is not an error to watch
// authorised keys for zero entities.
func (s *authorisedKeysSuite) TestWatchAuthorisedKeysNothing(c *gc.C) {
	endPoint, err := newKeyUpdaterAPI(
		s.authorizer, s.keyUpdaterService, s.watcherRegistry,
	)
	c.Assert(err, jc.ErrorIsNil)
	// Not an error to watch nothing
	results, err := endPoint.WatchAuthorisedKeys(context.Background(), params.Entities{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 0)
}

// TestWatchAuthorisedKeys is asserting that for machines the caller is allowed
// to watch we get back a valid watcher id. For machines that cannot be watched
// by the caller an unathorised error is returned.
func (s *authorisedKeysSuite) TestWatchAuthorisedKeys(c *gc.C) {
	defer s.setupMocks(c).Finish()

	endPoint, err := newKeyUpdaterAPI(
		s.authorizer, s.keyUpdaterService, s.watcherRegistry,
	)
	c.Assert(err, jc.ErrorIsNil)

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
	ch := make(chan []string)
	w := watchertest.NewMockStringsWatcher(ch)

	s.keyUpdaterService.EXPECT().WatchAuthorisedKeysForMachine(
		gomock.Any(),
		coremachine.ID("0"),
	).DoAndReturn(func(_ context.Context, _ coremachine.ID) (watcher.Watcher[[]string], error) {
		wg.Add(1)
		time.AfterFunc(testing.ShortWait, func() {
			defer wg.Done()
			// Send initial event.
			select {
			case ch <- []string{}:
			case <-done:
				c.ExpectFailure("watcher did not fire")
			}
		})
		return w, nil
	})
	s.watcherRegistry.EXPECT().Register(gomock.Any()).Return("1", nil)

	result, err := endPoint.WatchAuthorisedKeys(context.Background(), args)
	c.Check(err, jc.ErrorIsNil)
	c.Check(result, gc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{NotifyWatcherId: "1"},
			{Error: &params.Error{
				Code:    params.CodeUnauthorized,
				Message: "does not have permission to read authorised keys for \"machine-40\"",
			}},
			{Error: &params.Error{
				Code:    params.CodeUnauthorized,
				Message: "does not have permission to read authorised keys for \"machine-42\"",
			}},
		},
	})
}

// TestAuthorisedKeysForNoone is asserting that if we ask for authorised keys
// for zero machines we back an empty result with no errors.
func (s *authorisedKeysSuite) TestAuthorisedKeysForNoone(c *gc.C) {
	endPoint, err := newKeyUpdaterAPI(
		s.authorizer, s.keyUpdaterService, s.watcherRegistry,
	)
	c.Assert(err, jc.ErrorIsNil)
	// Not an error to watch nothing
	results, err := endPoint.AuthorisedKeys(context.Background(), params.Entities{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(results.Results, gc.HasLen, 0)
}

// TestAuthorisedKeys is asserting that the caller can get back authorised keys
// for the authenticated machine. For any other machines that the caller is not
// authenticated for we back unathorised errors.
func (s *authorisedKeysSuite) TestAuthorisedKeys(c *gc.C) {
	defer s.setupMocks(c).Finish()
	endPoint, err := newKeyUpdaterAPI(
		s.authorizer, s.keyUpdaterService, s.watcherRegistry,
	)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: s.machineTag.String()},
			{Tag: "machine-40"},
			{Tag: "machine-42"},
		},
	}

	s.keyUpdaterService.EXPECT().AuthorisedKeysForMachine(gomock.Any(), coremachine.ID("0")).
		Return([]string{"key1", "key2"}, nil)

	result, err := endPoint.AuthorisedKeys(context.Background(), args)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringsResults{
		Results: []params.StringsResult{
			{Result: []string{"key1", "key2"}},
			{Error: &params.Error{
				Code:    params.CodeUnauthorized,
				Message: "does not have permission to read authorised keys for \"machine-40\"",
			}},
			{Error: &params.Error{
				Code:    params.CodeUnauthorized,
				Message: "does not have permission to read authorised keys for \"machine-42\"",
			}},
		},
	})
}

// TestAuthorisedKeysForNonMachineEntity is asserting that if we try and get
// authorised keys for a non machine enitity we get back a
// [params.CodeTagKindNotSupported] error.
func (s *authorisedKeysSuite) TestAuthorisedKeysForNonMachineEntity(c *gc.C) {
	endPoint, err := newKeyUpdaterAPI(
		s.authorizer, s.keyUpdaterService, s.watcherRegistry,
	)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewUnitTag("ubuntu/1").String()},
		},
	}

	result, err := endPoint.AuthorisedKeys(context.Background(), args)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringsResults{
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
func (s *authorisedKeysSuite) TestWatchAuthorisedKeysForNonMachineEntity(c *gc.C) {
	endPoint, err := newKeyUpdaterAPI(
		s.authorizer, s.keyUpdaterService, s.watcherRegistry,
	)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewUnitTag("ubuntu/1").String()},
		},
	}

	result, err := endPoint.WatchAuthorisedKeys(context.Background(), args)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.NotifyWatchResults{
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
func (s *authorisedKeysSuite) TestAuthorisedKeysForNotFoundMachine(c *gc.C) {
	defer s.setupMocks(c).Finish()
	endPoint, err := newKeyUpdaterAPI(
		s.authorizer, s.keyUpdaterService, s.watcherRegistry,
	)
	c.Assert(err, jc.ErrorIsNil)

	args := params.Entities{
		Entities: []params.Entity{
			{Tag: s.machineTag.String()},
		},
	}

	s.keyUpdaterService.EXPECT().AuthorisedKeysForMachine(
		gomock.Any(), coremachine.ID("0"),
	).Return(nil, machineerrors.NotFound)

	result, err := endPoint.AuthorisedKeys(context.Background(), args)
	c.Check(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, params.StringsResults{
		Results: []params.StringsResult{
			{Error: &params.Error{
				Code:    params.CodeMachineNotFound,
				Message: "machine \"0\" does not exist",
			}},
		},
	})
}
