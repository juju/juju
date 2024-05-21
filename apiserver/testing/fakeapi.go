// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"

	"github.com/bmizerany/pat"
	"github.com/gorilla/websocket"
	jujuhttp "github.com/juju/juju/internal/http"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/apiserver/observer/fakeobserver"
	apiwebsocket "github.com/juju/juju/apiserver/websocket"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/internal/rpcreflect"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/jsoncodec"
	"github.com/juju/juju/testing"
)

// Server represents a fake API server. It must be closed
// after use.
type Server struct {
	// Addrs holds the address used for the
	// server, suitable for including in api.Info.Addrs
	Addrs []string

	*httptest.Server
	newRoot func(modelUUID string) (interface{}, error)
}

// NewAPIServer serves RPC methods on a localhost HTTP server.
// When a connection is made to the API, the newRoot function
// is called with the requested model UUID and the returned
// value defines the API (see the juju/rpc package).
//
// Note that the root value accepts any facade version number - it
// is not currently possible to use this to serve several different
// facade versions.
//
// The server uses testing.ServerCert and testing.ServerKey
// to host the server.
//
// The returned server must be closed after use.
func NewAPIServer(newRoot func(modelUUID string) (interface{}, error)) *Server {
	tlsCert, err := tls.X509KeyPair([]byte(testing.ServerCert), []byte(testing.ServerKey))
	if err != nil {
		panic("bad key pair")
	}

	srv := &Server{
		newRoot: newRoot,
	}
	pmux := pat.New()
	pmux.Get("/model/:modeluuid/api", http.HandlerFunc(srv.serveAPI))
	pmux.Get("/model/:modeluuid/log", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Dummy debug log handler - wait forever.
		apiwebsocket.Serve(w, r, func(conn *apiwebsocket.Conn) {
			defer conn.Close()
			_ = conn.SendInitialErrorV0(err)
			var ch chan bool
			for {
				select {
				case <-ch:
				}
			}
		})
	}))
	pmux.Get("/api", http.HandlerFunc(srv.serveAPI))

	srv.Server = httptest.NewUnstartedServer(pmux)

	tlsConfig := jujuhttp.SecureTLSConfig()
	tlsConfig.Certificates = []tls.Certificate{tlsCert}
	srv.Server.TLS = tlsConfig

	srv.StartTLS()
	u, _ := url.Parse(srv.URL)
	srv.Addrs = []string{u.Host}
	return srv
}

func (srv *Server) serveAPI(w http.ResponseWriter, req *http.Request) {
	var websocketUpgrader = websocket.Upgrader{}
	conn, err := websocketUpgrader.Upgrade(w, req, nil)
	if err != nil {
		return
	}
	srv.serveConn(req.Context(), conn, req.URL.Query().Get(":modeluuid"))
}

func (srv *Server) serveConn(ctx context.Context, wsConn *websocket.Conn, modelUUID string) {
	codec := jsoncodec.NewWebsocket(wsConn)
	conn := rpc.NewConn(codec, observer.NewRecorderFactory(
		&fakeobserver.Instance{}, nil, observer.NoCaptureArgs))

	r, rootErr := srv.newRoot(modelUUID)
	root := allVersions{
		Value: rpcreflect.ValueOf(reflect.ValueOf(r)),
		err:   rootErr,
	}
	conn.ServeRoot(root, nil, serverError)
	conn.Start(ctx)
	<-conn.Dead()
	conn.Close()
}

func serverError(err error) error {
	return apiservererrors.ServerError(err)
}

// allVersions serves the same methods as would be served
// by rpc.Conn.Serve except that the facade version is ignored.
type allVersions struct {
	err error
	rpcreflect.Value
}

func (av allVersions) FindMethod(rootMethodName string, version int, objMethodName string) (rpcreflect.MethodCaller, error) {
	if av.err != nil {
		return nil, av.err
	}
	return av.Value.FindMethod(rootMethodName, 0, objMethodName)
}

func (av allVersions) StartTrace(ctx context.Context) (context.Context, trace.Span) {
	return ctx, trace.NoopSpan{}
}
