// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package transport

import (
	"strings"

	"github.com/pkg/errors"
)

// APIError represents the error from the CharmHub API.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// APIErrors represents a slice of APIError's
type APIErrors []APIError

// Combine will combine any errors into one error.
func (a APIErrors) Combine() error {
	if len(a) > 0 {
		var combined []string
		for _, err := range a {
			if err.Message != "" {
				combined = append(combined, err.Message)
			}
		}
		return errors.Errorf(strings.Join(combined, "\n"))
	}
	return nil
}
