// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package conv2state

import (
	"github.com/juju/names"

	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
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

func (fakeWatcher) Changes() <-chan struct{} {
	return nil
}

func (fakeWatcher) Stop() error {
	return nil
}

func (fakeWatcher) Err() error {
	return nil
}

type fakeAgent struct {
	tag        names.Tag
	restartErr error
	didRestart bool
}

func (f *fakeAgent) Restart() error {
	f.didRestart = true
	return f.restartErr
}

func (f fakeAgent) Tag() names.Tag {
	return f.tag
}
