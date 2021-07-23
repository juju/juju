// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"sync"

	"github.com/juju/pubsub/v2"
	"gopkg.in/tomb.v2"
)

// ModelWatcher will pass back models added to the cache.
type ModelWatcher interface {
	Watcher
	Changes() <-chan *Model
}

type modelWatcher struct {
	tomb    tomb.Tomb
	changes chan *Model
	// We can't send down a closed channel, so protect the sending
	// with a mutex and bool. Since you can't really even ask a channel
	// if it is closed.
	closed bool
	mu     sync.Mutex

	modelUUID string
}

func newModelWatcher(uuid string, hub *pubsub.SimpleHub, model *Model) *modelWatcher {
	// We use a single entry buffered channel for the changes.
	// The model may already exist, in which case the model is sent down
	// the changes channel immediately.
	w := &modelWatcher{
		changes:   make(chan *Model, 1),
		modelUUID: uuid,
	}
	if model != nil {
		// Since changes is buffered, this doesn't block.
		w.changes <- model
	}
	unsub := hub.Subscribe(modelUpdatedTopic, w.onUpdate)
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		unsub()
		return nil
	})

	return w
}

// Changes is part of the core watcher definition.
func (w *modelWatcher) Changes() <-chan *Model {
	return w.changes
}

// Kill is part of the worker.Worker interface.
func (w *modelWatcher) Kill() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return
	}

	// The watcher must be dying or dead before we close the channel.
	// Otherwise readers could fail, but the watcher's tomb would indicate
	// "still alive".
	w.tomb.Kill(nil)
	w.closed = true
	close(w.changes)
}

// Wait is part of the worker.Worker interface.
func (w *modelWatcher) Wait() error {
	return w.tomb.Wait()
}

// Stop is currently required by the Resources wrapper in the apiserver.
func (w *modelWatcher) Stop() error {
	w.Kill()
	return w.Wait()
}

func (w *modelWatcher) onUpdate(topic string, data interface{}) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return
	}

	model, ok := data.(*Model)
	if !ok {
		logger.Criticalf("programming error: topic data expected *Model, got %T", data)
		return
	}

	if model.UUID() == w.modelUUID {
		// If there is no cached change, then we should be able to send immediately
		// with no block.
		//
		// We explicitly don't sent with a select on the dying channel because we are inside
		// the mutex, so no one else would be able to kill the watcher. If we try to move this
		// outside of the mutex, we hit the potential for someone to kill the worker while we are
		// trying to send, which would cause us to send down a closed channel which causes a panic.
		select {
		case w.changes <- model:
		default:
			// If there is a pending change then that is fine. We know the controller doesn't
			// recreate models, and any new model will have a different UUID.
		}
	}
}
