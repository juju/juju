// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"sync"

	"github.com/juju/pubsub"
)

const (
	applicationConfigChange = "application-config-change"
	applicationFieldChange  = "application-field-change"
)

func newApplication(metrics *ControllerGauges, hub *pubsub.SimpleHub) *Application {
	m := &Application{
		metrics: metrics,
		hub:     hub,
	}
	return m
}

// Application represents an application in a model.
type Application struct {
	// Link to model?
	metrics *ControllerGauges
	hub     *pubsub.SimpleHub
	mu      sync.Mutex

	details    ApplicationChange
	configHash string
}

func (a *Application) WatchFields(comparitors ...func(*ApplicationDelta) bool) *applicationFieldWatcher {
	w := newApplicationFieldWatcher(comparitors)

	unsub := a.hub.Subscribe(a.topic(applicationFieldChange), w.detailsChange)

	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		unsub()
		return nil
	})

	return w
}

// TODO (manadart 2018-03-13) Should we change this up and down the call stack
// so that pointers are passed instead of copies?
func (a *Application) setDetails(details ApplicationChange) {
	a.mu.Lock()
	defer a.mu.Unlock()

	delta := &ApplicationDelta{old: a.details, new: details}
	a.details = details

	configHash, err := hash(details.Config)
	if err != nil {
		logger.Errorf("invariant error - application config should be yaml serializable and hashable, %v", err)
		configHash = ""
	}
	if configHash != a.configHash {
		a.configHash = configHash
		// TODO: publish config change...
	}

	a.hub.Publish(a.topic(applicationFieldChange), delta)
}

// topic prefixes the input string with the application name.
func (a *Application) topic(suffix string) string {
	return a.details.Name + ":" + suffix
}

type applicationFieldWatcher struct {
	*notifyWatcherBase

	comparitors []func(*ApplicationDelta) bool
}

func newApplicationFieldWatcher(comparitors []func(*ApplicationDelta) bool) *applicationFieldWatcher {
	return &applicationFieldWatcher{
		notifyWatcherBase: newNotifyWatcherBase(),

		comparitors: comparitors,
	}
}

func (w *applicationFieldWatcher) detailsChange(topic string, value interface{}) {
	delta, ok := value.(*ApplicationDelta)
	if !ok {
		logger.Errorf("programming error, value not of type *ApplicationChange")
	}

	// If no comparitors were specified, notify for any change.
	if len(w.comparitors) == 0 {
		w.notify()
		return
	}

	// Otherwise notify if any of the articles we are interested in change.
	for _, c := range w.comparitors {
		if c(delta) {
			w.notify()
			return
		}
	}
}
