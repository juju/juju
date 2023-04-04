// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vault

import (
	"net/http"
	"strings"

	"github.com/hashicorp/vault/api"
	"github.com/juju/errors"

	"github.com/juju/juju/secrets"
)

func isNotFound(err error) bool {
	var apiErr *api.ResponseError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusNotFound
	}
	return false
}

func isAlreadyExists(err error, message string) bool {
	var apiErr *api.ResponseError
	if errors.As(err, &apiErr) {
		errMessage := strings.Join(apiErr.Errors, ",")
		return apiErr.StatusCode == http.StatusBadRequest && strings.Contains(errMessage, message)
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
