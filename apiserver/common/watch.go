// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"sync"
	"time"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

// AgentEntityWatcher implements a common Watch method for use by
// various facades.
type AgentEntityWatcher struct {
	st          state.EntityFinder
	resources   facade.Resources
	getCanWatch GetAuthFunc
}

// NewAgentEntityWatcher returns a new AgentEntityWatcher. The
// GetAuthFunc will be used on each invocation of Watch to determine
// current permissions.
func NewAgentEntityWatcher(st state.EntityFinder, resources facade.Resources, getCanWatch GetAuthFunc) *AgentEntityWatcher {
	return &AgentEntityWatcher{
		st:          st,
		resources:   resources,
		getCanWatch: getCanWatch,
	}
}

func (a *AgentEntityWatcher) watchEntity(tag names.Tag) (string, error) {
	entity0, err := a.st.FindEntity(tag)
	if err != nil {
		return "", err
	}
	entity, ok := entity0.(state.NotifyWatcherFactory)
	if !ok {
		return "", NotSupportedError(tag, "watching")
	}
	watch := entity.Watch()
	// Consume the initial event. Technically, API
	// calls to Watch 'transmit' the initial event
	// in the Watch response. But NotifyWatchers
	// have no state to transmit.
	if _, ok := <-watch.Changes(); ok {
		return a.resources.Register(watch), nil
	}
	return "", watcher.EnsureErr(watch)
}

// Watch starts an NotifyWatcher for each given entity.
func (a *AgentEntityWatcher) Watch(args params.Entities) (params.NotifyWatchResults, error) {
	result := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	if len(args.Entities) == 0 {
		return result, nil
	}
	canWatch, err := a.getCanWatch()
	if err != nil {
		return params.NotifyWatchResults{}, errors.Trace(err)
	}
	for i, entity := range args.Entities {
		tag, err := names.ParseTag(entity.Tag)
		if err != nil {
			result.Results[i].Error = ServerError(ErrPerm)
			continue
		}
		err = ErrPerm
		watcherId := ""
		if canWatch(tag) {
			watcherId, err = a.watchEntity(tag)
		}
		result.Results[i].NotifyWatcherId = watcherId
		result.Results[i].Error = ServerError(err)
	}
	return result, nil
}

// MultiNotifyWatcher implements state.NotifyWatcher, combining
// multiple NotifyWatchers.
type MultiNotifyWatcher struct {
	tomb     tomb.Tomb
	watchers []state.NotifyWatcher
	changes  chan struct{}
}

// NewMultiNotifyWatcher creates a NotifyWatcher that combines
// each of the NotifyWatchers passed in. Each watcher's initial
// event is consumed, and a single initial event is sent.
// Subsequent events are not coalesced.
func NewMultiNotifyWatcher(w ...state.NotifyWatcher) *MultiNotifyWatcher {
	m := &MultiNotifyWatcher{
		watchers: w,
		changes:  make(chan struct{}),
	}
	var wg sync.WaitGroup
	wg.Add(len(w))
	staging := make(chan struct{})
	for _, w := range w {
		// Consume the first event of each watcher.
		<-w.Changes()
		go func(wCopy state.NotifyWatcher) {
			defer wg.Done()
			wCopy.Wait()
		}(w)
		// Copy events from the watcher to the staging channel.
		go copyEvents(staging, w.Changes(), &m.tomb)
	}
	m.tomb.Go(func() error {
		m.loop(staging)
		wg.Wait()
		return nil
	})
	return m
}

// loop copies events from the input channel to the output channel,
// coalescing events by waiting a short time between receiving and
// sending.
func (w *MultiNotifyWatcher) loop(in <-chan struct{}) {
	defer close(w.changes)
	// out is initialised to m.changes to send the initial event.
	out := w.changes
	var timer <-chan time.Time
	for {
		select {
		case <-w.tomb.Dying():
			return
		case <-in:
			if timer == nil {
				// TODO(fwereade): 2016-03-17 lp:1558657
				timer = time.After(10 * time.Millisecond)
			}
		case <-timer:
			timer = nil
			out = w.changes
		case out <- struct{}{}:
			out = nil
		}
	}
}

// copyEvents copies channel events from "in" to "out", coalescing.
func copyEvents(out chan<- struct{}, in <-chan struct{}, tomb *tomb.Tomb) {
	var outC chan<- struct{}
	for {
		select {
		case <-tomb.Dying():
			return
		case _, ok := <-in:
			if !ok {
				return
			}
			outC = out
		case outC <- struct{}{}:
			outC = nil
		}
	}
}

func (w *MultiNotifyWatcher) Kill() {
	w.tomb.Kill(nil)
	for _, w := range w.watchers {
		w.Kill()
	}
}

func (w *MultiNotifyWatcher) Wait() error {
	return w.tomb.Wait()
}

func (w *MultiNotifyWatcher) Stop() error {
	w.Kill()
	return w.Wait()
}

func (w *MultiNotifyWatcher) Err() error {
	return w.tomb.Err()
}

func (w *MultiNotifyWatcher) Changes() <-chan struct{} {
	return w.changes
}
