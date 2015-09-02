// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	apiaddresser "github.com/juju/juju/api/addresser"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.addresser")

type addresserHandler struct {
	api *apiaddresser.API
}

// NewWorker returns a worker that keeps track of IP address
// lifecycles, releaseing and removing dead addresses.
func NewWorker(api *apiaddresser.API) (worker.Worker, error) {
	ok, err := api.CanDeallocateAddresses()
	if err != nil {
		return nil, errors.Annotate(err, "checking address deallocation")
	}
	if !ok {
		// Environment does not support IP address
		// deallocation.
		logger.Debugf("address deallocation not supported; not starting worker")
		return worker.FinishedWorker{}, nil
	}
	ah := &addresserHandler{
		api: api,
	}
	aw := worker.NewStringsWorker(ah)
	return aw, nil
}

// SetUp is part of the StringsWorker interface.
func (a *addresserHandler) SetUp() (watcher.StringsWatcher, error) {
	// WatchIPAddresses returns an EntityWatcher which is a StringsWatcher.
	return a.api.WatchIPAddresses()
}

// TearDown is part of the StringsWorker interface.
func (a *addresserHandler) TearDown() error {
	return nil
}

// Handle is part of the Worker interface.
func (a *addresserHandler) Handle(watcherTags []string) error {
	// Changed IP address lifes are reported, clean them up.
	err := a.api.CleanupIPAddresses()
	if err != nil {
		// TryAgainError are already logged on server-side.
		// TODO(mue) Add a time based trigger for the cleanup
		// so that those to try again will be cleaned up even
		// without lifecycle changes of IP addresses.
		if !params.IsCodeTryAgain(err) {
			return errors.Annotate(err, "cannot cleanup IP addresses")
		}
	} else {
		logger.Tracef("released and removed dead IP addresses")
	}
	return nil
}
