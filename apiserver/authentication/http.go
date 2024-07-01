// Copyright 2023 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"fmt"
	"net/http"

	"github.com/juju/errors"
)

// HTTPStrategicAuthenticator is responsible for trying multiple Authenticators
// until one succeeds or an error is returned that is not equal to NotFound or
// NotImplemented.
type HTTPStrategicAuthenticator []HTTPAuthenticator

// Authenticate implements HTTPAuthenticator and calls each authenticator in
// order.
func (s HTTPStrategicAuthenticator) Authenticate(req *http.Request) (AuthInfo, error) {
	for _, authenticator := range s {
		authInfo, err := authenticator.Authenticate(req)
		if errors.Is(err, errors.NotImplemented) {
			continue
		} else if errors.Is(err, errors.NotFound) {
			continue
		} else if err != nil {
			return AuthInfo{}, err
		}
		return authInfo, nil
	}

	return AuthInfo{}, fmt.Errorf("authentication %w", errors.NotFound)
}
