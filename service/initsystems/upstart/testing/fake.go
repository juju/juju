// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"io/ioutil"

	"github.com/juju/errors"
	"github.com/juju/utils/set"

	"github.com/juju/juju/service"
	"github.com/juju/juju/service/initsystems"
	"github.com/juju/juju/service/initsystems/upstart"
)

// TODO(ericsnow) Use the fake in the testing repo as soon as it lands.

type FakeInitSystem struct {
	Services map[string]initsystems.Conf
	Enabled  set.Strings
	Running  set.Strings
}

func NewFakeInitSystem() *FakeInitSystem {
	return &FakeInitSystem{
		Services: make(map[string]initsystems.Conf),
		Enabled:  set.NewStrings(),
		Running:  set.NewStrings(),
	}
}

func (fui *FakeInitSystem) Name() string {
	return service.InitSystemUpstart
}

func (fui *FakeInitSystem) List(include ...string) ([]string, error) {
	if len(include) == 0 {
		return fui.Enabled.Values(), nil
	}

	var names []string
	for _, name := range fui.Enabled.Values() {
		for _, included := range include {
			if name == included {
				names = append(names, name)
				break
			}
		}
	}
	return names, nil
}

func (fui *FakeInitSystem) Start(name string) error {
	if !fui.Enabled.Contains(name) {
		return errors.NotFoundf("service %q", name)
	}
	if fui.Running.Contains(name) {
		return errors.AlreadyExistsf("service %q", name)
	}

	fui.Running.Add(name)
	return nil
}

func (fui *FakeInitSystem) Stop(name string) error {
	if !fui.Enabled.Contains(name) {
		return errors.NotFoundf("service %q", name)
	}
	if !fui.Running.Contains(name) {
		return errors.NotFoundf("service %q", name)
	}

	fui.Running.Remove(name)
	return nil
}

func (fui *FakeInitSystem) Enable(name, filename string) error {
	if fui.Enabled.Contains(name) {
		return errors.AlreadyExistsf("service %q", name)
	}

	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return errors.Trace(err)
	}
	conf, err := fui.Deserialize(data)
	if err != nil {
		return errors.Trace(err)
	}

	fui.Services[name] = *conf
	fui.Enabled.Add(name)
	return nil
}

func (fui *FakeInitSystem) Disable(name string) error {
	if !fui.Enabled.Contains(name) {
		return errors.NotFoundf("service %q", name)
	}

	fui.Enabled.Remove(name)
	delete(fui.Services, name)
	return nil
}

func (fui *FakeInitSystem) IsEnabled(name string) (bool, error) {
	return fui.Enabled.Contains(name), nil
}

func (fui *FakeInitSystem) Info(name string) (*initsystems.ServiceInfo, error) {
	if !fui.Enabled.Contains(name) {
		return nil, errors.NotFoundf("service %q", name)
	}

	status := initsystems.StatusStopped
	if fui.Running.Contains(name) {
		status = initsystems.StatusRunning
	}

	conf := fui.Services[name]
	info := initsystems.ServiceInfo{
		Name:        name,
		Description: conf.Desc,
		Status:      status,
	}
	return &info, nil
}

func (fui *FakeInitSystem) Conf(name string) (*initsystems.Conf, error) {
	if !fui.Enabled.Contains(name) {
		return nil, errors.NotFoundf("service %q", name)
	}

	conf := fui.Services[name]
	return &conf, nil
}

func (fui *FakeInitSystem) Validate(name string, conf initsystems.Conf) error {
	return nil
}

func (fui *FakeInitSystem) Serialize(name string, conf initsystems.Conf) ([]byte, error) {
	data, err := upstart.Serialize(name, conf)
	return data, errors.Trace(err)
}

func (fui *FakeInitSystem) Deserialize(data []byte) (*initsystems.Conf, error) {
	conf, err := upstart.Deserialize(data)
	return conf, errors.Trace(err)
}
