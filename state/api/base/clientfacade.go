// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

// APICallCloser is the same as APICaller, but also provides a Close() method
// for when we are done with this connection.
type APICallCloser interface {
	APICaller

	// Close is used when we have finished with this connection.
	Close() error
}

// ClientFacade should be embedded by client-side facades that are intended as
// "client" (aka user facing) facades versus agent facing facades.
// They provide two common methods for writing the client side code.
// BestAPIVersion() is used to allow for compatibility testing, and Close() is
// used to indicate when we are done with the connection.
type ClientFacade interface {
	// BestAPIVersion returns the API version that we were able to
	// determine is supported by both the client and the API Server
	BestAPIVersion() int

	// Close the connection to the API server.
	Close() error
}

type closer interface {
	Close() error
}

type clientFacade struct {
	facadeCaller
	closer
}

var _ ClientFacade = (*clientFacade)(nil)

// NewClientFacade prepares a client-facing facade for work against the API.
// It is expected that most client-facing facades will embed a ClientFacade and
// will use a FacadeCaller so this function returns both.
func NewClientFacade(caller APICallCloser, facadeName string) (ClientFacade, FacadeCaller) {
	clientFacade := clientFacade{
		facadeCaller: facadeCaller{
			facadeName:  facadeName,
			bestVersion: caller.BestFacadeVersion(facadeName),
			caller:      caller,
		}, closer: caller,
	}
	return clientFacade, clientFacade
}
