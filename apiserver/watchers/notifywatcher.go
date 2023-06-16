// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watchers

import (
	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/state"
)

// srvNotifyWatcher defines the API access to methods on a NotifyWatcher.
// Each client has its own current set of watchers, stored in resources.
type srvNotifyWatcher struct {
	watcherCommon
	watcher state.NotifyWatcher
}

func NewNotifyWatcher(context facade.Context) (facade.Facade, error) {
	var (
		id              = context.ID()
		auth            = context.Auth()
		watcherRegistry = context.WatcherRegistry()
	)

	// TODO(wallyworld) - enhance this watcher to support
	// anonymous api calls with macaroons.
	if auth.GetAuthTag() != nil && !isAgentOrUser(auth) {
		return nil, apiservererrors.ErrPerm
	}

	watcher, err := watcherRegistry.Get(id)
	if err != nil {
		return nil, errors.Trace(err)
	}

	notifyWatcher, ok := watcher.(state.NotifyWatcher)
	if !ok {
		return nil, apiservererrors.ErrUnknownWatcher
	}

	return &srvNotifyWatcher{
		watcherCommon: newWatcherCommon(context),
		watcher:       notifyWatcher,
	}, nil
}

// Next returns when a change has occurred to the
// entity being watched since the most recent call to Next
// or the Watch call that created the NotifyWatcher.
func (w *srvNotifyWatcher) Next() error {
	if _, ok := <-w.watcher.Changes(); ok {
		return nil
	}

	var err error
	if e, ok := w.watcher.(hasErr); ok {
		err = e.Err()
	}
	if err == nil {
		err = apiservererrors.ErrStoppedWatcher
	}
	return err
}
