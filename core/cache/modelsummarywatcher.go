// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/juju/collections/set"
	"gopkg.in/tomb.v2"
)

// Status values show a high level indicator of model health.
const (
	StatusRed    = "red"
	StatusYellow = "yellow" // amber?
	StatusGreen  = "green"
)

// ModelSummary represents a high level view of a model. Primary use case is to
// drive a dashboard with model level overview.
type ModelSummary struct {
	UUID string
	// Removed indicates that the user can no longer see this model, because it has
	// either been removed, or there access revoked.
	Removed bool

	Controller  string
	Namespace   string
	Name        string
	Admins      []string
	Status      string
	Annotations map[string]string

	// Messages contain status message for any unit status in error.
	Messages []ModelSummaryMessage

	Cloud        string
	Region       string
	Credential   string
	LastModified time.Time // Currently missing in cache and underlying db model.

	// MachineCount is just top level machines.
	MachineCount     int
	ContainerCount   int
	ApplicationCount int
	UnitCount        int
	RelationCount    int
}

// ModelSummaryMessage holds information about an error message from an
// agent, and when that message was set.
type ModelSummaryMessage struct {
	Agent   string
	Message string
}

// ModelSummaryWatcher will return a slice for all existing models
type ModelSummaryWatcher interface {
	Watcher
	Changes() <-chan []ModelSummary
}

type modelSummaryPayload struct {
	summary   ModelSummary
	visibleTo func(user string) bool
	hash      string
}

func newModelSummaryWatcher(controller *Controller, user string) *modelSummaryWatcher {
	w := &modelSummaryWatcher{
		controller: controller,
		// Force the user name to lowercase as all permissions use lower case user keys.
		user:          strings.ToLower(user),
		hashes:        make(map[string]string),
		visibleModels: set.NewStrings(),
		changes:       make(chan []ModelSummary),
		summaryChange: make(chan ModelSummary),
	}
	w.tomb.Go(w.loop)
	return w
}

type modelSummaryWatcher struct {
	tomb       tomb.Tomb
	controller *Controller
	changes    chan []ModelSummary

	// If user is set, the models returned will be limited to the models
	// that the user can see. If user is the empty string, all models are returned.
	user string

	summaryChange chan ModelSummary
	pending       []ModelSummary

	// The mutex protects the attributes below it. Used primarily to keep track of
	// which models we are seeing.
	mu            sync.Mutex
	hashes        map[string]string
	visibleModels set.Strings
}

func (w *modelSummaryWatcher) init() {
	controllerName := w.controller.Name()
	w.controller.modelsMu.Lock()
	defer w.controller.modelsMu.Unlock()
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, model := range w.controller.models {
		if !model.visibleTo(w.user) {
			continue
		}
		summary, hash := model.Summary()
		summary.Controller = controllerName
		w.pending = append(w.pending, summary)
		uuid := model.UUID()
		w.hashes[uuid] = hash
		w.visibleModels.Add(uuid)
	}
}

func (w *modelSummaryWatcher) loop() error {
	defer close(w.changes)
	// Use a hub multiplexer to avoid all the extra goroutines for subscribers.
	multiplexer := w.controller.hub.NewMultiplexer()
	defer multiplexer.Unsubscribe()
	multiplexer.Add(modelSummaryUpdatedTopic, w.onSummaryUpdate)
	multiplexer.Add(modelRemovedTopic, w.onModelRemove)

	w.init()
	// We want the first call to Next to get an empty list if that is all the user
	// can see.
	first := true
	for {
		var changes chan []ModelSummary
		// If the pending slice is empty, we don't want to send down the changes
		// channel, so we evaluate that each time through the loop, and determine
		// whether the changes channel should be the member variable, or nil.
		// Sending down a nil channel blocks forever (https://dave.cheney.net/2014/03/19/channel-axioms).
		if first || len(w.pending) > 0 {
			changes = w.changes
		}

		select {
		case <-w.tomb.Dying():
			return nil
		case changes <- w.pending:
			// Changes received, clear pending.
			w.pending = nil
			first = false
		case summary := <-w.summaryChange:
			// If there is an existing summary for this model, replace it with the
			// new summary, otherwise add it to the end.
			replaced := false
			for i, value := range w.pending {
				if value.UUID == summary.UUID {
					logger.Tracef("replacing pending summary for %q", summary.UUID)
					w.pending[i] = summary
					replaced = true
					break
				}
			}
			if !replaced {
				logger.Tracef("adding new summary for %q", summary.UUID)
				w.pending = append(w.pending, summary)
			}
		}
	}

}

func (w *modelSummaryWatcher) onSummaryUpdate(topic string, data interface{}) {
	// Cast to expected type.
	payload, ok := data.(modelSummaryPayload)
	if !ok {
		logger.Criticalf("programming error: topic data expected modelSummaryPayload, got %T", data)
		return
	}

	uuid := payload.summary.UUID
	logger.Tracef("onSummaryChange: %s", uuid)
	w.mu.Lock()
	defer w.mu.Unlock()
	var summary ModelSummary
	if payload.visibleTo(w.user) {
		// Add is idempotent, and we don't care if we weren't previously tracking.
		if lastHash := w.hashes[uuid]; lastHash == payload.hash {
			// If we have already notified for this hash, then don't send
			// again. If this is a newly watched model, the hash in the map
			// will be the empty string.
			return
		}
		w.hashes[uuid] = payload.hash
		w.visibleModels.Add(uuid)
		summary = payload.summary
		summary.Controller = w.controller.Name()
	} else {
		if !w.visibleModels.Contains(uuid) {
			// We aren't tracking, and shouldn't be, so nothing to do.
			return
		}
		// If we were tracking this model, stop tracking this model.
		w.visibleModels.Remove(uuid)
		delete(w.hashes, uuid)
		summary = ModelSummary{
			UUID:    uuid,
			Removed: true,
		}
	}

	select {
	case <-w.tomb.Dying():
	case w.summaryChange <- summary:
	}
}

func (w *modelSummaryWatcher) onModelRemove(topic string, data interface{}) {
	uuid, ok := data.(string)
	if !ok {
		logger.Criticalf("programming error: topic data expected string, got %T", data)
		return
	}
	logger.Tracef("onModelRemove: %s", uuid)
	w.mu.Lock()
	if !w.visibleModels.Contains(uuid) {
		// We are not tracking it, so nothing to do.
		w.mu.Unlock()
		return
	}
	w.mu.Unlock()
	summary := ModelSummary{
		UUID:    uuid,
		Removed: true,
	}
	select {
	case <-w.tomb.Dying():
	case w.summaryChange <- summary:
	}
}

// Kill is part of the worker.Worker interface.
func (w *modelSummaryWatcher) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *modelSummaryWatcher) Wait() error {
	return w.tomb.Wait()
}

// Stop is currently required by the Resources wrapper in the apiserver.
func (w *modelSummaryWatcher) Stop() error {
	w.Kill()
	return w.Wait()
}

// Changes returns the channel that notifies of summary updates.
func (w *modelSummaryWatcher) Changes() <-chan []ModelSummary {
	return w.changes
}

func (s *ModelSummary) hash() (string, error) {
	// Make a string representation of the summary, and hash that string.
	var messages string
	for _, m := range s.Messages {
		messages += m.Agent + m.Message
	}
	var annotations string
	var keys []string
	for key := range s.Annotations {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		annotations += key + "=" + s.Annotations[key] + " "
	}
	admins := strings.Join(s.Admins, ", ")
	return hash(
		s.UUID, strconv.FormatBool(s.Removed),
		s.Namespace, s.Name, admins, s.Status, annotations, messages,
		s.Cloud, s.Region, s.Credential,
		s.LastModified.String(),
		strconv.Itoa(s.MachineCount),
		strconv.Itoa(s.ContainerCount),
		strconv.Itoa(s.ApplicationCount),
		strconv.Itoa(s.UnitCount),
		strconv.Itoa(s.RelationCount),
	)
}
