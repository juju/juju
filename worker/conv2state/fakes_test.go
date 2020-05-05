// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package conv2state

import (
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/names/v4"
)

type fakeMachiner struct {
	m      machine
	err    error
	gotTag names.MachineTag
}

func (f *fakeMachiner) Machine(tag names.MachineTag) (machine, error) {
	f.gotTag = tag
	return f.m, f.err
}

type fakeMachine struct {
	jobs     *params.JobsResult
	jobsErr  error
	watchErr error
	w        fakeWatcher
}

func (f fakeMachine) Jobs() (*params.JobsResult, error) {
	return f.jobs, f.jobsErr
}

func (f fakeMachine) Watch() (watcher.NotifyWatcher, error) {
	if f.watchErr == nil {
		return f.w, nil
	}
	return nil, f.watchErr
}

type fakeWatcher struct{}

func (fakeWatcher) Changes() watcher.NotifyChannel {
	return nil
}

func (fakeWatcher) Kill() {
}

func (fakeWatcher) Wait() error {
	return nil
}

type fakeAgent struct {
	tag names.Tag
}

func (f fakeAgent) Tag() names.Tag {
	return f.tag
}
