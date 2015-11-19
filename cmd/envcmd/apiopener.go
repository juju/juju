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

// Opener provides a way to open a connection to the Juju
// API Server through the named connection.
type Opener interface {
	Open(connectionName string) (api.Connection, error)
}

type passthroughOpener struct {
	fn func(string) (api.Connection, error)
}

// NewPassthroughOpener returns an instance that will just call the opener
// function when Open is called.
func NewPassthroughOpener(opener func(string) (api.Connection, error)) Opener {
	return &passthroughOpener{fn: opener}
}

func (p *passthroughOpener) Open(name string) (api.Connection, error) {
	return p.fn(name)
}

type timeoutOpener struct {
	fn      func(string) (api.Connection, error)
	clock   clock.Clock
	timeout time.Duration
}

// NewTimeoutOpener will call the opener function when Open is called, but if
// the function does not return by the specified timeout, ErrConnTimeOut is
// returned.
func NewTimeoutOpener(
	opener func(string) (api.Connection, error),
	clock clock.Clock,
	timeout time.Duration) Opener {
	return &timeoutOpener{
		fn:      opener,
		clock:   clock,
		timeout: timeout,
	}
}

func (t *timeoutOpener) Open(name string) (api.Connection, error) {
	// Make the cannels buffered so the created go routine is guaranteed
	// not go get blocked trying to send down the channel.
	apic := make(chan api.Connection, 1)
	errc := make(chan error, 1)
	timedOut := make(chan struct{})
	go func() {
		api, dialErr := t.fn(name)

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
