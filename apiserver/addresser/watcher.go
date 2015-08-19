// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser

import (
	"github.com/juju/errors"

	"github.com/juju/juju/state"
)

// ipAddressesWatcher implements an entity watcher with the transformation
// of the received document ids of IP addresses into their according tags.
type ipAddressesWatcher struct {
	state.StringsWatcher

	st StateInterface
}

// MapChanges converts IP address values to tags.
func (w *ipAddressesWatcher) MapChanges(in []string) ([]string, error) {
	if len(in) == 0 {
		return in, nil
	}
	mapped := make([]string, len(in))
	for i, v := range in {
		ipAddr, err := w.st.IPAddress(v)
		if err != nil {
			return nil, errors.Annotate(err, "cannot fetch address")
		}
		mapped[i] = ipAddr.Tag().String()
	}
	return mapped, nil
}
