// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apicaller

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
)

// ConnectStrategy defines a strategy for opening a API connection.
type ConnectStrategy struct {
	dialOpts         api.DialOpts
	apiOpenFn        api.OpenFunc
	logger           Logger
	fallbackRequired bool
}

// DefaultConnectStrategy creates a connect strategy with the default dial
// options required to open a API connection.
func DefaultConnectStrategy(apiOpenFn api.OpenFunc, logger Logger) *ConnectStrategy {
	return &ConnectStrategy{
		dialOpts: api.DialOpts{
			// The DialTimeout is for connecting to the underlying
			// socket. We use three seconds because it should be fast
			// but it is possible to add a manual machine to a distant
			// controller such that the round trip time could be as high
			// as 500ms.
			DialTimeout: 3 * time.Second,
			// The delay between connecting to a different controller. Setting this to 0 means we try all controllers
			// simultaneously. We set it to approximately how long the TLS handshake takes, to avoid doing TLS
			// handshakes to a controller that we are going to end up ignoring.
			DialAddressInterval: 200 * time.Millisecond,
			// The timeout is for the complete login handshake.
			// If the server is rate limiting, it will normally pause
			// before responding to the login request, but the pause is
			// in the realm of five to ten seconds.
			Timeout: time.Minute,
		},
		apiOpenFn: apiOpenFn,
		logger:    logger,
	}
}

// Connect attempts to connect to a given api using the information provided. If
// the first connection attempt fails then fallback with the supplied fallback
// password.
func (s *ConnectStrategy) Connect(info *api.Info, fallbackPassword string) (api.Connection, error) {
	var (
		err  error
		conn api.Connection
	)
	if info.Password != "" {
		s.logger.Debugf("connecting with current password")

		if conn, err = s.connect(info); err == nil {
			return conn, nil
		}
	}

	// We've perhaps used the wrong password, so
	// try again with the fallback password.
	if params.IsCodeUnauthorized(err) || errors.Cause(err) == common.ErrBadCreds {
		s.logger.Debugf("connecting with old password")

		s.fallbackRequired = true

		infoCopy := *info
		infoCopy.Password = fallbackPassword

		if conn, err = s.connect(&infoCopy); err == nil {
			return conn, nil
		}
	}
	return nil, errors.Trace(err)
}

// RequiredFallback holds the state of if the connection required the fallback
// password or not.
func (s *ConnectStrategy) RequiredFallback() bool {
	return s.fallbackRequired
}

func (s *ConnectStrategy) connect(info *api.Info) (api.Connection, error) {
	return s.apiOpenFn(info, s.dialOpts)
}
