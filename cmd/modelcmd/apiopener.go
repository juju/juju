// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelcmd

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/api"
	"github.com/juju/juju/jujuclient"
)

var ErrConnTimedOut = errors.New("open connection timed out")

// APIOpener provides a way to open a connection to the Juju
// API Server through the named connection.
type APIOpener interface {
	Open(store jujuclient.ClientStore, controllerName, accountName, modelName string) (api.Connection, error)
}

type OpenFunc func(jujuclient.ClientStore, string, string, string) (api.Connection, error)

func (f OpenFunc) Open(store jujuclient.ClientStore, controllerName, accountName, modelName string) (api.Connection, error) {
	return f(store, controllerName, accountName, modelName)
}

type timeoutOpener struct {
	opener  APIOpener
	clock   clock.Clock
	timeout time.Duration
}

// NewTimeoutOpener will call the opener when Open is called, but if the
// opener does not return by the specified timeout, ErrConnTimeOut is
// returned.
func NewTimeoutOpener(opener APIOpener, clock clock.Clock, timeout time.Duration) APIOpener {
	return &timeoutOpener{
		opener:  opener,
		clock:   clock,
		timeout: timeout,
	}
}

func (t *timeoutOpener) Open(store jujuclient.ClientStore, controllerName, accountName, modelName string) (api.Connection, error) {
	// Make the channels buffered so the created goroutine is guaranteed
	// not go get blocked trying to send down the channel.
	apic := make(chan api.Connection, 1)
	errc := make(chan error, 1)
	go func() {
		api, dialErr := t.opener.Open(store, controllerName, accountName, modelName)
		if dialErr != nil {
			errc <- dialErr
			return
		}

		select {
		case apic <- api:
			// sent fine
		default:
			// couldn't send, was blocked by the dummy value, must have timed out.
			api.Close()
		}
	}()

	var apiRoot api.Connection
	select {
	case err := <-errc:
		return nil, err
	case apiRoot = <-apic:
	case <-t.clock.After(t.timeout):
		select {
		case apic <- nil:
			// Fill up the buffer on the apic to indicate to the other goroutine
			// that we have timed out.
		case apiRoot = <-apic:
			// We hit that weird edge case where we have both timed out and
			// returned a viable apiRoot at exactly the same time, and the other
			// goroutine managed to send back the apiRoot before we pushed the
			// dummy value.  If this is the case, then we are good, return the
			// apiRoot
			return apiRoot, nil
		}
		return nil, ErrConnTimedOut
	}

	return apiRoot, nil
}
