// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineundertaker

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/worker/v2"

	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/network"
	"github.com/juju/juju/worker/common"
)

// Logger is here to stop the desire of creating a package level Logger.
// Don't do this, instead use the one passed as manifold config.
var logger interface{}

// Facade defines the interface we require from the machine undertaker
// facade.
type Facade interface {
	WatchMachineRemovals() (watcher.NotifyWatcher, error)
	AllMachineRemovals() ([]names.MachineTag, error)
	GetProviderInterfaceInfo(names.MachineTag) ([]network.ProviderInterfaceInfo, error)
	CompleteRemoval(names.MachineTag) error
}

// AddressReleaser defines the interface we need from the environment
// networking.
type AddressReleaser interface {
	ReleaseContainerAddresses(context.ProviderCallContext, []network.ProviderInterfaceInfo) error
}

// Undertaker is responsible for doing any provider-level
// cleanup needed and then removing the machine.
type Undertaker struct {
	API         Facade
	Releaser    AddressReleaser
	CallContext context.ProviderCallContext
	Logger      Logger
}

// NewWorker returns a machine undertaker worker that will watch for
// machines that need to be removed and remove them, cleaning up any
// necessary provider-level resources first.
func NewWorker(api Facade, env environs.Environ, credentialAPI common.CredentialAPI, logger Logger) (worker.Worker, error) {
	envNetworking, _ := environs.SupportsNetworking(env)
	w, err := watcher.NewNotifyWorker(watcher.NotifyConfig{
		Handler: &Undertaker{
			API:         api,
			Releaser:    envNetworking,
			CallContext: common.NewCloudCallContext(credentialAPI, nil),
			Logger:      logger,
		},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// SetUp (part of watcher.NotifyHandler) starts watching for machine
// removals.
func (u *Undertaker) SetUp() (watcher.NotifyWatcher, error) {
	u.Logger.Infof("setting up machine undertaker")
	return u.API.WatchMachineRemovals()
}

// Handle (part of watcher.NotifyHandler) cleans up provider resources
// and removes machines that have been marked for removal.
func (u *Undertaker) Handle(<-chan struct{}) error {
	removals, err := u.API.AllMachineRemovals()
	if err != nil {
		return errors.Trace(err)
	}
	u.Logger.Debugf("handling removals: %v", removals)
	// TODO(babbageclunk): shuffle the removals so if there's a
	// problem with one others can still get past?
	for _, machine := range removals {
		err := u.MaybeReleaseAddresses(machine)
		if err != nil {
			u.Logger.Errorf("couldn't release addresses for %s: %s", machine, err)
			continue
		}
		err = u.API.CompleteRemoval(machine)
		if err != nil {
			u.Logger.Errorf("couldn't complete removal for %s: %s", machine, err)
		} else {
			u.Logger.Debugf("completed removal: %s", machine)
		}
	}
	return nil
}

// MaybeReleaseAddresses releases any addresses that have been
// allocated to this machine by the provider (if the provider supports
// that).
func (u *Undertaker) MaybeReleaseAddresses(machine names.MachineTag) error {
	if u.Releaser == nil {
		// This environ doesn't support releasing addresses.
		return nil
	}
	if !names.IsContainerMachine(machine.Id()) {
		// At the moment, only containers need their addresses releasing.
		return nil
	}
	interfaceInfos, err := u.API.GetProviderInterfaceInfo(machine)
	if err != nil {
		return errors.Trace(err)
	}
	if len(interfaceInfos) == 0 {
		u.Logger.Debugf("%s has no addresses to release", machine)
		return nil
	}
	err = u.Releaser.ReleaseContainerAddresses(u.CallContext, interfaceInfos)
	// Some providers say they support networking but don't
	// actually support container addressing; don't freak out
	// about those.
	if errors.IsNotSupported(err) {
		u.Logger.Debugf("%s has addresses but provider doesn't support releasing them", machine)
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// TearDown (part of watcher.NotifyHandler) is an opportunity to stop
// or release any resources created in SetUp other than the watcher,
// which watcher.NotifyWorker takes care of for us.
func (u *Undertaker) TearDown() error {
	u.Logger.Infof("tearing down machine undertaker")
	return nil
}
