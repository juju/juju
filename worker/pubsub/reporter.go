// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pubsub

import (
	"sync"

	"github.com/juju/worker/v2"
)

// Reporter gives visibility for the introspection worker into the
// internals of the pubsub forwarding worker. Also defines the
// Report method used by the engine report.
type Reporter interface {
	Report() map[string]interface{}
	IntrospectionReport() string
}

// NewReporter returns a reporter for the pubsub forwarding worker.
func NewReporter() Reporter {
	return &reporter{}
}

type reporter struct {
	mu     sync.Mutex
	worker Reporter
}

// IntrospectionReport is the method called by the introspection
// worker to get what to show to the user.
func (r *reporter) IntrospectionReport() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.worker == nil {
		return "pubsub worker not started"
	}
	return r.worker.IntrospectionReport()
}

// Report hooks in to the worker's report mechanism.
func (r *reporter) Report() map[string]interface{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.worker == nil {
		return nil
	}
	return r.worker.Report()
}

func (r *reporter) setWorker(w worker.Worker) {
	if rep, ok := w.(Reporter); ok {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.worker = rep
	}
}
