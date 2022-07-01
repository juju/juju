// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package collect

import (
	"fmt"
	"net"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/v2/agent"
	"github.com/juju/juju/v2/worker/fortress"
	"github.com/juju/juju/v2/worker/metrics/spool"
	"github.com/juju/juju/v2/worker/uniter"
)

// handlerConfig stores configuration values for the socketListener.
type handlerConfig struct {
	charmdir       fortress.Guest
	agent          agent.Agent
	unitTag        names.UnitTag
	metricsFactory spool.MetricFactory
	runner         *hookRunner
}

func newHandler(config handlerConfig) *handler {
	return &handler{config: config}
}

type handler struct {
	config handlerConfig
}

// Handle triggers the collect-metrics hook and writes collected metrics
// to the specified connection.
func (l *handler) Handle(c net.Conn, abort <-chan struct{}) error {
	defer func() { _ = c.Close() }()

	// TODO(fwereade): 2016-03-17 lp:1558657
	err := c.SetDeadline(time.Now().Add(spool.DefaultTimeout))
	if err != nil {
		return errors.Annotate(err, "failed to set the deadline")
	}

	err = l.config.charmdir.Visit(func() error {
		return l.do()
	}, abort)
	if err != nil {
		_, _ = fmt.Fprintf(c, "error: %v\n", err.Error())
	} else {
		_, _ = fmt.Fprintf(c, "ok\n")
	}
	return errors.Trace(err)
}

func (l *handler) do() error {
	paths := uniter.NewWorkerPaths(l.config.agent.CurrentConfig().DataDir(), l.config.unitTag, "metrics-collect", nil)
	charmURL, validMetrics, err := readCharm(l.config.unitTag, paths)
	if err != nil {
		return errors.Trace(err)
	}

	recorder, err := l.config.metricsFactory.Recorder(
		validMetrics,
		charmURL,
		l.config.unitTag.String(),
	)
	if err != nil {
		return errors.Annotate(err, "failed to create the metric recorder")
	}
	defer func() { _ = recorder.Close() }()
	err = l.config.runner.do(recorder)
	if err != nil {
		return errors.Annotate(err, "failed to collect metrics")
	}
	return nil
}
