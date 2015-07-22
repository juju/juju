// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	apiaddresser "github.com/juju/juju/api/addresser"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.addresser")

type releaser interface {
	// ReleaseAddress has the same signature as the same method in the
	// environs.Networking interface.
	ReleaseAddress(instance.Id, network.Id, network.Address, string) error
}

type addresserHandler struct {
	api      *apiaddresser.API
	releaser releaser
}

// NewWorker returns a worker that keeps track of
// IP address lifecycles, releaseing and removing Dead addresses.
func NewWorker(api *apiaddresser.API) (worker.Worker, error) {
	config, err := api.EnvironConfig()
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
	aw := newWorkerWithReleaser(api, netEnviron)
	return aw, nil
}

// newWorkerWithReleaser creates a worker with the addresser handler.
// It's an own helper function to be exported for tests.
func newWorkerWithReleaser(api *apiaddresser.API, releaser releaser) worker.Worker {
	a := &addresserHandler{
		api:      api,
		releaser: releaser,
	}
	w := worker.NewStringsWorker(a)
	return w
}

// Handle is part of the StringsWorker interface.
func (a *addresserHandler) Handle(watcherTags []string) error {
	if a.releaser == nil {
		return nil
	}
	// Convert received tag strings into tags.
	tags := make([]names.IPAddressTags, len(watcherTags))
	for i, watcherTag := range watcherTags {
		tag, err := name.ParseIPAddressTag(watcherTag)
		if err != nil {
			return errors.Annotatef(err, "cannot parse IP address tag %q", watcherTag)
		}
		tags[i] = tag
	}
	// Retrieve IP addresses and process them.
	ipAddresses, err := a.api.IPAddresses(tags...)
	if err != nil {
		if err != common.ErrPartialResults {
			return errors.Annotate(err, "cannot retrieve IP addresses")
		}
	}
	toBeRemoved := []*apiaddresser.IPAddress
	for i, ipAddress := range ipAddresses {
		tag := tags[i]
		if ipAddress == nil {
			logger.Debugf("IP address %v was removed; skipping", tag)
			continue
		}
		if ipAddress.Life() != state.Dead {
			logger.Debugf("IP address %v is not Dead (life %q); skipping", tag, ipAddress.Life())
			continue
		}
		if err := a.releaseIPAddress(addr); err != nil {
			return errors.Annotatef(err, "cannot release IP address %v", tag)
		}
		logger.Debugf("IP address %v released", tag)
		toBeRemoved = append(toBeRemoved, ipAddress)
	}
	// Finally remove the release ones.
	if err := a.api.Remove(toBeRemoved...); err != nil {
		return errors.Annotate(err, "cannot remove all released IP addresses")
	}
	logger.Debugf("removed released IP addresses")
	return nil
}

func (a *addresserHandler) releaseIPAddress(ipAddress *apiaddresser.IPAddress) (err error) {
	defer errors.DeferredAnnotatef(&err, "failed to release address %v", addr.Value())
	logger.Debugf("attempting to release dead address %#v", addr.Value())

	subnetId := network.Id(addr.SubnetId())
	for attempt := common.ShortAttempt.Start(); attempt.Next(); {
		err = a.releaser.ReleaseAddress(addr.InstanceId(), subnetId, addr.Address(), addr.MACAddress())
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
func (a *addresserHandler) SetUp() (watcher.StringsWatcher, error) {
	// WatchIPAddresses returns an EntityWatche which is a StringsWatcher.
	return a.st.WatchIPAddresses(), nil
}

// TearDown is part of the StringsWorker interface.
func (a *addresserHandler) TearDown() error {
	return nil
}
