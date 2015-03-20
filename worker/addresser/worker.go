// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	apiWatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.addresser")

type releaser interface {
	// ReleaseAddress has the same signature as the same method in the
	// NetworkingEnviron.
	ReleaseAddress(instId instance.Id, subnetId network.Id, addr network.Address) error
}

type addresserHandler struct {
	dying    chan struct{}
	st       *state.State
	releaser releaser
}

// NewWorker returns a worker that keeps track of
// IP address lifecycles, removing Dead addresses.
func NewWorker(st *state.State) (worker.Worker, error) {
	config, err := st.EnvironConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	environ, err := environs.New(config)
	if err != nil {
		return nil, errors.Trace(err)
	}
	netEnviron, ok := environs.SupportsNetworking(environ)
	if !ok {
		return nil, errors.New("environment does not support networking")
	}
	a := NewWorkerWithReleaser(st, netEnviron)
	return a, nil
}

func NewWorkerWithReleaser(st *state.State, releaser releaser) worker.Worker {
	a := &addresserHandler{
		st:       st,
		releaser: releaser,
		dying:    make(chan struct{}),
	}
	w := worker.NewStringsWorker(a)
	return w
}

func (a *addresserHandler) Handle(ids []string) error {
	for _, id := range ids {
		addr, err := a.st.IPAddress(id)
		if err != nil {
			return err
		}
		if addr.Life() != state.Dead {
			continue
		}
		err = a.removeIPAddress(addr)
		if err != nil {
			return err
		}
	}
	return nil
}

func (a *addresserHandler) removeIPAddress(addr *state.IPAddress) (err error) {
	defer errors.DeferredAnnotatef(&err, "failed to release address %v", addr.Value)
	machine, err := a.st.Machine(addr.MachineId())
	if err != nil {
		errors.Annotatef(err, "failed to remove address %v", addr.Value)
		return err
	}
	instId, err := machine.InstanceId()
	if err != nil {
		errors.Annotatef(err, "failed to remove address %v", addr.Value)
		return err
	}
	err = a.releaser.ReleaseAddress(instId, network.Id(addr.SubnetId()), addr.Address())
	if err != nil {
		// Don't remove the address from state so we
		// can retry releasing the address later.
		errors.Annotatef(err, "failed to remove address %v", addr.Value)
		return err
	}

	err = addr.Remove()
	if err != nil {
		errors.Annotatef(err, "failed to remove address %v", addr.Value)
		return err
	}
	return nil
}

func (a *addresserHandler) SetUp() (apiWatcher.StringsWatcher, error) {
	w := a.st.WatchIPAddresses()
	dead, err := a.st.DeadIPAddresses()
	if err != nil {
		return w, errors.Trace(err)
	}
	deadQueue := make(chan *state.IPAddress, len(dead))
	for _, deadAddr := range dead {
		deadQueue <- deadAddr
	}
	go func() {
		select {
		case addr := <-deadQueue:
			err := a.removeIPAddress(addr)
			if err != nil {
				logger.Warningf("error releasing dead IP address %q: %v", addr, err)
			}
		case <-a.dying:
			return
		default:
			return
		}
	}()
	return w, nil
}

func (a *addresserHandler) TearDown() error {
	close(a.dying)
	return nil
}
