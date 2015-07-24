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
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.addresser")

type addresserHandler struct {
	api *apiaddresser.API
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
	ah := &addresserHandler{
		api:      api,
	}
	w := worker.NewStringsWorker(ah)
	return aw, nil
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
	toBeReleased := []*apiaddresser.IPAddress
	for i, ipAddress := range ipAddresses {
		tag := tags[i]
		if ipAddress == nil {
			logger.Debugf("IP address %v already has been removed; skipping", tag)
			continue
		}
		if ipAddress.Life() != state.Dead {
			logger.Debugf("IP address %v is not dead (life %q); skipping", tag, ipAddress.Life())
			continue
		}
		toBeReleased = append(toBeReleased, ipAddress)
	}
	// Release the IP addresses.
	if err := a.api.ReleaseIPAddresses(toBeReleased...); err != nil {
		return errors.Annotate(err, "cannot release all IP addresses")
	}
	logger.Debugf("IP address %v released", tag)
	// Finally remove the released ones.
	if err := a.api.Remove(toBeReleased...); err != nil {
		return errors.Annotate(err, "cannot remove all released IP addresses")
	}
	logger.Debugf("removed released IP addresses")
	return nil
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
