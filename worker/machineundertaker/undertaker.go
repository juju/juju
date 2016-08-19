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
	api Facade
	env environs.Environ
}

func NewWorker(api Facade, env environs.Environ) (worker.Worker, error) {
	w, err := watcher.NewNotifyWorker(watcher.NotifyConfig{
		Handler: &Undertaker{api: api, env: env},
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
	logger.Infof("handling removals of %v, %v", removals, err)
	return nil
}

func (u *Undertaker) TearDown() error {
	logger.Infof("tearing down machine undertaker")
	return nil
}
