// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watchers

import (
	"github.com/juju/loggo"

	"github.com/juju/juju/apiserver/facade"
)

var (
	logger = loggo.GetLogger("juju.apiserver.watchers")
)

type watcherCommon struct {
	id              string
	watcherRegistry facade.WatcherRegistry
	dispose         func()
}

func newWatcherCommon(context facade.Context) watcherCommon {
	return watcherCommon{
		id:              context.ID(),
		watcherRegistry: context.WatcherRegistry(),
		dispose:         context.Dispose,
	}
}

// Stop stops the watcher.
func (w *watcherCommon) Stop() error {
	w.dispose()
	return w.watcherRegistry.Stop(w.id)
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
