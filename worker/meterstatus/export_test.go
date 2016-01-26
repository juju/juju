// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus

import (
	"github.com/juju/juju/worker/metrics/spool"
)

var (
	NewSocketListener = &newSocketListener
)

type handlerSetterStopper interface {
	SetHandler(spool.ConnectionHandler)
	Stop()
}

func NewSocketListenerFnc(listener handlerSetterStopper) func(string, spool.ConnectionHandler) (stopper, error) {
	return func(_ string, handler spool.ConnectionHandler) (stopper, error) {
		listener.SetHandler(handler)
		return listener, nil
	}
}
