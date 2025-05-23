// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"context"
	"time"

	jujuclock "github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4/catacomb"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/tools/cache"

	"github.com/juju/juju/core/watcher"
	internallogger "github.com/juju/juju/internal/logger"
)

var logger = internallogger.GetLogger("juju.kubernetes.provider.watcher")

type KubernetesNotifyWatcher = watcher.NotifyWatcher

// kubernetesNotifyWatcher reports changes to kubernetes
// resources. A native kubernetes watcher is passed
// in to generate change events from the kubernetes
// model. These events are consolidated into a Juju
// notification watcher event.
type kubernetesNotifyWatcher struct {
	clock    jujuclock.Clock
	catacomb catacomb.Catacomb

	out      chan struct{}
	name     string
	informer cache.SharedIndexInformer
}

// NewK8sWatcherFunc defines a function which returns a k8s watcher based on the supplied config.
type NewK8sWatcherFunc func(
	informer cache.SharedIndexInformer,
	name string,
	clock jujuclock.Clock) (KubernetesNotifyWatcher, error)

type WatchEvent string

var (
	WatchEventAdd    WatchEvent = "add"
	WatchEventDelete WatchEvent = "delete"
	WatchEventUpdate WatchEvent = "update"
)

func NewKubernetesNotifyWatcher(informer cache.SharedIndexInformer, name string, clock jujuclock.Clock) (KubernetesNotifyWatcher, error) {
	w := &kubernetesNotifyWatcher{
		clock:    clock,
		informer: informer,
		name:     name,
		out:      make(chan struct{}),
	}
	err := catacomb.Invoke(catacomb.Plan{
		Name: "kubernetes-notify-watcher",
		Site: &w.catacomb,
		Work: w.loop,
	})
	return w, err
}

const sendDelay = 1 * time.Second

func (w *kubernetesNotifyWatcher) loop() error {
	signals := make(chan struct{}, 1)
	defer close(w.out)

	fireFn := func(evt WatchEvent) func(interface{}) {
		return func(obj interface{}) {
			meta, err := meta.Accessor(obj)
			if err != nil {
				logger.Errorf(context.TODO(), "getting kubernetes watcher event meta: %v", err)
			} else {
				logger.Tracef(context.TODO(), "kubernetes watch event %s %v(%v) = %v", evt,
					meta.GetName(), meta.GetUID(), meta.GetLabels())
			}

			select {
			case signals <- struct{}{}:
			default:
			}
		}
	}

	_, err := w.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    fireFn("add"),
		DeleteFunc: fireFn("delete"),
		UpdateFunc: func(_, obj interface{}) {
			fireFn("update")(obj)
		},
	})
	if err != nil {
		return errors.Trace(err)
	}

	// Set out now so that initial event is sent.
	out := w.out
	var delayCh <-chan time.Time

	go w.informer.Run(w.catacomb.Dying())
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case <-signals:
			if delayCh == nil {
				delayCh = w.clock.After(sendDelay)
			}
		case <-delayCh:
			out = w.out
		case out <- struct{}{}:
			logger.Debugf(context.TODO(), "fire notify watcher for %v", w.name)
			out = nil
			delayCh = nil
		}
	}
}

// Changes returns the event channel for this watcher.
func (w *kubernetesNotifyWatcher) Changes() <-chan struct{} {
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

type KubernetesStringsWatcher = watcher.StringsWatcher

type kubernetesStringsWatcher struct {
	clock    jujuclock.Clock
	catacomb catacomb.Catacomb

	out           chan []string
	name          string
	informer      cache.SharedIndexInformer
	initialEvents []string
	filterFunc    K8sStringsWatcherFilterFunc
}

type K8sStringsWatcherFilterFunc func(evt WatchEvent, obj interface{}) (string, bool)

// NewK8sStringsWatcherFunc defines a function which returns a k8s string watcher
// based on the supplied config
type NewK8sStringsWatcherFunc func(
	informer cache.SharedIndexInformer,
	name string,
	clock jujuclock.Clock, initialEvents []string,
	filterFunc K8sStringsWatcherFilterFunc) (KubernetesStringsWatcher, error)

func NewKubernetesStringsWatcher(informer cache.SharedIndexInformer, name string, clock jujuclock.Clock,
	initialEvents []string, filterFunc K8sStringsWatcherFilterFunc) (KubernetesStringsWatcher, error) {
	w := &kubernetesStringsWatcher{
		clock:         clock,
		out:           make(chan []string),
		informer:      informer,
		name:          name,
		initialEvents: initialEvents,
		filterFunc:    filterFunc,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Name: "kubernetes-strings-watcher",
		Site: &w.catacomb,
		Work: w.loop,
	})
	return w, err
}

func (w *kubernetesStringsWatcher) loop() error {
	defer close(w.out)

	select {
	case <-w.catacomb.Dying():
		return w.catacomb.ErrDying()
	case w.out <- w.initialEvents:
	}
	w.initialEvents = nil

	signals := make(chan string)
	fireFn := func(evt WatchEvent) func(interface{}) {
		return func(obj interface{}) {
			meta, err := meta.Accessor(obj)
			if err != nil {
				logger.Errorf(context.TODO(), "getting kubernetes watcher event meta: %v", err)
			} else {
				logger.Tracef(context.TODO(), "kubernetes watch event %s %v(%v) = %v", evt,
					meta.GetName(), meta.GetUID(), meta.GetLabels())
			}

			if emittedEvent, ok := w.filterFunc(evt, obj); ok {
				select {
				case signals <- emittedEvent:
				case <-w.catacomb.Dying():
				}
			}
		}
	}

	_, err := w.informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    fireFn(WatchEventAdd),
		DeleteFunc: fireFn(WatchEventDelete),
		UpdateFunc: func(_, obj interface{}) {
			fireFn(WatchEventUpdate)(obj)
		},
	})
	if err != nil {
		return errors.Trace(err)
	}

	// Set out now so that initial event is sent.
	var out chan []string
	var delayCh <-chan time.Time
	var pendingEvents []string

	go w.informer.Run(w.catacomb.Dying())
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case emittedEvent := <-signals:
			pendingEvents = append(pendingEvents, emittedEvent)
			if delayCh == nil {
				delayCh = w.clock.After(sendDelay)
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
func (w *kubernetesStringsWatcher) Changes() <-chan []string {
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
