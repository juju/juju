// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sender

import "github.com/juju/juju/internal/worker/metrics/spool"

var (
	NewSender            = newSender
	NewListener          = &newListener
	NewMetricAdderClient = newMetricAdderClient
	SocketName           = &socketName
)

type handlerStopper interface {
	spool.ConnectionHandler
	Stop() error
}

func NewListenerFunc(listener handlerStopper) func(spool.ConnectionHandler, string, string) (stopper, error) {
	return func(spool.ConnectionHandler, string, string) (stopper, error) {
		return listener, nil
	}
}
