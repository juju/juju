// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	"context"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/model"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

type modelMachinesWatcherSuite struct {
	testing.BaseSuite
}

var _ = tc.Suite(&modelMachinesWatcherSuite{})

func (f *fakeModelMachinesWatcher) WatchModelMachines() state.StringsWatcher {
	changes := make(chan []string, 1)
	// Simulate initial event.
	changes <- f.initial
	return &fakeStringsWatcher{changes: changes}
}

func (s *modelMachinesWatcherSuite) TestWatchModelMachines(c *tc.C) {
	authorizer := apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("0"),
		Controller: true,
	}
	resources := common.NewResources()
	s.AddCleanup(func(_ *tc.C) { resources.StopAll() })
	e := model.NewModelMachinesWatcher(
		&fakeModelMachinesWatcher{initial: []string{"foo"}},
		resources,
		authorizer,
	)
	result, err := e.WatchModelMachines(context.Background())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, params.StringsWatchResult{StringsWatcherId: "1", Changes: []string{"foo"}, Error: nil})
	c.Assert(resources.Count(), tc.Equals, 1)
}

func (s *modelMachinesWatcherSuite) TestWatchAuthError(c *tc.C) {
	authorizer := apiservertesting.FakeAuthorizer{
		Tag:        names.NewMachineTag("1"),
		Controller: false,
	}
	resources := common.NewResources()
	s.AddCleanup(func(_ *tc.C) { resources.StopAll() })
	e := model.NewModelMachinesWatcher(
		&fakeModelMachinesWatcher{},
		resources,
		authorizer,
	)
	_, err := e.WatchModelMachines(context.Background())
	c.Assert(err, tc.ErrorMatches, "permission denied")
	c.Assert(resources.Count(), tc.Equals, 0)
}

type fakeModelMachinesWatcher struct {
	state.ModelMachinesWatcher
	initial []string
}

type fakeStringsWatcher struct {
	changes chan []string
}

func (*fakeStringsWatcher) Stop() error {
	return nil
}

func (*fakeStringsWatcher) Kill() {}

func (*fakeStringsWatcher) Wait() error {
	return nil
}

func (*fakeStringsWatcher) Err() error {
	return nil
}

func (w *fakeStringsWatcher) Changes() <-chan []string {
	return w.changes
}
