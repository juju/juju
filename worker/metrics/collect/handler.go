// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package collect

import (
	"fmt"
	"io/ioutil"
	"net"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	corecharm "gopkg.in/juju/charm.v6-unstable"

	"github.com/juju/juju/worker/metrics/spool"
)

// handlerConfig stores configuration values for the socketListener.
type handlerConfig struct {
	unitTag      names.UnitTag
	charmURL     *corecharm.URL
	validMetrics map[string]corecharm.Metric
	runner       *hookRunner
}

func newHandler(config handlerConfig) *handler {
	return &handler{config: config}
}

type handler struct {
	config handlerConfig
}

// Handle triggers the collect-metrics hook and writes collected metrics
// to the specified connection.
func (l *handler) Handle(c net.Conn) error {
	defer c.Close()
	err := c.SetDeadline(time.Now().Add(spool.DefaultTimeout))
	if err != nil {
		return errors.Annotate(err, "failed to set the deadline")
	}
	tmpDir, err := ioutil.TempDir("", "metrics-collect")
	if err != nil {
		return errors.Annotate(err, "failed to create a temporary dir")
	}
	recorder, err := spool.NewJSONMetricRecorder(spool.MetricRecorderConfig{
		SpoolDir: tmpDir,
		Metrics:  l.config.validMetrics,
		CharmURL: l.config.charmURL.String(),
		UnitTag:  l.config.unitTag.String(),
	})
	if err != nil {
		return errors.Annotate(err, "failed to create the metric recorder")
	}
	defer recorder.Close()
	err = l.config.runner.do(recorder)
	if err != nil {
		return errors.Annotate(err, "failed to collect metrics")
	}
	_, err = fmt.Fprintf(c, "%v\n", tmpDir)
	if err != nil {
		return errors.Annotate(err, "failed to write the response")
	}
	return nil
}
