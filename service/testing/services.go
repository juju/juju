// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils/set"

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

	CheckPassed bool
	Err         error
}

func NewFakeServices(init string) *FakeServices {
	return &FakeServices{
		init:        init,
		CheckPassed: true,
		Status: ServicesStatus{
			Running: set.NewStrings(),
			Enabled: set.NewStrings(),
			Managed: set.NewStrings(),
		},
	}
}

func (fs *FakeServices) InitSystem() string {
	return fs.init
}

func (fs *FakeServices) Start(name string) error {
	fs.Status.Running.Add(name)
	return fs.Err
}

func (fs *FakeServices) Stop(name string) error {
	fs.Status.Running.Remove(name)
	return fs.Err
}

func (fs *FakeServices) IsRunning(name string) (bool, error) {
	return fs.Status.Running.Contains(name), fs.Err
}

func (fs *FakeServices) Enable(name string) error {
	fs.Status.Enabled.Add(name)
	return fs.Err
}

func (fs *FakeServices) Disable(name string) error {
	fs.Status.Running.Remove(name)
	fs.Status.Enabled.Remove(name)
	return fs.Err
}

func (fs *FakeServices) ListEnabled() ([]string, error) {
	return fs.Status.Enabled.Values(), fs.Err
}

func (fs *FakeServices) IsEnabled(name string) (bool, error) {
	return fs.Status.Enabled.Contains(name), fs.Err
}

func (fs *FakeServices) Add(name string, conf common.Conf) error {
	fs.Status.Managed.Add(name)
	return fs.Err
}

func (fs *FakeServices) Remove(name string) error {
	fs.Status.Running.Remove(name)
	fs.Status.Enabled.Remove(name)
	fs.Status.Managed.Remove(name)
	return fs.Err
}

func (fs *FakeServices) Check(name string, conf common.Conf) (bool, error) {
	return fs.CheckPassed, fs.Err
}

func (fs *FakeServices) IsManaged(name string) bool {
	return fs.Status.Managed.Contains(name)
}

func (fs *FakeServices) Install(name string, conf common.Conf) error {
	fs.Status.Managed.Add(name)
	fs.Status.Enabled.Add(name)
	fs.Status.Running.Add(name)
	return fs.Err
}

func (fs *FakeServices) NewAgentService(tag names.Tag, paths service.AgentPaths, env map[string]string) (*service.Service, error) {
	svc, err := service.WrapAgentService(tag, paths, env, fs)
	return svc, errors.Trace(err)
}
