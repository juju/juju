// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"time"

	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	"gopkg.in/juju/worker.v1/catacomb"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/watch"

	"github.com/juju/juju/core/watcher"
)

// kubernetesNotifyWatcher reports changes to kubernetes
// resources. A native kubernetes watcher is passed
// in to generate change events from the kubernetes
// model. These events are consolidated into a Juju
// notification watcher event.
type kubernetesNotifyWatcher struct {
	clock    jujuclock.Clock
	catacomb catacomb.Catacomb

	out       chan struct{}
	name      string
	k8watcher watch.Interface
}

func newKubernetesNotifyWatcher(wi watch.Interface, name string, clock jujuclock.Clock) (*kubernetesNotifyWatcher, error) {
	w := &kubernetesNotifyWatcher{
		clock:     clock,
		out:       make(chan struct{}),
		name:      name,
		k8watcher: wi,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	})
	return w, err
}

const sendDelay = 1 * time.Second

func (w *kubernetesNotifyWatcher) loop() error {
	defer close(w.out)
	defer w.k8watcher.Stop()

	// Set out now so that initial event is sent.
	out := w.out
	var delayCh <-chan time.Time

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case evt, ok := <-w.k8watcher.ResultChan():
			// This can happen if the k8s API connection drops.
			if !ok {
				return errors.Errorf("k8s event watcher closed, restarting")
			}
			logger.Tracef("received k8s event: %+v", evt.Type)
			if pod, ok := evt.Object.(*core.Pod); ok {
				logger.Tracef("%v(%v) = %v, status=%+v", pod.Name, pod.UID, pod.Labels, pod.Status)
			}
			if ns, ok := evt.Object.(*core.Namespace); ok {
				logger.Tracef("%v(%v) = %v, status=%+v", ns.Name, ns.UID, ns.Labels, ns.Status)
			}
			if evt.Type == watch.Error {
				return errors.Errorf("kubernetes watcher error: %v", k8serrors.FromObject(evt.Object))
			}
			if delayCh == nil {
				delayCh = w.clock.After(sendDelay)
			}
		case <-delayCh:
			out = w.out
		case out <- struct{}{}:
			logger.Debugf("fire notify watcher for %v", w.name)
			out = nil
			delayCh = nil
		}
	}
}

// Changes returns the event channel for this watcher.
func (w *kubernetesNotifyWatcher) Changes() watcher.NotifyChannel {
	return w.out
}

// Kill asks the watcher to stop without waiting for it do so.
func (w *kubernetesNotifyWatcher) Kill() {
	w.catacomb.Kill(nil)
}

// Wait waits for the watcher to die and returns any
// error encountered when it was running.
func (w *kubernetesNotifyWatcher) Wait() error {
	return w.catacomb.Wait()
}

type kubernetesStringsWatcher struct {
	clock    jujuclock.Clock
	catacomb catacomb.Catacomb

	out           chan []string
	name          string
	k8watcher     watch.Interface
	initialEvents []string
	filterFunc    k8sStringsWatcherFilterFunc
}

type k8sStringsWatcherFilterFunc func(evt watch.Event) (string, bool)

func newKubernetesStringsWatcher(wi watch.Interface, name string, clock jujuclock.Clock,
	initialEvents []string, filterFunc k8sStringsWatcherFilterFunc) (*kubernetesStringsWatcher, error) {
	w := &kubernetesStringsWatcher{
		clock:         clock,
		out:           make(chan []string),
		name:          name,
		k8watcher:     wi,
		initialEvents: initialEvents,
		filterFunc:    filterFunc,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	})
	return w, err
}

func (w *kubernetesStringsWatcher) loop() error {
	defer close(w.out)
	defer w.k8watcher.Stop()

	select {
	case <-w.catacomb.Dying():
		return w.catacomb.ErrDying()
	case w.out <- w.initialEvents:
	}
	w.initialEvents = nil

	// Set out now so that initial event is sent.
	var out chan []string
	var delayCh <-chan time.Time
	var pendingEvents []string

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case evt, ok := <-w.k8watcher.ResultChan():
			// This can happen if the k8s API connection drops.
			if !ok {
				return errors.Errorf("k8s event watcher closed, restarting")
			}
			if evt.Type == watch.Error {
				return errors.Errorf("kubernetes watcher error: %v", k8serrors.FromObject(evt.Object))
			}
			logger.Tracef("received k8s event: %+v", evt.Type)
			if emittedEvent, ok := w.filterFunc(evt); ok {
				pendingEvents = append(pendingEvents, emittedEvent)
				if delayCh == nil {
					delayCh = w.clock.After(sendDelay)
				}
			}
		case <-delayCh:
			delayCh = nil
			out = w.out
		case out <- pendingEvents:
			out = nil
			pendingEvents = nil
		}
	}
}

// Changes returns the event channel for this watcher.
func (w *kubernetesStringsWatcher) Changes() watcher.StringsChannel {
	return w.out
}

// Kill asks the watcher to stop without waiting for it do so.
func (w *kubernetesStringsWatcher) Kill() {
	w.catacomb.Kill(nil)
}

// Wait waits for the watcher to die and returns any
// error encountered when it was running.
func (w *kubernetesStringsWatcher) Wait() error {
	return w.catacomb.Wait()
}
