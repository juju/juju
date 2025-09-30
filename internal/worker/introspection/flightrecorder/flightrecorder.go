// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package flightrecorder

import (
	"net/http"

	"github.com/juju/juju/internal/worker/flightrecorder"
)

// StartHandler returns an http handler for starting the flight recorder.
func StartHandler(w flightrecorder.FlightRecorder) http.HandlerFunc {
	return func(wr http.ResponseWriter, req *http.Request) {
		if err := w.Start(); err != nil {
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
		if err := w.Capture(); err != nil {
			http.Error(wr, err.Error(), http.StatusInternalServerError)
			return
		}
		wr.WriteHeader(http.StatusOK)
	}
}
