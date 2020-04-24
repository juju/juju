// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrevisionupdater

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/state"
)

// LatestCharmHandler exposes the functionality needed to deal with
// the latest info (from the store) for a charm.
type LatestCharmHandler interface {
	// HandleLatest deals with the given charm info, treating it as the
	// most up-to-date information for the charms most recent revision.
	HandleLatest(names.ApplicationTag, charmstore.CharmInfo) error
}

type newHandlerFunc func(*state.State) (LatestCharmHandler, error)

var registeredHandlers = map[string]newHandlerFunc{}

// RegisterLatestCharmHandler adds the factory func for the identified
// handler to the handler registry.
func RegisterLatestCharmHandler(name string, newHandler newHandlerFunc) error {
	if _, ok := registeredHandlers[name]; ok {
		msg := fmt.Sprintf(`"latest charm" handler %q already registered`, name)
		return errors.NewAlreadyExists(nil, msg)
	}
	registeredHandlers[name] = newHandler
	return nil
}

func createHandlers(st *state.State) ([]LatestCharmHandler, error) {
	var handlers []LatestCharmHandler
	for _, newHandler := range registeredHandlers {
		handler, err := newHandler(st)
		if err != nil {
			return nil, errors.Trace(err)
		}
		handlers = append(handlers, handler)
	}
	return handlers, nil
}
