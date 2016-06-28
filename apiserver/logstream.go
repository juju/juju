// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"net/http"

	"github.com/gorilla/schema"
	"github.com/juju/errors"
	"golang.org/x/net/websocket"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
)

type logStreamSource interface {
	getStart(sink string, allModels bool) (int64, error)
	newTailer(*state.LogTailerParams) (state.LogTailer, error)
}

// logStreamEndpointHandler takes requests to stream logs from the DB.
type logStreamEndpointHandler struct {
	stopCh    <-chan struct{}
	newSource func(*http.Request) (logStreamSource, error)
}

func newLogStreamEndpointHandler(ctxt httpContext) *logStreamEndpointHandler {
	newSource := func(req *http.Request) (logStreamSource, error) {
		st, _, err := ctxt.stateForRequestAuthenticatedAgent(req)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return &logStreamState{st}, nil
	}
	return &logStreamEndpointHandler{
		stopCh:    ctxt.stop(),
		newSource: newSource,
	}
}

// ServeHTTP will serve up connections as a websocket for the logstream API.
//
// Args for the HTTP request are as follows:
//   all -> string - one of [true, false], if true, include records from all models
//   sink -> string - the name of the the log forwarding target
func (eph *logStreamEndpointHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	logger.Infof("log stream request handler starting")
	server := websocket.Server{
		Handler: func(conn *websocket.Conn) {
			defer conn.Close()
			reqHandler, err := eph.newLogStreamRequestHandler(req)
			if err == nil {
				defer reqHandler.tailer.Stop()
			}
			stream, initErr := initStream(conn, err)
			if initErr != nil {
				logger.Debugf("failed to send initial error (%v): %v", err, initErr)
				return
			}
			if err != nil {
				return
			}
			reqHandler.serveWebsocket(conn, stream, eph.stopCh)
		},
	}
	server.ServeHTTP(w, req)
}

func (eph *logStreamEndpointHandler) newLogStreamRequestHandler(req *http.Request) (*logStreamRequestHandler, error) {
	// Validate before authenticate because the authentication is
	// dependent on the state connection that is determined during the
	// validation.
	source, err := eph.newSource(req)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var cfg params.LogStreamConfig
	query := req.URL.Query()
	query.Del(":modeluuid")
	if err := schema.NewDecoder().Decode(&cfg, query); err != nil {
		return nil, errors.Annotate(err, "decoding schema")
	}

	tailer, err := eph.newTailer(source, cfg)
	if err != nil {
		return nil, errors.Annotate(err, "creating new tailer")
	}

	reqHandler := &logStreamRequestHandler{
		req:           req,
		tailer:        tailer,
		sendModelUUID: cfg.AllModels,
	}
	return reqHandler, nil
}

func (eph logStreamEndpointHandler) newTailer(source logStreamSource, cfg params.LogStreamConfig) (state.LogTailer, error) {
	start, err := source.getStart(cfg.Sink, cfg.AllModels)
	if err != nil {
		return nil, errors.Annotate(err, "getting log start position")
	}
	tailerArgs := &state.LogTailerParams{
		StartID:   start,
		AllModels: cfg.AllModels,
	}
	tailer, err := source.newTailer(tailerArgs)
	if err != nil {
		return nil, errors.Annotate(err, "tailing logs")
	}
	return tailer, nil
}

// logStreamState is an implementation of logStreamSource.
type logStreamState struct {
	state.LogTailerState
}

func (st logStreamState) getStart(sink string, allModels bool) (int64, error) {
	tracker := state.NewLastSentLogTracker(st, sink)
	if allModels {
		allTracker, err := state.NewAllLastSentLogTracker(st, sink)
		if err != nil {
			return 0, errors.Trace(err)
		}
		tracker = allTracker
	}

	// Resume for the sink...
	lastSent, err := tracker.Get()
	if errors.Cause(err) == state.ErrNeverForwarded {
		// If we've never forwarded a message, we start from
		// position zero.
		lastSent = 0
	} else if err != nil {
		return 0, errors.Trace(err)
	}

	// Using the same timestamp will cause at least 1 duplicate
	// entry, but that is better than dropping records.
	// TODO(ericsnow) Add 1 to start once we track by sequential int ID
	// instead of by timestamp.
	start := lastSent

	return start, nil
}

func (st logStreamState) newTailer(args *state.LogTailerParams) (state.LogTailer, error) {
	tailer, err := state.NewLogTailer(st, args)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return tailer, nil
}

type logStreamRequestHandler struct {
	req           *http.Request
	tailer        state.LogTailer
	sendModelUUID bool

	stream *apiLogStream
}

func (rh *logStreamRequestHandler) serveWebsocket(conn *websocket.Conn, stream *apiLogStream, stop <-chan struct{}) {
	logger.Infof("log stream request handler starting")

	for {
		select {
		case <-stop:
			return
		case rec, ok := <-rh.tailer.Logs():
			if !ok {
				logger.Errorf("tailer stopped: %v", rh.tailer.Err())
				return
			}
			if err := stream.sendRecord(rec, rh.sendModelUUID); err != nil {
				if isBrokenPipe(err) {
					logger.Tracef("logstream handler stopped (client disconnected)")
				} else {
					logger.Errorf("logstream handler error: %v", err)
				}
			}
		}
	}
}

func initStream(conn *websocket.Conn, initial error) (*apiLogStream, error) {
	stream := &apiLogStream{
		conn:  conn,
		codec: websocket.JSON,
	}
	if err := stream.sendInitial(initial); err != nil {
		return nil, errors.Trace(err)
	}
	return stream, nil
}

type apiLogStream struct {
	conn  *websocket.Conn
	codec websocket.Codec
}

func (als *apiLogStream) sendInitial(err error) error {
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
	return initialCodec.Send(als.conn, &params.ErrorResult{
		Error: common.ServerError(err),
	})
}

func (als *apiLogStream) sendRecord(rec *state.LogRecord, sendModelUUID bool) error {
	apiRec := als.apiFromRec(rec, sendModelUUID)
	if err := als.send(apiRec); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func (als *apiLogStream) send(rec params.LogStreamRecord) error {
	return als.codec.Send(als.conn, rec)
}

func (als *apiLogStream) apiFromRec(rec *state.LogRecord, sendModelUUID bool) params.LogStreamRecord {
	apiRec := params.LogStreamRecord{
		ID:        rec.ID,
		Version:   rec.Version.String(),
		Entity:    rec.Entity.String(),
		Timestamp: rec.Time,
		Module:    rec.Module,
		Location:  rec.Location,
		Level:     rec.Level.String(),
		Message:   rec.Message,
	}
	if sendModelUUID {
		apiRec.ModelUUID = rec.ModelUUID
	}
	return apiRec
}
