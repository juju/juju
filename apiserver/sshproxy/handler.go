// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshproxy

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

const (
	errConnHijacked errors.ConstError = "connection hijacked"
)

func Handler(logger loggo.Logger, authFunc func(*http.Request) (*state.PooledState, error)) http.Handler {
	handler := &sshHandler{
		logger:   logger,
		authFunc: authFunc,
	}
	router := mux.NewRouter()
	router.Handle("/model/{model}/machine/{machine:[a-z0-9/]+}/ssh", handler).GetPathRegexp()
	router.Handle("/model/{model}/application/{application}/unit/{unit}/ssh", handler)
	return router
}

type sshHandler struct {
	logger   loggo.Logger
	authFunc func(*http.Request) (*state.PooledState, error)
}

func (h *sshHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	send := func(err error) {
		if err := sendError(w, err); err != nil {
			h.logger.Errorf("%v", err)
		}
	}
	if r.Method != "CONNECT" ||
		strings.ToLower(r.Header.Get("Connection")) != "upgrade" ||
		strings.ToLower(r.Header.Get("Upgrade")) != "ssh" {
		send(apiservererrors.ErrBadRequest)
		return
	}

	vars := mux.Vars(r)
	var tag names.Tag
	if machine, ok := vars["machine"]; ok {
		if !names.IsValidMachine(machine) {
			send(apiservererrors.ErrBadRequest)
			return
		}
		tag = names.NewMachineTag(machine)
	} else if app, ok := vars["application"]; ok {
		unit, ok := vars["unit"]
		if !ok {
			send(apiservererrors.ErrBadRequest)
			return
		}
		unitName := fmt.Sprintf("%s/%s", app, unit)
		if !names.IsValidUnit(unitName) {
			send(apiservererrors.ErrBadRequest)
			return
		}
		tag = names.NewUnitTag(unitName)
	} else {
		send(apiservererrors.ErrBadRequest)
		return
	}

	st, err := h.authFunc(r)
	if err != nil {
		send(fmt.Errorf("cannot auth: %w", err))
		return
	}
	defer st.Release()

	m, err := st.Model()
	if err != nil {
		send(fmt.Errorf("cannot get model: %w", err))
		return
	}

	switch m.Type() {
	case state.ModelTypeIAAS:
		err := h.handleIAAS(st, w, r, tag)
		if err != nil {
			send(fmt.Errorf("cannot handle for model: %w", err))
		}
		return
	case state.ModelTypeCAAS:
		err := h.handleCAAS(st, m, w, r, tag, r.URL.Query().Get("container"))
		if err != nil {
			send(fmt.Errorf("cannot handle for k8s model: %w", err))
		}
		return
	default:
		send(apiservererrors.ErrBadRequest)
		return
	}
}

func (h *sshHandler) hijack(w http.ResponseWriter) (net.Conn, error) {
	conn, buf, err := http.NewResponseController(w).Hijack()
	if err != nil {
		return nil, err
	}

	buf.Writer.Flush()
	if buf.Reader.Buffered() > 0 {
		// handle if there is already stuff buffered here...
	}

	res := http.Response{
		Proto:      "HTTP/1.1",
		StatusCode: http.StatusSwitchingProtocols,
		Status:     "101 Switching Protocols",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header: http.Header{
			"Upgrade":    []string{"ssh"},
			"Connection": []string{"Upgrade"},
		},
		ContentLength: 0,
	}
	conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	err = res.Write(conn)
	if err != nil {
		conn.Close()
		return nil, err
	}

	conn.SetWriteDeadline(time.Time{})
	conn.SetReadDeadline(time.Time{})
	return conn, nil
}

// sendStatusAndJSON sends an HTTP status code and
// a JSON-encoded response to a client.
func sendStatusAndJSON(w http.ResponseWriter, statusCode int, response interface{}) error {
	body, err := json.Marshal(response)
	if err != nil {
		return errors.Errorf("cannot marshal JSON result %#v: %v", response, err)
	}
	if statusCode == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", `Basic realm="juju"`)
	}
	w.Header().Set("Content-Type", params.ContentTypeJSON)
	w.Header().Set("Content-Length", fmt.Sprint(len(body)))
	w.WriteHeader(statusCode)
	if _, err := w.Write(body); err != nil {
		return errors.Annotate(err, "cannot write response")
	}
	return nil
}

// sendError sends a JSON-encoded error response
// for errors encountered during processing.
func sendError(w http.ResponseWriter, errToSend error) error {
	if errors.Is(errToSend, errConnHijacked) {
		return nil
	}
	paramsErr, statusCode := apiservererrors.ServerErrorAndStatus(errToSend)
	return errors.Trace(sendStatusAndJSON(w, statusCode, &params.ErrorResult{
		Error: paramsErr,
	}))
}
