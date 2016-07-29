// Copyright 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

// The parallel package provides utilities for running tasks
// concurrently.
package parallel

import (
	"fmt"
	"sync"
)

// Run represents a number of functions running concurrently.
type Run struct {
	mu      sync.Mutex
	results chan Errors
	max     int
	running int
	work    chan func() error
}

// Errors holds any errors encountered during the parallel run.
type Errors []error

func (errs Errors) Error() string {
	switch len(errs) {
	case 0:
		return "no error"
	case 1:
		return errs[0].Error()
	}
	return fmt.Sprintf("%s (and %d more)", errs[0].Error(), len(errs)-1)
}

// NewRun returns a new parallel instance. It provides a way of running
// functions concurrently while limiting the maximum number running at
// once to max.
func NewRun(max int) *Run {
	if max < 1 {
		panic("parameter max must be >= 1")
	}
	return &Run{
		max:     max,
		results: make(chan Errors),
		work:    make(chan func() error),
	}
}

// Do requests that r run f concurrently.  If there are already the maximum
// number of functions running concurrently, it will block until one of them
// has completed. Do may itself be called concurrently, but may not be called
// concurrently with Wait.
func (r *Run) Do(f func() error) {
	select {
	case r.work <- f:
		return
	default:
	}
	r.mu.Lock()
	if r.running < r.max {
		r.running++
		go r.runner()
	}
	r.mu.Unlock()
	r.work <- f
}

// Wait marks the parallel instance as complete and waits for all the functions
// to complete.  If any errors were encountered, it returns an Errors value
// describing all the errors in arbitrary order.
func (r *Run) Wait() error {
	close(r.work)
	var errs Errors
	for i := 0; i < r.running; i++ {
		errs = append(errs, <-r.results...)
	}
	if len(errs) == 0 {
		return nil
	}
	// TODO(rog) sort errors by original order of Do request?
	return errs
}

func (r *Run) runner() {
	var errs Errors
	for f := range r.work {
		if err := f(); err != nil {
			errs = append(errs, err)
		}
	}
	r.results <- errs
}
