// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package spool contains the implementation of a
// worker that extracts the spool directory path from the agent
// config and enables other workers to write and read
// metrics to and from a the spool directory using a writer
// and a reader.
package spool

import (
	"time"

	"github.com/juju/errors"
	corecharm "gopkg.in/juju/charm.v6-unstable"
	"launchpad.net/tomb"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/dependency"
	"github.com/juju/juju/worker/util"
)

// MetricRecorder records metrics to a spool directory.
type MetricRecorder interface {
	// AddMetric records a metric with the specified key, value and create time
	// to a spool directory.
	AddMetric(key, value string, created time.Time) error
	// Close implements io.Closer.
	Close() error
	// IsDeclaredMetrics returns true if the metric recorder
	// is permitted to store metrics with the specified key.
	IsDeclaredMetric(key string) bool
}

// MetricReader reads metrics from a spool directory.
type MetricReader interface {
	// Read returns all metric batches stored in the spool directory.
	Read() ([]MetricBatch, error)
	// Remove removes the metric batch with the specified uuid
	// from the spool directory.
	Remove(uuid string) error
	// Close implements io.Closer.
	Close() error
}

// MetricFactory contains the metrics reader and recorder factories.
type MetricFactory interface {
	// Recorder returns a new MetricRecorder.
	Recorder(metrics map[string]corecharm.Metric, charmURL, unitTag string) (MetricRecorder, error)

	// Reader returns a new MetricReader.
	Reader() (MetricReader, error)
}

type factory struct {
	spoolDir string
}

// Reader implements the MetricFactory interface.
func (f *factory) Reader() (MetricReader, error) {
	return NewJSONMetricReader(f.spoolDir)
}

// Recorder implements the MetricFactory interface.
func (f *factory) Recorder(declaredMetrics map[string]corecharm.Metric, charmURL, unitTag string) (MetricRecorder, error) {
	return NewJSONMetricRecorder(MetricRecorderConfig{
		SpoolDir: f.spoolDir,
		Metrics:  declaredMetrics,
		CharmURL: charmURL,
		UnitTag:  unitTag,
	})
}

var newFactory = func(spoolDir string) MetricFactory {
	return &factory{spoolDir: spoolDir}
}

// ManifoldConfig specifies names a spooldirectory manifold should use to
// address its dependencies.
type ManifoldConfig util.AgentManifoldConfig

// Manifold returns a dependency.Manifold that extracts the metrics
// spool directory path from the agent.
func Manifold(config ManifoldConfig) dependency.Manifold {
	manifold := util.AgentManifold(util.AgentManifoldConfig(config), newWorker)
	manifold.Output = outputFunc
	return manifold
}

// newWorker creates a degenerate worker that provides access to the metrics
// spool directory path.
func newWorker(a agent.Agent) (worker.Worker, error) {
	metricsSpoolDir := a.CurrentConfig().MetricsSpoolDir()
	err := checkSpoolDir(metricsSpoolDir)
	if err != nil {
		return nil, errors.Annotatef(err, "error checking spool directory %q", metricsSpoolDir)
	}
	w := &spoolWorker{factory: newFactory(metricsSpoolDir)}
	go func() {
		defer w.tomb.Done()
		<-w.tomb.Dying()
	}()
	return w, nil
}

// outputFunc extracts the metrics spool directory path from a *metricsSpoolDirWorker.
func outputFunc(in worker.Worker, out interface{}) error {
	inWorker, _ := in.(*spoolWorker)
	outPointer, _ := out.(*MetricFactory)
	if inWorker == nil || outPointer == nil {
		return errors.Errorf("expected %T->%T; got %T->%T", inWorker, outPointer, in, out)
	}
	*outPointer = inWorker.factory
	return nil
}

// spoolWorker is a worker that provides a MetricFactory.
type spoolWorker struct {
	tomb    tomb.Tomb
	factory MetricFactory
}

// Kill is part of the worker.Worker interface.
func (w *spoolWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *spoolWorker) Wait() error {
	return w.tomb.Wait()
}
