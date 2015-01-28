// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
)

type ServicesStatus struct {
	Running set.Strings
	Enabled set.Strings
	Managed set.Strings
}

type FakeServices struct {
	init   string
	Status ServicesStatus
	Confs  map[string]common.Conf

	calls []string

	CheckPassed []bool
	Errors      []error
}

func NewFakeServices(init string) *FakeServices {
	return &FakeServices{
		init:  init,
		Confs: make(map[string]common.Conf),
		Status: ServicesStatus{
			Running: set.NewStrings(),
			Enabled: set.NewStrings(),
			Managed: set.NewStrings(),
		},
	}
}

func (fs *FakeServices) addCall(name string) {
	fs.calls = append(fs.calls, name)
}

func (fs *FakeServices) err() error {
	if len(fs.Errors) == 0 {
		return nil
	}
	err := fs.Errors[0]
	fs.Errors = fs.Errors[1:]
	return err
}

func (fs *FakeServices) CheckCalls(c *gc.C, expected ...string) {
	c.Check(fs.calls, gc.DeepEquals, expected)
}

func (fs *FakeServices) ResetCalls() {
	fs.calls = nil
}

func (fs *FakeServices) InitSystem() string {
	fs.addCall("InitSystem")

	return fs.init
}

func (fs *FakeServices) List() ([]string, error) {
	fs.addCall("List")

	return fs.Status.Managed.Values(), fs.err()
}

func (fs *FakeServices) ListEnabled() ([]string, error) {
	fs.addCall("ListEnabled")

	return fs.Status.Enabled.Values(), fs.err()
}

func (fs *FakeServices) Start(name string) error {
	fs.addCall("Start")

	fs.Status.Running.Add(name)
	return fs.err()
}

func (fs *FakeServices) Stop(name string) error {
	fs.addCall("Stop")

	fs.Status.Running.Remove(name)
	return fs.err()
}

func (fs *FakeServices) IsRunning(name string) (bool, error) {
	fs.addCall("IsRunning")

	return fs.Status.Running.Contains(name), fs.err()
}

func (fs *FakeServices) Enable(name string) error {
	fs.addCall("Enable")

	fs.Status.Enabled.Add(name)
	return fs.err()
}

func (fs *FakeServices) Disable(name string) error {
	fs.addCall("Disable")

	fs.Status.Running.Remove(name)
	fs.Status.Enabled.Remove(name)
	return fs.err()
}

func (fs *FakeServices) IsEnabled(name string) (bool, error) {
	fs.addCall("IsEnabled")

	return fs.Status.Enabled.Contains(name), fs.err()
}

func (fs *FakeServices) Add(name string, conf common.Conf) error {
	fs.addCall("Add")
	if err := fs.err(); err != nil {
		return err
	}
	if fs.Status.Managed.Contains(name) {
		return errors.AlreadyExistsf("service %q", name)
	}

	fs.Status.Managed.Add(name)
	fs.Confs[name] = conf
	return fs.err()
}

func (fs *FakeServices) Remove(name string) error {
	fs.addCall("Remove")

	fs.Status.Running.Remove(name)
	fs.Status.Enabled.Remove(name)
	fs.Status.Managed.Remove(name)
	return fs.err()
}

func (fs *FakeServices) Check(name string, conf common.Conf) (bool, error) {
	fs.addCall("Check")

	passed := true
	if len(fs.CheckPassed) > 0 {
		passed = fs.CheckPassed[0]
		fs.CheckPassed = fs.CheckPassed[1:]
	}
	return passed, fs.err()
}

func (fs *FakeServices) IsManaged(name string) bool {
	fs.addCall("IsManaged")

	return fs.Status.Managed.Contains(name)
}

func (fs *FakeServices) Install(name string, conf common.Conf) error {
	fs.addCall("Install")

	fs.Status.Managed.Add(name)
	fs.Status.Enabled.Add(name)
	fs.Status.Running.Add(name)
	return fs.err()
}

func (fs *FakeServices) NewAgentService(tag names.Tag, paths service.AgentPaths, env map[string]string) (*service.Service, error) {
	fs.addCall("NewAgentService")

	svc, err := service.WrapAgentService(tag, paths, env, fs)
	return svc, errors.Trace(err)
}
