// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package observer

import (
	"net/http"
	"sync"

	"github.com/juju/juju/rpc"
	"github.com/juju/names/v4"
)

// Observer defines a type which will observe API server events as
// they happen.
type Observer interface {
	rpc.ObserverFactory

	// Login informs an Observer that an entity has logged in.
	Login(entity names.Tag, model names.ModelTag, fromController bool, userData string)

	// Join is called when the connection to the API server's
	// WebSocket is opened.
	Join(req *http.Request, connectionID uint64)

	// Leave is called when the connection to the API server's
	// WebSocket is closed.
	Leave()
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
func (m *Multiplexer) Join(req *http.Request, connectionID uint64) {
	mapConcurrent(func(o Observer) { o.Join(req, connectionID) }, m.observers)
}

// Leave implements Observer.
func (m *Multiplexer) Leave() {
	mapConcurrent(Observer.Leave, m.observers)
}

// Login implements Observer.
func (m *Multiplexer) Login(entity names.Tag, model names.ModelTag, fromController bool, userData string) {
	mapConcurrent(func(o Observer) { o.Login(entity, model, fromController, userData) }, m.observers)
}

// RPCObserver implements Observer. It will create an
// rpc.ObserverMultiplexer by calling all the Observer's RPCObserver
// methods.
func (m *Multiplexer) RPCObserver() rpc.Observer {
	rpcObservers := make([]rpc.Observer, len(m.observers))
	for i, o := range m.observers {
		rpcObservers[i] = o.RPCObserver()
	}
	return rpc.NewObserverMultiplexer(rpcObservers...)
}

// mapConcurrent calls fn on all observers concurrently and then waits
// for all calls to exit before returning.
func mapConcurrent(fn func(Observer), observers []Observer) {
	var wg sync.WaitGroup
	wg.Add(len(observers))
	defer wg.Wait()

	for _, o := range observers {
		go func(obs Observer) {
			defer wg.Done()
			fn(obs)
		}(o)
	}
}
