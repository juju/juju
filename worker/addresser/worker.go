// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	apiWatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.addresser")

type releaser interface {
	// ReleaseAddress has the same signature as the same method in the
	// environs.Networking interface.
	ReleaseAddress(instance.Id, network.Id, network.Address) error
}

// stateAddresser defines the State methods used by the addresserHandler
type stateAddresser interface {
	DeadIPAddresses() ([]*state.IPAddress, error)
	EnvironConfig() (*config.Config, error)
	IPAddress(string) (*state.IPAddress, error)
	Machine(string) (*state.Machine, error)
	WatchIPAddresses() state.StringsWatcher
}

type addresserHandler struct {
	st       stateAddresser
	releaser releaser
}

// NewWorker returns a worker that keeps track of
// IP address lifecycles, releaseing and removing Dead addresses.
func NewWorker(st stateAddresser) (worker.Worker, error) {
	config, err := st.EnvironConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	environ, err := environs.New(config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// If netEnviron is nil the worker will start but won't do anything as
	// no IP addresses will be created or destroyed.
	netEnviron, _ := environs.SupportsNetworking(environ)
	a := newWorkerWithReleaser(st, netEnviron)
	return a, nil
}

func newWorkerWithReleaser(st stateAddresser, releaser releaser) worker.Worker {
	a := &addresserHandler{
		st:       st,
		releaser: releaser,
	}
	w := worker.NewStringsWorker(a)
	return w
}

// Handle is part of the StringsWorker interface.
func (a *addresserHandler) Handle(ids []string) error {
	if a.releaser == nil {
		return nil
	}
	for _, id := range ids {
		logger.Debugf("received notification about address %v", id)
		addr, err := a.st.IPAddress(id)
		if err != nil {
			if errors.IsNotFound(err) {
				logger.Debugf("address %v was removed; skipping", id)
				continue
			}
			return err
		}
		if addr.Life() != state.Dead {
			logger.Debugf("address %v is not Dead (life %q); skipping", id, addr.Life())
			continue
		}
		err = a.releaseIPAddress(addr)
		if err != nil {
			return err
		}
		logger.Debugf("address %v released", id)
		err = addr.Remove()
		if err != nil {
			return err
		}
		logger.Debugf("address %v removed", id)
	}
	return nil
}

func (a *addresserHandler) releaseIPAddress(addr *state.IPAddress) (err error) {
	defer errors.DeferredAnnotatef(&err, "failed to release address %v", addr.Value())
	logger.Debugf("attempting to release dead address %#v", addr.Value())

	subnetId := network.Id(addr.SubnetId())
	for attempt := common.ShortAttempt.Start(); attempt.Next(); {
		err = a.releaser.ReleaseAddress(addr.InstanceId(), subnetId, addr.Address())
		if err == nil {
			return nil
		}
	}
	// Don't remove the address from state so we
	// can retry releasing the address later.
	logger.Warningf("cannot release address %q: %v (will retry)", addr.Value(), err)
	return errors.Trace(err)
}

// SetUp is part of the StringsWorker interface.
func (a *addresserHandler) SetUp() (apiWatcher.StringsWatcher, error) {
	return a.st.WatchIPAddresses(), nil
}

// TearDown is part of the StringsWorker interface.
func (a *addresserHandler) TearDown() error {
	return nil
}
