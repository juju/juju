// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"launchpad.net/juju-core/state/api/params"
)

// AllWatcher holds information allowing us to get Deltas describing changes
// to the entire environment.
type AllWatcher struct {
	client *Client
	id     *string
}

func newAllWatcher(client *Client, id *string) *AllWatcher {
	return &AllWatcher{client, id}
}

func (watcher *AllWatcher) Next() ([]params.Delta, error) {
	info := new(params.AllWatcherNextResults)
	err := watcher.client.st.Call("AllWatcher", *watcher.id, "Next", nil, info)
	return info.Deltas, err
}

func (watcher *AllWatcher) Stop() error {
	return watcher.client.st.Call("AllWatcher", *watcher.id, "Stop", nil, nil)
}
