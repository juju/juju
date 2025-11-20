// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package observer

import (
	"context"
	"net/http"
	"sync"

	"github.com/juju/names/v6"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/rpc"
)

// Observer defines a type which will observe API server events as
// they happen.
type Observer interface {
	rpc.ObserverFactory

	// Login informs an Observer that an entity has logged in.
	Login(ctx context.Context, entity names.Tag, model names.ModelTag, modelUUID model.UUID, fromController bool, userData string)

	// Join is called when the connection to the API server's
	// WebSocket is opened.
	Join(ctx context.Context, req *http.Request, connectionID uint64, fd int)

	// Leave is called when the connection to the API server's
	// WebSocket is closed.
	Leave(ctx context.Context)
}

// ObserverFactory is a function which creates an Observer.
type ObserverFactory func() Observer

// ObserverFactoryMultiplexer returns an ObserverFactory which will
// return a Multiplexer of all the observers instantiated from the
// factories passed in.
func ObserverFactoryMultiplexer(factories ...ObserverFactory) ObserverFactory {
	return func() Observer {
		observers := make([]Observer, 0, len(factories))
		for _, newObserver := range factories {
			observers = append(observers, newObserver())
		}
		return NewMultiplexer(observers...)
	}
}

// None is a wrapper around the Multiplexer factory to add clarity to
// code that doesn't need any observers.
func None() *Multiplexer {
	return NewMultiplexer()
}

// NewMultiplexer creates a new Multiplexer with the provided
// observers.
func NewMultiplexer(observers ...Observer) *Multiplexer {
	return &Multiplexer{
		observers: removeNilObservers(observers),
	}
}

func removeNilObservers(observers []Observer) []Observer {
	var validatedObservers []Observer
	for _, o := range observers {
		if o == nil {
			continue
		}
		validatedObservers = append(validatedObservers, o)
	}
	return validatedObservers
}

// Multiplexer multiplexes calls to an arbitrary number of observers.
type Multiplexer struct {
	observers []Observer
}

// Join is called when the connection to the API server's WebSocket is
// opened.
func (m *Multiplexer) Join(ctx context.Context, req *http.Request, connectionID uint64, fd int) {
	mapConcurrent(req.Context(), func(ctx context.Context, o Observer) {
		o.Join(ctx, req, connectionID, fd)
	}, m.observers)
}

// Leave implements Observer.
func (m *Multiplexer) Leave(ctx context.Context) {
	mapConcurrent(ctx, func(ctx context.Context, o Observer) {
		o.Leave(ctx)
	}, m.observers)
}

// Login implements Observer.
func (m *Multiplexer) Login(ctx context.Context, entity names.Tag, model names.ModelTag, modelUUID model.UUID, fromController bool, userData string) {
	mapConcurrent(ctx, func(ctx context.Context, o Observer) {
		o.Login(ctx, entity, model, modelUUID, fromController, userData)
	}, m.observers)
}

// RPCObserver implements Observer. It will create an
// rpc.ObserverMultiplexer by calling all the Observer's RPCObserver
// methods.
func (m *Multiplexer) RPCObserver() rpc.Observer {
	rpcObservers := make([]rpc.Observer, 0, len(m.observers))
	for _, o := range m.observers {
		observer := o.RPCObserver()
		if observer == nil {
			continue
		}

		rpcObservers = append(rpcObservers, observer)
	}
	return rpc.NewObserverMultiplexer(rpcObservers...)
}

// mapConcurrent calls fn on all observers concurrently and then waits
// for all calls to exit before returning.
func mapConcurrent(ctx context.Context, fn func(context.Context, Observer), observers []Observer) {
	var wg sync.WaitGroup
	wg.Add(len(observers))

	for _, o := range observers {
		go func(obs Observer) {
			defer wg.Done()
			fn(ctx, obs)
		}(o)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
	case <-done:
	}
}
