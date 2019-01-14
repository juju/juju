// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import (
	"io"
	"net/http"
	"net/url"

	"github.com/juju/httprequest"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v2-unstable/httpbakery"
)

//go:generate mockgen -package mocks -destination mocks/caller_mock.go github.com/juju/juju/api/base APICaller,FacadeCaller

// APICaller is implemented by the client-facing State object.
// It defines the lowest level of API calls and is used by
// the various API implementations to actually make
// the calls to the API. It should not be used outside
// of tests or the api/* hierarchy.
type APICaller interface {
	// APICall makes a call to the API server with the given object type,
	// id, request and parameters. The response is filled in with the
	// call's result if the call is successful.
	APICall(objType string, version int, id, request string, params, response interface{}) error

	// BestFacadeVersion returns the newest version of 'objType' that this
	// client can use with the current API server.
	BestFacadeVersion(facade string) int

	// ModelTag returns the tag of the model the client is connected
	// to if there is one. It returns false for a controller-only connection.
	ModelTag() (names.ModelTag, bool)

	// HTTPClient returns an httprequest.Client that can be used
	// to make HTTP requests to the API. URLs passed to the client
	// will be made relative to the API host and the current model.
	//
	// Note that the URLs in HTTP requests passed to the Client.Do
	// method should not include a host part.
	HTTPClient() (*httprequest.Client, error)

	// BakeryClient returns the bakery client for this connection.
	BakeryClient() *httpbakery.Client

	StreamConnector
	ControllerStreamConnector
}

// StreamConnector is implemented by the client-facing State object.
type StreamConnector interface {
	// ConnectStream connects to the given HTTP websocket
	// endpoint path (interpreted relative to the receiver's
	// model) and returns the resulting connection.
	// The given parameters are used as URL query values
	// when making the initial HTTP request.
	//
	// The path must start with a "/".
	ConnectStream(path string, attrs url.Values) (Stream, error)
}

// ControllerStreamConnector is implemented by the client-facing State object.
type ControllerStreamConnector interface {
	// ConnectControllerStream connects to the given HTTP websocket
	// endpoint path and returns the resulting connection. The given
	// values are used as URL query values when making the initial
	// HTTP request. Headers passed in will be added to the HTTP
	// request.
	//
	// The path must be absolute and can't start with "/model".
	ConnectControllerStream(path string, attrs url.Values, headers http.Header) (Stream, error)
}

// Stream represents a streaming connection to the API.
type Stream interface {
	io.Closer

	// NextReader is used to get direct access to the underlying Read methods
	// on the websocket. Mostly just to read the initial error resonse.
	NextReader() (messageType int, r io.Reader, err error)

	// WriteJSON encodes the given value as JSON
	// and writes it to the connection.
	WriteJSON(v interface{}) error

	// ReadJSON reads a JSON value from the stream
	// and decodes it into the element pointed to by
	// the given value, which should be a pointer.
	ReadJSON(v interface{}) error
}

// FacadeCaller is a wrapper for the common paradigm that a given client just
// wants to make calls on a facade using the best known version of the API. And
// without dealing with an id parameter.
type FacadeCaller interface {
	// FacadeCall will place a request against the API using the requested
	// Facade and the best version that the API server supports that is
	// also known to the client.
	FacadeCall(request string, params, response interface{}) error

	// Name returns the facade name.
	Name() string

	// BestAPIVersion returns the API version that we were able to
	// determine is supported by both the client and the API Server
	BestAPIVersion() int

	// RawAPICaller returns the wrapped APICaller. This can be used if you need
	// to switch what Facade you are calling (such as Facades that return
	// Watchers and then need to use the Watcher facade)
	RawAPICaller() APICaller
}

type facadeCaller struct {
	facadeName  string
	bestVersion int
	caller      APICaller
}

var _ FacadeCaller = facadeCaller{}

// FacadeCall will place a request against the API using the requested
// Facade and the best version that the API server supports that is
// also known to the client. (id is always passed as the empty string.)
func (fc facadeCaller) FacadeCall(request string, params, response interface{}) error {
	return fc.caller.APICall(
		fc.facadeName, fc.bestVersion, "",
		request, params, response)
}

// Name returns the facade name.
func (fc facadeCaller) Name() string {
	return fc.facadeName
}

// BestAPIVersion returns the version of the Facade that is going to be used
// for calls. It is determined using the algorithm defined in api
// BestFacadeVersion. Callers can use this to determine what methods must be
// used for compatibility.
func (fc facadeCaller) BestAPIVersion() int {
	return fc.bestVersion
}

// RawAPICaller returns the wrapped APICaller. This can be used if you need to
// switch what Facade you are calling (such as Facades that return Watchers and
// then need to use the Watcher facade)
func (fc facadeCaller) RawAPICaller() APICaller {
	return fc.caller
}

// NewFacadeCaller wraps an APICaller for a given facade name and the
// best available version.
func NewFacadeCaller(caller APICaller, facadeName string) FacadeCaller {
	return NewFacadeCallerForVersion(caller, facadeName, caller.BestFacadeVersion(facadeName))
}

// NewFacadeCallerForVersion wraps an APICaller for a given facade
// name and version.
func NewFacadeCallerForVersion(caller APICaller, facadeName string, version int) FacadeCaller {
	return facadeCaller{
		facadeName:  facadeName,
		bestVersion: version,
		caller:      caller,
	}
}
