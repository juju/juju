// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"fmt"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/core/watcher/watchertest"
)

// NamespaceWatcherFactory implements a simple NamespaceWatcher creation process
// for tests. It supports channels of changes on to multiple namespaces.
// Channels created on this factory are buffered by 1 slot. This means that data
// needs to be consumed from the watcher before the next lot of data is placed
// to avoid a single goroutine lock.
type NamespaceWatcherFactory struct {
	InitialStateFunc func() ([]string, error)
	WatcherChans     map[string]chan []string
}

// Close must be called after using this factory to safely clean up all the
// channels.
func (f *NamespaceWatcherFactory) Close() {
	for _, ch := range f.WatcherChans {
		close(ch)
	}
}

// FeedChange feeds the supplied change for the given namespace into any
// watchers that are open. If no watchers exist this func call evaluates to a
// noop. FeedChange will block until the watches channel can accept the data or
// the context is canceled.
func (f *NamespaceWatcherFactory) FeedChange(
	ctx context.Context,
	namespace string,
	_ changestream.ChangeType,
	change []string,
) error {
	ch, exists := f.WatcherChans[namespace]
	if !exists {
		return nil
	}

	select {
	case <-ctx.Done():
		return fmt.Errorf("unable to feed watcher change for namespace %q: %w", namespace, ctx.Err())
	case ch <- change:
	}
	return nil
}

// NewNamespaceWatcher implements WatcherFactory.NewNamespaceWatcher
func (f *NamespaceWatcherFactory) NewNamespaceWatcher(
	initialStateQuery eventsource.NamespaceQuery,
	filterOption eventsource.FilterOption, filterOptions ...eventsource.FilterOption,
) (watcher.StringsWatcher, error) {
	if len(filterOptions) > 0 {
		return nil, fmt.Errorf("filter options are not supported in testing")
	}

	namespace := filterOption.Namespace()
	ch, exists := f.WatcherChans[namespace]
	if !exists {
		f.WatcherChans[namespace] = make(chan []string, 1)
		ch = f.WatcherChans[namespace]
	}
	if f.InitialStateFunc != nil {
		state, err := f.InitialStateFunc()
		if err != nil {
			return nil, fmt.Errorf("fetching initial state for watcher: %w", err)
		}
		ch <- state
	}
	return watchertest.NewMockStringsWatcher(ch), nil
}

// NewNamespaceWatcherFactory constructs a new NamespaceWatchFactory with a func
// that can provide initial state into newly minted watchers. initial func can
// be nil in which cash no initial state will be supplied to watchers.
func NewNamespaceWatcherFactory(initial func() ([]string, error)) *NamespaceWatcherFactory {
	return &NamespaceWatcherFactory{
		InitialStateFunc: initial,
		WatcherChans:     map[string]chan []string{},
	}
}
