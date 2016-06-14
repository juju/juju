// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"net/http"

	"github.com/juju/errors"
	"golang.org/x/net/websocket"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

// logStreamEndpointHandler takes requests to stream logs from the DB.
type logStreamEndpointHandler struct {
	stopCh    <-chan struct{}
	newState  func(*http.Request) (state.LogTailerState, error)
	newTailer func(state.LogTailerState, *state.LogTailerParams) (state.LogTailer, error)
}

func newLogStreamEndpointHandler(ctxt httpContext) *logStreamEndpointHandler {
	newState := func(req *http.Request) (state.LogTailerState, error) {
		st, _, err := ctxt.stateForRequestAuthenticatedAgent(req)
		return st, err
	}
	return &logStreamEndpointHandler{
		stopCh:    ctxt.stop(),
		newState:  newState,
		newTailer: state.NewLogTailer,
	}
}

// ServeHTTP will serve up connections as a websocket for the logstream API.
//
// Args for the HTTP request are as follows:
//   all -> string - one of [true, false], if true, include records from all models
//   start -> string - the unix timestamp of where to start ("sec.microsec")
func (eph *logStreamEndpointHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	logger.Infof("log stream request handler starting")
	reqHandler, initial := eph.newLogStreamRequestHandler(req)
	defer reqHandler.tailer.Stop()

	server := websocket.Server{
		Handler: func(conn *websocket.Conn) {
			defer conn.Close()

			reqHandler.serveWebsocket(conn, initial, eph.stopCh)
		},
	}
	server.ServeHTTP(w, req)
}

func (eph *logStreamEndpointHandler) newLogStreamRequestHandler(req *http.Request) (*logStreamRequestHandler, error) {
	// Validate before authenticate because the authentication is
	// dependent on the state connection that is determined during the
	// validation.
	st, err := eph.newState(req)
	if err != nil {
		return nil, errors.Trace(err)
	}

	cfg, err := params.GetLogStreamConfig(req.URL.Query())
	if err != nil {
		return nil, errors.Trace(err)
	}

	tailerArgs := &state.LogTailerParams{
		StartTime: cfg.StartTime,
		AllModels: cfg.AllModels,
	}
	tailer, err := eph.newTailer(st, tailerArgs)
	if err != nil {
		return nil, errors.Trace(err)
	}

	reqHandler := &logStreamRequestHandler{
		req:    req,
		cfg:    cfg,
		tailer: tailer,
	}
	return reqHandler, nil
}

type logStreamRequestHandler struct {
	req    *http.Request
	cfg    params.LogStreamConfig
	tailer state.LogTailer

	stream *apiLogStream
}

func (rh *logStreamRequestHandler) serveWebsocket(conn *websocket.Conn, initial error, stop <-chan struct{}) {
	logger.Infof("log stream request handler starting")

	if ok := rh.initStream(conn, initial); !ok {
		return
	}

	for {
		select {
		case <-stop:
			return
		case rec, ok := <-rh.tailer.Logs():
			if !ok {
				logger.Errorf("tailer stopped: %v", rh.tailer.Err())
				return
			}

			if err := rh.stream.sendRecord(rec); err != nil {
				if isBrokenPipe(err) {
					logger.Tracef("logstream handler stopped (client disconnected)")
				} else {
					logger.Errorf("logstream handler error: %v", err)
				}
			}
		}
	}
}

func (rh *logStreamRequestHandler) initStream(conn *websocket.Conn, initial error) bool {
	stream := &apiLogStream{
		conn:          conn,
		codec:         websocket.JSON,
		sendModelUUID: rh.cfg.AllModels,
	}
	stream.sendInitial(initial)

	rh.stream = stream
	return (initial == nil)
}

type apiLogStream struct {
	conn  *websocket.Conn
	codec websocket.Codec

	sendModelUUID bool
}

func (als *apiLogStream) sendInitial(initial error) {
	// The client is waiting for an indication that the stream
	// is ready (or that it failed).
	// See api/apiclient.go:readInitialStreamError().
	initialCodec := websocket.Codec{
		Marshal: func(v interface{}) (data []byte, payloadType byte, err error) {
			data, payloadType, err = websocket.JSON.Marshal(v)
			if err != nil {
				return data, payloadType, err
			}
			// readInitialStreamError() looks for LF.
			return append(data, '\n'), payloadType, nil
		},
	}
	initialCodec.Send(als.conn, &params.ErrorResult{
		Error: common.ServerError(initial),
	})
}

func (als *apiLogStream) sendRecord(rec *state.LogRecord) error {
	apiRec := params.LogStreamRecord{
		Time:     rec.Time,
		Module:   rec.Module,
		Location: rec.Location,
		Level:    rec.Level.String(),
		Message:  rec.Message,
	}
	if als.sendModelUUID {
		apiRec.ModelUUID = rec.ModelUUID
	}
	if err := als.send(apiRec); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (als *apiLogStream) send(rec params.LogStreamRecord) error {
	return als.codec.Send(als.conn, rec)
}
