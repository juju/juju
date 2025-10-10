// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package flightrecorder

import (
	"fmt"
	"net/http"

	"github.com/juju/juju/core/flightrecorder"
)

// StartHandler returns an http handler for starting the flight recorder.
func StartHandler(w flightrecorder.FlightRecorder) http.HandlerFunc {
	return func(wr http.ResponseWriter, req *http.Request) {
		kind, err := flightrecorder.ParseKind(req.URL.Query().Get("kind"))
		if err != nil {
			http.Error(wr, fmt.Sprintf("invalid kind: %v", err), http.StatusBadRequest)
			return
		}
		if err := w.Start(kind); err != nil {
			http.Error(wr, err.Error(), http.StatusInternalServerError)
			return
		}
		wr.WriteHeader(http.StatusOK)
	}
}

// StopHandler returns an http handler for stopping the flight recorder.
func StopHandler(w flightrecorder.FlightRecorder) http.HandlerFunc {
	return func(wr http.ResponseWriter, req *http.Request) {
		if err := w.Stop(); err != nil {
			http.Error(wr, err.Error(), http.StatusInternalServerError)
			return
		}
		wr.WriteHeader(http.StatusOK)
	}
}

// CaptureHandler returns an http handler for capturing a flight recording.
func CaptureHandler(w flightrecorder.FlightRecorder) http.HandlerFunc {
	return func(wr http.ResponseWriter, req *http.Request) {
		kind, err := flightrecorder.ParseKind(req.URL.Query().Get("kind"))
		if err != nil {
			http.Error(wr, fmt.Sprintf("invalid kind: %v", err), http.StatusBadRequest)
			return
		}

		if err := w.Capture(kind); err != nil {
			http.Error(wr, err.Error(), http.StatusInternalServerError)
			return
		}
		wr.WriteHeader(http.StatusOK)
	}
}
