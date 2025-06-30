// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade/mocks"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type agentEntityWatcherSuite struct {
	watcherRegistry *mocks.MockWatcherRegistry
}

func TestAgentEntityWatcherSuite(t *testing.T) {
	tc.Run(t, &agentEntityWatcherSuite{})
}

type fakeAgentEntityWatcher struct {
	state.Entity
	fetchError
}

func (a *fakeAgentEntityWatcher) Watch() state.NotifyWatcher {
	return apiservertesting.NewFakeNotifyWatcher()
}

func (s *agentEntityWatcherSuite) setUpMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.watcherRegistry = mocks.NewMockWatcherRegistry(ctrl)
	return ctrl
}

func (s *agentEntityWatcherSuite) TestWatch(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	st := &fakeState{
		entities: map[names.Tag]entityWithError{
			fs("0"): &fakeAgentEntityWatcher{fetchError: "fs0 fails"},
			fs("1"): &fakeAgentEntityWatcher{},
			fs("2"): &fakeAgentEntityWatcher{},
		},
	}
	getCanWatch := func(ctx context.Context) (common.AuthFunc, error) {
		x0 := fs("0")
		x1 := fs("1")
		return func(tag names.Tag) bool {
			return tag == x0 || tag == x1
		}, nil
	}
	// Only the watcher on x/1 is registered.
	s.watcherRegistry.EXPECT().Register(gomock.AssignableToTypeOf(&apiservertesting.FakeNotifyWatcher{})).Return("1", nil)
	a := common.NewAgentEntityWatcher(st, s.watcherRegistry, getCanWatch)
	entities := params.Entities{Entities: []params.Entity{
		{Tag: "filesystem-0"}, {Tag: "filesystem-1"}, {Tag: "filesystem-2"}, {Tag: "filesystem-3"},
	}}
	result, err := a.Watch(c.Context(), entities)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.NotifyWatchResults{
		Results: []params.NotifyWatchResult{
			{Error: &params.Error{Message: "fs0 fails"}},
			{NotifyWatcherId: "1", Error: nil},
			{Error: apiservertesting.ErrUnauthorized},
			{Error: apiservertesting.ErrUnauthorized},
		},
	})
}

func (*agentEntityWatcherSuite) TestWatchError(c *tc.C) {
	getCanWatch := func(ctx context.Context) (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	a := common.NewAgentEntityWatcher(
		&fakeState{},
		nil,
		getCanWatch,
	)
	_, err := a.Watch(c.Context(), params.Entities{Entities: []params.Entity{{Tag: "fs0"}}})
	c.Assert(err, tc.ErrorMatches, "pow")
}

func (*agentEntityWatcherSuite) TestWatchNoArgsNoError(c *tc.C) {
	getCanWatch := func(ctx context.Context) (common.AuthFunc, error) {
		return nil, fmt.Errorf("pow")
	}
	a := common.NewAgentEntityWatcher(
		&fakeState{},
		nil,
		getCanWatch,
	)
	result, err := a.Watch(c.Context(), params.Entities{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Results, tc.HasLen, 0)
}

func fs(fs string) names.Tag { return names.NewFilesystemTag(fs) }
