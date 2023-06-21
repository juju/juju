// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watchers

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/worker/v3"

	"github.com/juju/juju/apiserver/facade"
)

var (
	logger = loggo.GetLogger("juju.apiserver.watchers")
)

type watcherCommon struct {
	id              string
	watcherRegistry facade.WatcherRegistry
	resources       facade.Resources
	dispose         func()
}

func newWatcherCommon(context facade.Context) watcherCommon {
	return watcherCommon{
		id:              context.ID(),
		watcherRegistry: context.WatcherRegistry(),
		resources:       context.Resources(),
		dispose:         context.Dispose,
	}
}

// Stop stops the watcher.
func (w *watcherCommon) Stop() error {
	w.dispose()
	if _, err := w.watcherRegistry.Get(w.id); err == nil {
		return w.watcherRegistry.Stop(w.id)
	}
	return w.resources.Stop(w.id)
}

func isAgent(auth facade.Authorizer) bool {
	return auth.AuthMachineAgent() || auth.AuthUnitAgent() || auth.AuthApplicationAgent() || auth.AuthModelAgent()
}

func isAgentOrUser(auth facade.Authorizer) bool {
	return isAgent(auth) || auth.AuthClient()
}

// state watchers have an Err method, but cache watchers do not.
type hasErr interface {
	Err() error
}

// GetWatcherByID returns a watcher by id, first looking in the watcher registry
// and then in the deprecated resources. Eventually, the resources will be
// removed and lookup via the watcherRegistry will be required.
func GetWatcherByID(watcherRegistry facade.WatcherRegistry, resources facade.Resources, id string) (worker.Worker, error) {
	watcher, err := watcherRegistry.Get(id)
	if err != nil {
		if errors.Is(err, errors.NotFound) {
			watcher = resources.Get(id)
		} else {
			return nil, errors.Trace(err)
		}
	}
	return watcher, nil
}
