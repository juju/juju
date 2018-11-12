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

// KubernetesWatcher reports changes to kubernetes
// resources. A native kubernetes watcher is passed
// in to generate change events from the kubernetes
// model. These events are consolidated into a Juju
// notification watcher event.
type KubernetesWatcher struct {
	clock    jujuclock.Clock
	catacomb catacomb.Catacomb

	out       chan struct{}
	name      string
	k8watcher watch.Interface
}

func newKubernetesWatcher(wi watch.Interface, name string, clock jujuclock.Clock) (*KubernetesWatcher, error) {
	w := &KubernetesWatcher{
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

func (w *KubernetesWatcher) loop() error {
	defer close(w.out)
	defer w.k8watcher.Stop()

	var out chan struct{}
	// Set delayCh now so that initial event is sent.
	delayCh := w.clock.After(sendDelay)
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
				logger.Debugf("%v(%v) = %v, status=%+v", pod.Name, pod.UID, pod.Labels, pod.Status)
			}
			if ns, ok := evt.Object.(*core.Namespace); ok {
				logger.Debugf("%v(%v) = %v, status=%+v", ns.Name, ns.UID, ns.Labels, ns.Status)
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
func (w *KubernetesWatcher) Changes() watcher.NotifyChannel {
	return w.out
}

// Kill asks the watcher to stop without waiting for it do so.
func (w *KubernetesWatcher) Kill() {
	w.catacomb.Kill(nil)
}

// Wait waits for the watcher to die and returns any
// error encountered when it was running.
func (w *KubernetesWatcher) Wait() error {
	return w.catacomb.Wait()
}
