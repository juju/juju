// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"net/http"

	"github.com/juju/errors"

	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/params"
)

// The following lease related error types are situated here to prevent core
// packaging having any API related information and the API client knowing too
// much about the api-server.

// leaseErrorInfoMap turns a lease error into one that can rehydrated on the
// other side of the API call.
func leaseErrorInfoMap(err error) map[string]interface{} {
	m := make(map[string]interface{})
	switch errors.Cause(err) {
	case lease.ErrInvalid:
		m["type"] = "lease-invalid"
	case lease.ErrHeld:
		m["type"] = "lease-held"
	case lease.ErrTimeout:
		m["type"] = "lease-timeout"
	case lease.ErrAborted:
		m["type"] = "lease-aborted"
	case lease.ErrNotHeld:
		m["type"] = "lease-not-held"
	case lease.ErrDropped:
		m["type"] = "lease-dropped"
	}
	return m
}

func rehydrateLeaseError(err error) error {
	e, ok := err.(*params.Error)
	if !ok {
		return err
	}
	leaseErrType, _ := e.Info["type"].(string)
	switch leaseErrType {
	case "lease-invalid":
		return lease.ErrInvalid
	case "lease-held":
		return lease.ErrHeld
	case "lease-timeout":
		return lease.ErrTimeout
	case "lease-aborted":
		return lease.ErrAborted
	case "lease-not-held":
		return lease.ErrNotHeld
	case "lease-dropped":
		return lease.ErrDropped
	}

	return err
}

func leaseStatusCode(err error) int {
	e, ok := err.(*params.Error)
	if !ok {
		return http.StatusInternalServerError
	}
	leaseErrType, _ := e.Info["type"].(string)
	switch leaseErrType {
	case "lease-timeout", "lease-dropped":
		return http.StatusInternalServerError
	}
	return http.StatusBadRequest
}
