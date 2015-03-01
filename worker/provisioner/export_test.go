// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"reflect"

	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/environs/config"
)

func SetObserver(p Provisioner, observer chan<- *config.Config) {
	ep := p.(*environProvisioner)
	ep.Lock()
	ep.observer = observer
	ep.Unlock()
}

func GetRetryWatcher(p Provisioner) (watcher.NotifyWatcher, error) {
	return p.getRetryWatcher()
}

var (
	ContainerManagerConfig = containerManagerConfig
	GetToolsFinder         = &getToolsFinder
	SysctlConfig           = &sysctlConfig
)

const IPForwardSysctlKey = ipForwardSysctlKey

// SetIPForwarding calls the internal setIPForwarding and then
// restores the mocked one.
var SetIPForwarding func(bool) error

func init() {
	// In order to isolate the host machine from the running tests,
	// but also allow calling the original setIPForwarding func to
	// test it, we need a litte bit of reflect magic, mostly borrowed
	// from the juju/testing pacakge.
	newv := reflect.ValueOf(&setIPForwarding).Elem()
	oldv := reflect.New(newv.Type()).Elem()
	oldv.Set(newv)
	mockv := reflect.ValueOf(func(bool) error { return nil })
	restore := func() { newv.Set(oldv) }
	remock := func() { newv.Set(mockv) }
	remock()

	SetIPForwarding = func(v bool) error {
		restore()
		defer remock()
		return setIPForwarding(v)
	}
}
