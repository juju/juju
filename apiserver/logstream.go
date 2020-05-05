// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"net/http"
	"time"

	"github.com/gorilla/schema"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/featureflag"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/websocket"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/state"
)

type logStreamSource interface {
	getStart(sink string) (time.Time, error)
	newTailer(state.LogTailerParams) (state.LogTailer, error)
}

type messageWriter interface {
	WriteJSON(v interface{}) error
}

// logStreamEndpointHandler takes requests to stream logs from the DB.
type logStreamEndpointHandler struct {
	stopCh    <-chan struct{}
	newSource func(*http.Request) (logStreamSource, state.PoolHelper, error)
}

func newLogStreamEndpointHandler(ctxt httpContext) *logStreamEndpointHandler {
	newSource := func(req *http.Request) (logStreamSource, state.PoolHelper, error) {
		st, _, err := ctxt.stateForRequestAuthenticatedAgent(req)
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		return &logStreamState{st}, st, nil
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
func (h *logStreamEndpointHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	logger.Infof("log stream request handler starting")
	handler := func(conn *websocket.Conn) {
		defer conn.Close()
		reqHandler, err := h.newLogStreamRequestHandler(conn, req, clock.WallClock)
		if err != nil {
			h.sendError(conn, req, err)
			return
		}
		defer reqHandler.close()

		// If we get to here, no more errors to report, so we report a nil
		// error.  This way the first line of the connection is always a json
		// formatted simple error.
		h.sendError(conn, req, nil)
		reqHandler.serveWebsocket(h.stopCh)
	}
	websocket.Serve(w, req, handler)
}

func (h *logStreamEndpointHandler) newLogStreamRequestHandler(conn messageWriter, req *http.Request, clock clock.Clock) (rh *logStreamRequestHandler, err error) {
	// Validate before authenticate because the authentication is
	// dependent on the state connection that is determined during the
	// validation.
	source, ph, err := h.newSource(req)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer func() {
		if err != nil {
			ph.Release()
		}
	}()

	var cfg params.LogStreamConfig
	query := req.URL.Query()
	query.Del(":modeluuid")
	if err := schema.NewDecoder().Decode(&cfg, query); err != nil {
		return nil, errors.Annotate(err, "decoding schema")
	}

	tailer, err := h.newTailer(source, cfg, clock)
	if err != nil {
		return nil, errors.Annotate(err, "creating new tailer")
	}

	reqHandler := &logStreamRequestHandler{
		conn:       conn,
		req:        req,
		tailer:     tailer,
		poolHelper: ph,
	}
	return reqHandler, nil
}

func (h *logStreamEndpointHandler) newTailer(source logStreamSource, cfg params.LogStreamConfig, clock clock.Clock) (state.LogTailer, error) {
	start, err := source.getStart(cfg.Sink)
	if err != nil {
		return nil, errors.Annotate(err, "getting log start position")
	}
	if cfg.MaxLookbackDuration != "" {
		d, err := time.ParseDuration(cfg.MaxLookbackDuration)
		if err != nil {
			return nil, errors.Annotatef(err, "invalid lookback duration")
		}
		now := clock.Now()
		if now.Sub(start) > d {
			start = now.Add(-1 * d)
		}
	}

	tailerArgs := state.LogTailerParams{
		StartTime:    start,
		InitialLines: cfg.MaxLookbackRecords,
	}
	tailer, err := source.newTailer(tailerArgs)
	if err != nil {
		return nil, errors.Annotate(err, "tailing logs")
	}
	return tailer, nil
}

// sendError sends a JSON-encoded error response.
func (h *logStreamEndpointHandler) sendError(ws *websocket.Conn, req *http.Request, err error) {
	// There is no need to log the error for normal operators as there is nothing
	// they can action. This is for developers.
	if err != nil && featureflag.Enabled(feature.DeveloperMode) {
		logger.Errorf("returning error from %s %s: %s", req.Method, req.URL.Path, errors.Details(err))
	}
	if sendErr := ws.SendInitialErrorV0(err); sendErr != nil {
		logger.Errorf("closing websocket, %v", err)
		ws.Close()
	}
}

// logStreamState is an implementation of logStreamSource.
type logStreamState struct {
	state.LogTailerState
}

func (st logStreamState) getStart(sink string) (time.Time, error) {
	tracker := state.NewLastSentLogTracker(st, st.ModelUUID(), sink)
	defer tracker.Close()

	// Resume for the sink...
	_, lastSentTimestamp, err := tracker.Get()
	if errors.Cause(err) == state.ErrNeverForwarded {
		// If we've never forwarded a message, we start from
		// position zero.
		lastSentTimestamp = 0
	} else if err != nil {
		return time.Time{}, errors.Trace(err)
	}

	// Using the same timestamp will cause at least 1 duplicate
	// entry, but that is better than dropping records.
	// TODO(ericsnow) Add 1 to start once we track by sequential int ID
	// instead of by timestamp.
	return time.Unix(0, lastSentTimestamp), nil
}

func (st logStreamState) newTailer(args state.LogTailerParams) (state.LogTailer, error) {
	tailer, err := state.NewLogTailer(st, args)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return tailer, nil
}

type logStreamRequestHandler struct {
	conn       messageWriter
	req        *http.Request
	tailer     state.LogTailer
	poolHelper state.PoolHelper
}

func (h *logStreamRequestHandler) serveWebsocket(stop <-chan struct{}) {
	logger.Infof("log stream request handler starting")

	// TODO(wallyworld) - we currently only send one record at a time, but the API allows for
	// sending batches of records, so we need to batch up the output from tailer.Logs().
	for {
		select {
		case <-stop:
			return
		case rec, ok := <-h.tailer.Logs():
			if !ok {
				logger.Errorf("tailer stopped: %v", h.tailer.Err())
				return
			}
			if err := h.sendRecords([]*state.LogRecord{rec}); err != nil {
				if isBrokenPipe(err) {
					logger.Tracef("logstream handler stopped (client disconnected)")
				} else {
					logger.Errorf("logstream handler error: %v", err)
				}
			}
		}
	}
}

func (h *logStreamRequestHandler) close() {
	h.tailer.Stop()
	h.poolHelper.Release()
}

func (h *logStreamRequestHandler) sendRecords(rec []*state.LogRecord) error {
	apiRec := h.apiFromRecords(rec)
	return errors.Trace(h.conn.WriteJSON(apiRec))
}

func (h *logStreamRequestHandler) apiFromRecords(records []*state.LogRecord) params.LogStreamRecords {
	var result params.LogStreamRecords
	result.Records = make([]params.LogStreamRecord, len(records))
	for i, rec := range records {
		apiRec := params.LogStreamRecord{
			ID:        rec.ID,
			ModelUUID: rec.ModelUUID,
			Version:   rec.Version.String(),
			Entity:    rec.Entity,
			Timestamp: rec.Time,
			Module:    rec.Module,
			Location:  rec.Location,
			Level:     rec.Level.String(),
			Message:   rec.Message,
		}
		result.Records[i] = apiRec
	}
	return result
}
