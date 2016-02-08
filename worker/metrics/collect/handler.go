// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package collect

import (
	"fmt"
	"net"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	corecharm "gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/worker/metrics/spool"
)

// handlerConfig stores configuration values for the socketListener.
type handlerConfig struct {
	unitTag        names.UnitTag
	charmURL       *corecharm.URL
	validMetrics   map[string]corecharm.Metric
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
func (l *handler) Handle(c net.Conn) (err error) {
	defer func() {
		if err != nil {
			fmt.Fprintf(c, "%v\n", err.Error())
		} else {
			fmt.Fprintf(c, "ok\n")
		}
		c.Close()
	}()
	err = c.SetDeadline(time.Now().Add(spool.DefaultTimeout))
	if err != nil {
		return errors.Annotate(err, "failed to set the deadline")
	}
	recorder, err := l.config.metricsFactory.Recorder(
		l.config.validMetrics,
		l.config.charmURL.String(),
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
