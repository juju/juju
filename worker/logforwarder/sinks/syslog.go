// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logforwarder

import (
	"github.com/juju/errors"

	"github.com/juju/juju/logfwd/syslog"
	"github.com/juju/juju/worker/logforwarder"
)

func OpenSyslog(cfg logforwarder.LoggingConfig) (logforwarder.LogSink, error) {
	client, name, err := OpenSyslogSender(cfg, syslog.Open)
	if err != nil {
		return nil, errors.Trace(err)
	}
	sink := &logforwarder.LogSink{
		SendCloser: client,
		Name:       name,
	}
	return sink, nil
}

func OpenSyslogSender(cfg logforwarder.LoggingConfig, open func(syslog.RawConfig) (*syslog.Client, error)) (*syslog.Client, string, error) {
	syslogCfg, ok := cfg.LogFwdSyslog()
	if !ok {
		return nil, "", nil // not enabled
	}
	sink := syslogCfg.Host

	client, err := Open(*syslogCfg)
	return client, sink, errors.Trace(err)
}
