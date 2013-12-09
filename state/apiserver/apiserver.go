// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"code.google.com/p/go.net/websocket"
	"launchpad.net/loggo"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/rpc"
	"launchpad.net/juju-core/rpc/jsoncodec"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
)

var logger = loggo.GetLogger("juju.state.apiserver")

// Server holds the server side of the API.
type Server struct {
	tomb  tomb.Tomb
	wg    sync.WaitGroup
	state *state.State
	addr  net.Addr
}

// Serve serves the given state by accepting requests on the given
// listener, using the given certificate and key (in PEM format) for
// authentication.
func NewServer(s *state.State, addr string, cert, key []byte) (*Server, error) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	logger.Infof("listening on %q", lis.Addr())
	tlsCert, err := tls.X509KeyPair(cert, key)
	if err != nil {
		return nil, err
	}
	srv := &Server{
		state: s,
		addr:  lis.Addr(),
	}
	// TODO(rog) check that *srvRoot is a valid type for using
	// as an RPC server.
	lis = tls.NewListener(lis, &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
	})
	go srv.run(lis)
	return srv, nil
}

// Dead returns a channel that signals when the server has exited.
func (srv *Server) Dead() <-chan struct{} {
	return srv.tomb.Dead()
}

// Stop stops the server and returns when all running requests
// have completed.
func (srv *Server) Stop() error {
	srv.tomb.Kill(nil)
	return srv.tomb.Wait()
}

// Kill implements worker.Worker.Kill.
func (srv *Server) Kill() {
	srv.tomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (srv *Server) Wait() error {
	return srv.tomb.Wait()
}

type requestNotifier struct {
	connCounter int64
	identifier  string
}

var globalCounter int64

func nextCounter() int64 {
	return atomic.AddInt64(&globalCounter, 1)
}

func (n *requestNotifier) SetIdentifier(identifier string) {
	n.identifier = identifier
}

func (n requestNotifier) ServerRequest(hdr *rpc.Header, body interface{}) {
	if hdr.Request.Type == "Pinger" && hdr.Request.Action == "Ping" {
		return
	}
	// TODO(rog) 2013-10-11 remove secrets from some requests.
	logger.Debugf("<- [%X] %s %s", n.connCounter, n.identifier, jsoncodec.DumpRequest(hdr, body))
}

func (n requestNotifier) ServerReply(req rpc.Request, hdr *rpc.Header, body interface{}, timeSpent time.Duration) {
	if req.Type == "Pinger" && req.Action == "Ping" {
		return
	}
	logger.Debugf("-> [%X] %s %s %s %s[%q].%s", n.connCounter, n.identifier, timeSpent, jsoncodec.DumpRequest(hdr, body), req.Type, req.Id, req.Action)
}

func (n requestNotifier) ClientRequest(hdr *rpc.Header, body interface{}) {
}

func (n requestNotifier) ClientReply(req rpc.Request, hdr *rpc.Header, body interface{}) {
}

func (srv *Server) run(lis net.Listener) {
	defer srv.tomb.Done()
	defer srv.wg.Wait() // wait for any outstanding requests to complete.
	srv.wg.Add(1)
	go func() {
		<-srv.tomb.Dying()
		lis.Close()
		srv.wg.Done()
	}()
	handler := websocket.Handler(func(conn *websocket.Conn) {
		srv.wg.Add(1)
		defer srv.wg.Done()
		// If we've got to this stage and the tomb is still
		// alive, we know that any tomb.Kill must occur after we
		// have called wg.Add, so we avoid the possibility of a
		// handler goroutine running after Stop has returned.
		if srv.tomb.Err() != tomb.ErrStillAlive {
			return
		}
		if err := srv.serveConn(conn); err != nil {
			logger.Errorf("error serving RPCs: %v", err)
		}
	})
	mux := http.NewServeMux()
	mux.Handle("/", handler)
	mux.Handle("/charms", http.HandlerFunc(srv.charmsHandler))
	// The error from http.Serve is not interesting.
	http.Serve(lis, mux)
}

// CharmsResponse is the server response to a charm upload request.
type CharmsResponse struct {
	Code     int    `json:"code,omitempty"`
	Error    string `json:"error,omitempty"`
	CharmURL string `json:"charmUrl,omitempty"`
}

func sendJSON(w http.ResponseWriter, response *CharmsResponse) error {
	if response == nil {
		return fmt.Errorf("response is nil")
	}
	w.WriteHeader(response.Code)
	body, err := json.Marshal(response)
	if err != nil {
		return err
	}
	w.Write(body)
	return nil
}

func sendError(w http.ResponseWriter, code int, message string) error {
	if code == 0 {
		// Use code 400 by default.
		code = http.StatusBadRequest
	} else if code == http.StatusOK {
		// Dont' report 200 OK.
		code = 0
	}
	err := sendJSON(w, &CharmsResponse{Code: code, Error: message})
	if err != nil {
		return err
	}
	return nil
}

func (srv *Server) httpAuthenticate(w http.ResponseWriter, r *http.Request) error {
	if r == nil {
		return fmt.Errorf("invalid request")
	}
	parts := strings.SplitN(r.Header.Get("Authorization"), " ", 2)
	if len(parts) != 2 || parts[0] != "Basic" {
		// Invalid header format or no header provided.
		return fmt.Errorf("invalid request format")
	}
	// Challenge is a base64-encoded "tag:pass" string.
	challenge, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return fmt.Errorf("invalid request format")
	}
	tagPass := strings.SplitN(string(challenge), ":", 2)
	if len(tagPass) != 2 {
		return fmt.Errorf("invalid request format")
	}
	_, err = checkCreds(srv.state, params.Creds{
		AuthTag:  tagPass[0],
		Password: tagPass[1],
	})
	return err
}

func (srv *Server) requireHttpAuth(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="juju"`)
	sendError(w, http.StatusUnauthorized, "unauthorized")
}

func (srv *Server) charmsHandler(w http.ResponseWriter, r *http.Request) {
	if err := srv.httpAuthenticate(w, r); err != nil {
		srv.requireHttpAuth(w)
		return
	}
	if r.Method != "POST" {
		sendError(w, http.StatusMethodNotAllowed, fmt.Sprintf("unsupported method: %s", r.Method))
		return
	}
	query := r.URL.Query()
	series := query.Get("series")
	if series == "" {
		sendError(w, 0, "expected series= URL argument")
		return
	}
	reader, err := r.MultipartReader()
	if err != nil {
		sendError(w, 0, err.Error())
		return
	}
	part, err := reader.NextPart()
	if err == io.EOF {
		sendError(w, 0, "expected a single uploaded file, got none")
		return
	} else if err != nil {
		http.Error(w, fmt.Sprintf("cannot process uploaded file: %v", err), http.StatusBadRequest)
		return
	}
	tempFile, err := ioutil.TempFile("", "charm")
	if err != nil {
		sendError(w, http.StatusInternalServerError, fmt.Sprintf("cannot create temp file: %v", err))
		return
	}
	defer tempFile.Close()
	buffer := make([]byte, 100000)
	for {
		numRead, err := part.Read(buffer)
		if err != nil && err != io.EOF {
			sendError(w, http.StatusInternalServerError, fmt.Sprintf("uploaded file read error: %v", err))
			return
		}
		if numRead == 0 {
			break
		}
		if _, err := tempFile.Write(buffer[:numRead]); err != nil {
			sendError(w, http.StatusInternalServerError, fmt.Sprintf("temp file write error: %v", err))
			return
		}
	}
	if _, err := reader.NextPart(); err != io.EOF {
		sendError(w, 0, "expected a single uploaded file, got more")
		return
	}
	archive, err := charm.ReadBundle(tempFile.Name())
	if err != nil {
		sendError(w, 0, fmt.Sprintf("invalid charm archive: %v", err))
		return
	}
	charmUrl := "local:" + series + "/" + archive.Meta().Name
	sendJSON(w, &CharmsResponse{Code: http.StatusOK, CharmURL: charmUrl})
}

// Addr returns the address that the server is listening on.
func (srv *Server) Addr() string {
	return srv.addr.String()
}

func (srv *Server) serveConn(wsConn *websocket.Conn) error {
	codec := jsoncodec.NewWebsocket(wsConn)
	if loggo.GetLogger("juju.rpc.jsoncodec").EffectiveLogLevel() <= loggo.TRACE {
		codec.SetLogging(true)
	}
	var notifier rpc.RequestNotifier
	var loginCallback func(string)
	if logger.EffectiveLogLevel() <= loggo.DEBUG {
		reqNotifier := &requestNotifier{nextCounter(), "<unknown>"}
		loginCallback = reqNotifier.SetIdentifier
		notifier = reqNotifier
	}
	conn := rpc.NewConn(codec, notifier)
	conn.Serve(newStateServer(srv, conn, loginCallback), serverError)
	conn.Start()
	select {
	case <-conn.Dead():
	case <-srv.tomb.Dying():
	}
	return conn.Close()
}

func serverError(err error) error {
	if err := common.ServerError(err); err != nil {
		return err
	}
	return nil
}

var logRequests = true
