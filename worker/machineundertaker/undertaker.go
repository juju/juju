// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineundertaker

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/network"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.machineundertaker")

// Facade defines the interface we require from the machine undertaker
// facade.
type Facade interface {
	WatchMachineRemovals() (watcher.NotifyWatcher, error)
	AllMachineRemovals() ([]names.MachineTag, error)
	GetProviderInterfaceInfo(names.MachineTag) ([]network.ProviderInterfaceInfo, error)
	CompleteRemoval(names.MachineTag) error
}

// MachineUndertaker is responsible for doing any provider-level
// cleanup needed and then removing the machine.
type Undertaker struct {
	api           Facade
	envNetworking environs.Networking
}

func NewWorker(api Facade, env environs.Environ) (worker.Worker, error) {
	envNetworking, _ := environs.SupportsNetworking(env)
	w, err := watcher.NewNotifyWorker(watcher.NotifyConfig{
		Handler: &Undertaker{api: api, envNetworking: envNetworking},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

func (u *Undertaker) SetUp() (watcher.NotifyWatcher, error) {
	logger.Infof("setting up machine undertaker")
	return u.api.WatchMachineRemovals()
}

func (u *Undertaker) Handle(<-chan struct{}) error {
	removals, err := u.api.AllMachineRemovals()
	if err != nil {
		// Should this be a fatal error instead?
		logger.Errorf("couldn't get machine removals, %s", err)
		return nil
	}
	logger.Debugf("handling removals: %v", removals)
	// TODO(babbageclunk): shuffle the removals so if there's a
	// problem with one others can still get past.
	for _, machine := range removals {
		err := u.maybeReleaseAddresses(machine)
		if err != nil {
			logger.Errorf("couldn't release addresses for %s: %s", machine, err)
			continue
		}
		err = u.api.CompleteRemoval(machine)
		if err != nil {
			logger.Errorf("couldn't complete removal for %s: %s", machine, err)
		} else {
			logger.Debugf("completed removal: %s", machine)
		}
	}
	return nil
}

func (u *Undertaker) maybeReleaseAddresses(machine names.MachineTag) error {
	if u.envNetworking == nil {
		// This environ doesn't support releasing addresses.
		return nil
	}
	if !names.IsContainerMachine(machine.Id()) {
		// Only containers need their addresses releasing.
		return nil
	}
	interfaceInfos, err := u.api.GetProviderInterfaceInfo(machine)
	if err != nil {
		return errors.Trace(err)
	}
	if len(interfaceInfos) == 0 {
		logger.Debugf("%s has no addresses to release", machine)
		return nil
	}
	err = u.envNetworking.ReleaseContainerAddresses(interfaceInfos)
	// Some providers say they support networking but don't
	// actually support container addressing; don't freak out
	// about those.
	if err != nil && !errors.IsNotSupported(err) {
		return errors.Trace(err)
	}
	return nil
}

func (u *Undertaker) TearDown() error {
	logger.Infof("tearing down machine undertaker")
	return nil
}
