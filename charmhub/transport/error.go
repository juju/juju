// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package transport

// APIError represents the error from the CharmHub API.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
