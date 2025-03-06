// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rpc

import (
	"context"
	"sync"
)

// Observer can be implemented to find out about requests occurring in
// an RPC conn, for example to print requests for logging
// purposes. The calls should not block or interact with the Conn
// object as that can cause delays to the RPC server or deadlock.
type Observer interface {

	// ServerRequest informs the Observer of a request made
	// to the Conn. If the request was not recognized or there was
	// an error reading the body, body will be nil.
	//
	// ServerRequest is called just before the server method
	// is invoked.
	ServerRequest(ctx context.Context, hdr *Header, body interface{})

	// ServerReply informs the RequestNotifier of a reply sent to a
	// server request. The given Request gives details of the call
	// that was made; the given Header and body are the header and
	// body sent as reply.
	//
	// ServerReply is called just before the reply is written.
	ServerReply(ctx context.Context, req Request, hdr *Header, body interface{})
}

// ObserverFactory is a type which can construct a new Observer.
type ObserverFactory interface {
	// RPCObserver will return a new Observer usually constructed
	// from the state previously built up in the Observer. The
	// returned instance will be utilized per RPC request.
	RPCObserver() Observer
}

// NewObserverMultiplexer returns a new ObserverMultiplexer
// with the provided RequestNotifiers.
func NewObserverMultiplexer(rpcObservers ...Observer) *ObserverMultiplexer {
	return &ObserverMultiplexer{
		rpcObservers: rpcObservers,
	}
}

// ObserverMultiplexer multiplexes calls to an arbitrary number of
// Observers.
type ObserverMultiplexer struct {
	rpcObservers []Observer
}

// ServerReply implements Observer.
func (m *ObserverMultiplexer) ServerReply(ctx context.Context, req Request, hdr *Header, body interface{}) {
	mapConcurrent(ctx, func(ctx context.Context, n Observer) {
		n.ServerReply(ctx, req, hdr, body)
	}, m.rpcObservers)
}

// ServerRequest implements Observer.
func (m *ObserverMultiplexer) ServerRequest(ctx context.Context, hdr *Header, body interface{}) {
	mapConcurrent(ctx, func(ctx context.Context, n Observer) {
		n.ServerRequest(ctx, hdr, body)
	}, m.rpcObservers)
}

// mapConcurrent calls fn on all observers concurrently and then waits
// for all calls to exit before returning.
func mapConcurrent(ctx context.Context, fn func(context.Context, Observer), requestNotifiers []Observer) {
	var wg sync.WaitGroup
	wg.Add(len(requestNotifiers))

	done := make(chan struct{})
	for _, n := range requestNotifiers {
		go func(notifier Observer) {
			defer wg.Done()
			fn(ctx, notifier)
		}(n)
	}

	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-ctx.Done():
	case <-done:
	}
}
