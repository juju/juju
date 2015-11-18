// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package envcmd

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/api"
)

var ErrConnTimedOut = errors.New("open connection timed out")

// APIOpener provides a way to open a connection to the Juju
// API Server through the named connection.
type APIOpener interface {
	Open(
		openerFunc func(string) (api.Connection, error),
		connectionName string,
	) (api.Connection, error)
}

type passthroughOpener struct{}

// NewPassthroughOpener returns an instance that will just call the opener
// function when Open is called.
func NewPassthroughOpener() APIOpener {
	return &passthroughOpener{}
}

func (p *passthroughOpener) Open(
	openerFunc func(string) (api.Connection, error),
	connectionName string,
) (api.Connection, error) {
	return openerFunc(connectionName)
}

type timeoutOpener struct {
	clock   clock.Clock
	timeout time.Duration
}

// NewTimeoutOpener will call the opener function when Open is called, but if
// the function does not return by the specified timeout, ErrConnTimeOut is
// returned.
func NewTimeoutOpener(
	clock clock.Clock,
	timeout time.Duration) APIOpener {
	return &timeoutOpener{
		clock:   clock,
		timeout: timeout,
	}
}

func (t *timeoutOpener) Open(
	openerFunc func(string) (api.Connection, error),
	connectionName string,
) (api.Connection, error) {
	apic := make(chan api.Connection)
	errc := make(chan error)
	timedOut := make(chan struct{})
	go func() {
		api, dialErr := openerFunc(connectionName)

		// Check to see if we have timed out before we block forever.
		// Well, we take a certain block forever on timeout, to a very
		// small race around blocking forever.
		select {
		case <-timedOut:
			// If we have a valid api, close it.
			if api != nil {
				api.Close()
			}
			return
		default:
			// continue on
		}

		if dialErr != nil {
			errc <- dialErr
			return
		}
		apic <- api
	}()

	var apiRoot api.Connection
	select {
	case err := <-errc:
		return nil, err
	case apiRoot = <-apic:
	case <-t.clock.After(t.timeout):
		close(timedOut)
		return nil, ErrConnTimedOut
	}

	return apiRoot, nil
}
