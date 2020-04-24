// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package collect

import (
	"fmt"
	"net"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/worker/fortress"
	"github.com/juju/juju/worker/metrics/spool"
	"github.com/juju/juju/worker/uniter"
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
	defer c.Close()

	// TODO(fwereade): 2016-03-17 lp:1558657
	err := c.SetDeadline(time.Now().Add(spool.DefaultTimeout))
	if err != nil {
		return errors.Annotate(err, "failed to set the deadline")
	}

	err = l.config.charmdir.Visit(func() error {
		return l.do(c)
	}, abort)
	if err != nil {
		fmt.Fprintf(c, "error: %v\n", err.Error())
	} else {
		fmt.Fprintf(c, "ok\n")
	}
	return errors.Trace(err)
}

func (l *handler) do(c net.Conn) error {
	paths := uniter.NewWorkerPaths(l.config.agent.CurrentConfig().DataDir(), l.config.unitTag, "metrics-collect", nil)
	charmURL, validMetrics, err := readCharm(l.config.unitTag, paths)
	if err != nil {
		return errors.Trace(err)
	}

	recorder, err := l.config.metricsFactory.Recorder(
		validMetrics,
		charmURL.String(),
		l.config.unitTag.String(),
	)
	if err != nil {
		return errors.Annotate(err, "failed to create the metric recorder")
	}
	defer recorder.Close()
	err = l.config.runner.do(recorder)
	if err != nil {
		return errors.Annotate(err, "failed to collect metrics")
	}
	return nil
}
