// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machineundertaker

import (
	"context"
	stdcontext "context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/worker/v4"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/envcontext"
	"github.com/juju/juju/internal/worker/common"
)

// Facade defines the interface we require from the machine undertaker
// facade.
type Facade interface {
	WatchMachineRemovals(stdcontext.Context) (watcher.NotifyWatcher, error)
	AllMachineRemovals(stdcontext.Context) ([]names.MachineTag, error)
	GetProviderInterfaceInfo(stdcontext.Context, names.MachineTag) ([]network.ProviderInterfaceInfo, error)
	CompleteRemoval(stdcontext.Context, names.MachineTag) error
}

// AddressReleaser defines the interface we need from the environment
// networking.
type AddressReleaser interface {
	ReleaseContainerAddresses(envcontext.ProviderCallContext, []network.ProviderInterfaceInfo) error
}

// Undertaker is responsible for doing any provider-level
// cleanup needed and then removing the machine.
type Undertaker struct {
	API             Facade
	Releaser        AddressReleaser
	CallContextFunc common.CloudCallContextFunc
	Logger          logger.Logger
}

// NewWorker returns a machine undertaker worker that will watch for
// machines that need to be removed and remove them, cleaning up any
// necessary provider-level resources first.
func NewWorker(api Facade, env environs.Environ, credentialAPI common.CredentialAPI, logger logger.Logger) (worker.Worker, error) {
	envNetworking, _ := environs.SupportsNetworking(env)
	w, err := watcher.NewNotifyWorker(watcher.NotifyConfig{
		Handler: &Undertaker{
			API:             api,
			Releaser:        envNetworking,
			CallContextFunc: common.NewCloudCallContextFunc(credentialAPI),
			Logger:          logger,
		},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return w, nil
}

// SetUp (part of watcher.NotifyHandler) starts watching for machine
// removals.
func (u *Undertaker) SetUp(ctx stdcontext.Context) (watcher.NotifyWatcher, error) {
	u.Logger.Infof(context.TODO(), "setting up machine undertaker")
	return u.API.WatchMachineRemovals(ctx)
}

// Handle (part of watcher.NotifyHandler) cleans up provider resources
// and removes machines that have been marked for removal.
func (u *Undertaker) Handle(ctx stdcontext.Context) error {
	removals, err := u.API.AllMachineRemovals(ctx)
	if err != nil {
		return errors.Trace(err)
	}
	u.Logger.Debugf(context.TODO(), "handling removals: %v", removals)
	// TODO(babbageclunk): shuffle the removals so if there's a
	// problem with one others can still get past?
	for _, machine := range removals {
		err := u.MaybeReleaseAddresses(ctx, machine)
		if err != nil {
			u.Logger.Errorf(context.TODO(), "couldn't release addresses for %s: %s", machine, err)
			continue
		}
		err = u.API.CompleteRemoval(ctx, machine)
		if err != nil {
			u.Logger.Errorf(context.TODO(), "couldn't complete removal for %s: %s", machine, err)
		} else {
			u.Logger.Debugf(context.TODO(), "completed removal: %s", machine)
		}
	}
	return nil
}

// MaybeReleaseAddresses releases any addresses that have been
// allocated to this machine by the provider (if the provider supports
// that).
func (u *Undertaker) MaybeReleaseAddresses(ctx stdcontext.Context, machine names.MachineTag) error {
	if u.Releaser == nil {
		// This environ doesn't support releasing addresses.
		return nil
	}
	if !names.IsContainerMachine(machine.Id()) {
		// At the moment, only containers need their addresses releasing.
		return nil
	}
	interfaceInfos, err := u.API.GetProviderInterfaceInfo(ctx, machine)
	if err != nil {
		return errors.Trace(err)
	}
	if len(interfaceInfos) == 0 {
		u.Logger.Debugf(context.TODO(), "%s has no addresses to release", machine)
		return nil
	}
	err = u.Releaser.ReleaseContainerAddresses(u.CallContextFunc(stdcontext.Background()), interfaceInfos)
	// Some providers say they support networking but don't
	// actually support container addressing; don't freak out
	// about those.
	if errors.Is(err, errors.NotSupported) {
		u.Logger.Debugf(context.TODO(), "%s has addresses but provider doesn't support releasing them", machine)
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// TearDown (part of watcher.NotifyHandler) is an opportunity to stop
// or release any resources created in SetUp other than the watcher,
// which watcher.NotifyWorker takes care of for us.
func (u *Undertaker) TearDown() error {
	u.Logger.Infof(context.TODO(), "tearing down machine undertaker")
	return nil
}
