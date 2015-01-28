// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"io/ioutil"

	"github.com/juju/errors"
	"github.com/juju/utils/set"

	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
	"github.com/juju/juju/service/upstart"
)

type FakeUpstartInit struct {
	Services map[string]common.Conf
	Enabled  set.Strings
	Running  set.Strings
}

func NewFakeUpstartInit() *FakeUpstartInit {
	return &FakeUpstartInit{
		Services: make(map[string]common.Conf),
		Enabled:  set.NewStrings(),
		Running:  set.NewStrings(),
	}
}

func (fui *FakeUpstartInit) Name() string {
	return service.InitSystemUpstart
}

func (fui *FakeUpstartInit) List(include ...string) ([]string, error) {
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

func (fui *FakeUpstartInit) Start(name string) error {
	if !fui.Enabled.Contains(name) {
		return errors.NotFoundf("service %q", name)
	}
	if fui.Running.Contains(name) {
		return errors.AlreadyExistsf("service %q", name)
	}

	fui.Running.Add(name)
	return nil
}

func (fui *FakeUpstartInit) Stop(name string) error {
	if !fui.Enabled.Contains(name) {
		return errors.NotFoundf("service %q", name)
	}
	if !fui.Running.Contains(name) {
		return errors.NotFoundf("service %q", name)
	}

	fui.Running.Remove(name)
	return nil
}

func (fui *FakeUpstartInit) Enable(name, filename string) error {
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

func (fui *FakeUpstartInit) Disable(name string) error {
	if !fui.Enabled.Contains(name) {
		return errors.NotFoundf("service %q", name)
	}

	fui.Enabled.Remove(name)
	delete(fui.Services, name)
	return nil
}

func (fui *FakeUpstartInit) IsEnabled(name string) (bool, error) {
	return fui.Enabled.Contains(name), nil
}

func (fui *FakeUpstartInit) Info(name string) (*common.ServiceInfo, error) {
	if !fui.Enabled.Contains(name) {
		return nil, errors.NotFoundf("service %q", name)
	}

	status := common.StatusStopped
	if fui.Running.Contains(name) {
		status = common.StatusRunning
	}

	conf := fui.Services[name]
	info := common.ServiceInfo{
		Name:        name,
		Description: conf.Desc,
		Status:      status,
	}
	return &info, nil
}

func (fui *FakeUpstartInit) Conf(name string) (*common.Conf, error) {
	if !fui.Enabled.Contains(name) {
		return nil, errors.NotFoundf("service %q", name)
	}

	conf := fui.Services[name]
	return &conf, nil
}

func (fui *FakeUpstartInit) Serialize(name string, conf common.Conf) ([]byte, error) {
	data, err := upstart.Serialize(name, conf)
	return data, errors.Trace(err)
}

func (fui *FakeUpstartInit) Deserialize(data []byte) (*common.Conf, error) {
	conf, err := upstart.Deserialize(data)
	return conf, errors.Trace(err)
}
