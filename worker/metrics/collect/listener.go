// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package collect

import (
	"encoding/json"
	"fmt"
	"net"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils"
	corecharm "gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/juju/sockets"
	"github.com/juju/juju/worker/metrics/spool"
	"github.com/juju/juju/worker/uniter"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

const (
	defaultTimeout         = 3 * time.Second
	defaultNumberOfRetries = 3
	defaultRetryDelay      = 100 * time.Millisecond
)

// socketListenerConfig stores configuration values for the socketListener.
type socketListenerConfig struct {
	unitTag      names.UnitTag
	charmURL     *corecharm.URL
	validMetrics map[string]corecharm.Metric
	paths        uniter.Paths
}

type socketListener struct {
	listener net.Listener
	t        tomb.Tomb

	config socketListenerConfig
}

func newSocketListener(socketPath string, config socketListenerConfig) (*socketListener, error) {
	listener, err := sockets.Listen(socketPath)
	if err != nil {
		return nil, errors.Trace(err)
	}
	sListener := &socketListener{listener: listener, config: config}
	sListener.t.Go(func() error {
		return sListener.loop()
	})
	return sListener, nil
}

// stop closes the listener and releases all resources
// used by the socketListener.
func (l *socketListener) stop() {
	l.t.Killf("stopping")
	err := l.listener.Close()
	if err != nil {
		logger.Errorf("failed to close the collect-metrics listener: %v", err)
	}
	err = l.t.Wait()
	if err != nil {
		logger.Errorf("failed waiting for all goroutines to finish: %v", err)
	}
}

func (l *socketListener) loop() (err error) {
	for {
		conn, err := l.listener.Accept()
		if err != nil {
			break
		}
		l.t.Go(func() error {
			err := l.handle(conn)
			if err != nil {
				// log the error and continue
				logger.Errorf("failed to handle collect-metrics request: %v", err)
			}
			return nil
		})
	}
	select {
	case <-l.t.Dying():
		// stop() has been called, so it is likely that the error
		// from Accept is a direct result of the Listener being closed
		// and can therefore be safely ignored.
		err = nil
	default:
	}
	return
}

// handle triggers the collect-metrics hook and writes collected metrics
// to the specified connection.
func (l *socketListener) handle(c net.Conn) error {
	defer c.Close()
	err := c.SetDeadline(time.Now().Add(defaultTimeout))
	if err != nil {
		return errors.Annotate(err, "failed to set the deadline")
	}
	recorder, err := newInMemMetricRecorder(l.config.unitTag.String(), l.config.charmURL.String(), l.config.validMetrics)
	if err != nil {
		return errors.Annotate(err, "failed to create the metric recorder")
	}
	// we will try running the collect-metrics hook a few times in case
	// we happen to run it at the same time as the regular metrics
	// collection.
	err = l.tryCollectMetrics(recorder)
	if err != nil {
		return errors.Annotate(err, "failed to collect metrics")
	}
	data, err := json.Marshal(recorder.batches)
	if err != nil {
		return errors.Annotate(err, "failed to marshal metrics")
	}
	_, err = fmt.Fprintf(c, "%v\n", string(data))
	if err != nil {
		return errors.Annotate(err, "failed to write the response")
	}
	return nil
}

func (l *socketListener) tryCollectMetrics(recorder *inMemMetricRecorder) error {
	var err error
	for i := 0; i < defaultNumberOfRetries; i++ {
		recorder.reset()
		err = do(l.config.unitTag.String(), l.config.paths, recorder)
		if err == nil {
			return nil
		}
		time.Sleep(defaultRetryDelay)
	}
	return errors.Trace(err)
}

// newInMemMetricRecorder returns a new struct that implements the
// spool.MetricRecorder interface and stores collected metrics
// in memory.
func newInMemMetricRecorder(unitTag string, charmURL string, validMetrics map[string]corecharm.Metric) (*inMemMetricRecorder, error) {
	return &inMemMetricRecorder{
		unitTag:      unitTag,
		charmURL:     charmURL,
		validMetrics: validMetrics,
		batches:      []spool.MetricBatch{},
	}, nil
}

// inMemMetricRecorder implements the spool.MetricRecorder interface. It
// stores the collected metrics in memory.
type inMemMetricRecorder struct {
	batches      []spool.MetricBatch
	unitTag      string
	charmURL     string
	validMetrics map[string]corecharm.Metric
}

// AddMetrics implements the spool.MetricRecorder interface.
func (r *inMemMetricRecorder) AddMetric(key, value string, created time.Time) error {
	r.batches = append(r.batches, spool.MetricBatch{
		CharmURL: r.charmURL,
		UUID:     utils.MustNewUUID().String(),
		Created:  time.Now(),
		UnitTag:  r.unitTag,
		Metrics:  []jujuc.Metric{{Key: key, Value: value, Time: created}},
	})
	return nil
}

// IsDeclaredMetric implements the spool.MetricRecorder interface.
func (r *inMemMetricRecorder) IsDeclaredMetric(key string) bool {
	_, ok := r.validMetrics[key]
	return ok
}

// Close implements the spool.MetricRecorder interface.
func (r *inMemMetricRecorder) Close() error {
	return nil
}

func (r *inMemMetricRecorder) reset() {
	r.batches = []spool.MetricBatch{}
}
