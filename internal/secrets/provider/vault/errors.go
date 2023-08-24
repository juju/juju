// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vault

import (
	"net/http"
	"strings"

	"github.com/hashicorp/vault/api"
	"github.com/juju/errors"

	"github.com/juju/juju/internal/secrets"
)

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *api.ResponseError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusNotFound
	}
	// Sadly we can just get a string from the api.
	return strings.Contains(err.Error(), "no secret found")
}

func isAlreadyExists(err error, message string) bool {
	var apiErr *api.ResponseError
	if errors.As(err, &apiErr) {
		errMessage := strings.Join(apiErr.Errors, ",")
		return apiErr.StatusCode == http.StatusBadRequest && strings.Contains(errMessage, message)
	}
	return false
}

func isMountNotFound(err error) bool {
	var apiErr *api.ResponseError
	if errors.As(err, &apiErr) {
		errMessage := strings.Join(apiErr.Errors, ",")
		return apiErr.StatusCode == http.StatusBadRequest && strings.Contains(errMessage, "no matching mount")
	}
	return false
}

func maybePermissionDenied(err error) error {
	var apiErr *api.ResponseError
	if errors.As(err, &apiErr) {
		if apiErr.StatusCode == http.StatusForbidden {
			return errors.WithType(err, secrets.PermissionDenied)
		}
	}
	return err
}

func isPermissionDenied(err error) bool {
	var apiErr *api.ResponseError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusForbidden
	}
	return false
}
