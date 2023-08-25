// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sinks

import (
	"github.com/juju/errors"

	"github.com/juju/juju/internal/logfwd"
	"github.com/juju/juju/internal/logfwd/syslog"
	"github.com/juju/juju/worker/logforwarder"
)

// OpenSyslog returns a sink used to receive log messages to be forwarded.
func OpenSyslog(cfg *syslog.RawConfig) (*logforwarder.LogSink, error) {
	if !cfg.Enabled {
		return nil, errors.New("log forwarding not enabled")
	}
	client, err := syslog.Open(*cfg)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if client == nil {
		// TODO(axw) we should be returning an error
		// which we interpret up the stack.
		return &logforwarder.LogSink{
			SendCloser: emptySendCloser{},
		}, nil
	}
	sink := &logforwarder.LogSink{
		SendCloser: client,
	}
	return sink, nil
}

type emptySendCloser struct{}

func (emptySendCloser) Send([]logfwd.Record) error {
	return nil
}

func (emptySendCloser) Close() error {
	return nil
}
