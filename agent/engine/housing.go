// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package engine

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	"github.com/juju/juju/internal/worker/fortress"
)

// Decorator creates copies of dependency.Manifolds with additional
// features.
type Decorator interface {

	// Decorate returns a new Manifold, based on the one supplied.
	Decorate(dependency.Manifold) dependency.Manifold
}

// Housing is a Decorator that combines several common mechanisms
// for coordinating manifolds independently of their core concerns.
type Housing struct {

	// Flags contains a list of names of Flag manifolds, such
	// that a decorated manifold will not start until all flags
	// are both present and valid (and will be stopped when that
	// is no longer true).
	Flags []string

	// Occupy is ignored if empty; otherwise it contains the name
	// of a fortress.Guest manifold, such that a decorated manifold
	// will never be run outside a Visit to that fortress.
	//
	// NOTE: this acquires a lock, and holds it for your manifold's
	// worker's whole lifetime. It's fine in isolation, but multiple
	// Occupy~s are almost certainly a Bad Idea.
	Occupy string

	// Filter is ignored if nil; otherwise it's unconditionally set
	// as the manifold's Filter. Similarly to Occupy, attempted use
	// of multiple filters is unlikely to be a great idea; it most
	// likely indicates that either your Engine's IsFatal is too
	// enthusiastic, or responsibility for termination is spread too
	// widely across your installed manifolds, or both.
	Filter dependency.FilterFunc
}

// Decorate is part of the Decorator interface.
func (housing Housing) Decorate(base dependency.Manifold) dependency.Manifold {
	manifold := base
	// Apply Occupy wrapping first, so that it will be the last
	// wrapper to execute before calling the original Start func, so
	// as to minimise the time we hold the fortress open.
	if housing.Occupy != "" {
		manifold.Inputs = maybeAdd(manifold.Inputs, housing.Occupy)
		manifold.Start = occupyStart(manifold.Start, housing.Occupy)
	}
	for _, name := range housing.Flags {
		manifold.Inputs = maybeAdd(manifold.Inputs, name)
		manifold.Start = flagStart(manifold.Start, name)
	}
	if housing.Filter != nil {
		manifold.Filter = housing.Filter
	}
	return manifold
}

func maybeAdd(original []string, add string) []string {
	for _, name := range original {
		if name == add {
			return original
		}
	}
	count := len(original)
	result := make([]string, count, count+1)
	copy(result, original)
	return append(result, add)
}

func occupyStart(inner dependency.StartFunc, name string) dependency.StartFunc {
	return func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
		var guest fortress.Guest
		if err := getter.Get(name, &guest); err != nil {
			return nil, errors.Trace(err)
		}
		task := func() (worker.Worker, error) {
			return inner(ctx, getter)
		}
		worker, err := fortress.Occupy(ctx, guest, task)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return worker, nil
	}
}

func flagStart(inner dependency.StartFunc, name string) dependency.StartFunc {
	return func(ctx context.Context, getter dependency.Getter) (worker.Worker, error) {
		var flag Flag
		if err := getter.Get(name, &flag); err != nil {
			return nil, errors.Trace(err)
		}
		if !flag.Check() {
			return nil, errors.Annotatef(dependency.ErrMissing, "%q not set", name)
		}
		return inner(ctx, getter)
	}
}
