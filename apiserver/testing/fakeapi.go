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
	"github.com/juju/utils"

	"github.com/juju/juju/apiserver/observer"
	"github.com/juju/juju/apiserver/observer/fakeobserver"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/jsoncodec"
	"github.com/juju/juju/rpc/rpcreflect"
	"github.com/juju/juju/testing"
)

// Server represents a fake API server. It must be closed
// after use.
type Server struct {
	// Addrs holds the address used for the
	// server, suitable for including in api.Info.Addrs
	Addrs []string

	*httptest.Server
	newRoot func(modelUUID string) interface{}
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
func NewAPIServer(newRoot func(modelUUID string) interface{}) *Server {
	tlsCert, err := tls.X509KeyPair([]byte(testing.ServerCert), []byte(testing.ServerKey))
	if err != nil {
		panic("bad key pair")
	}

	srv := &Server{
		newRoot: newRoot,
	}
	pmux := pat.New()
	pmux.Get("/model/:modeluuid/api", http.HandlerFunc(srv.serveAPI))

	srv.Server = httptest.NewUnstartedServer(pmux)

	tlsConfig := utils.SecureTLSConfig()
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
	conn := rpc.NewConn(codec, observer.NewRecorderFactory(&fakeobserver.Instance{}, nil))

	root := allVersions{
		rpcreflect.ValueOf(reflect.ValueOf(srv.newRoot(modelUUID))),
	}
	conn.ServeRoot(root, nil, nil)
	conn.Start(ctx)
	<-conn.Dead()
	conn.Close()
}

// allVersions serves the same methods as would be served
// by rpc.Conn.Serve except that the facade version is ignored.
type allVersions struct {
	rpcreflect.Value
}

func (av allVersions) FindMethod(rootMethodName string, version int, objMethodName string) (rpcreflect.MethodCaller, error) {
	return av.Value.FindMethod(rootMethodName, 0, objMethodName)
}
