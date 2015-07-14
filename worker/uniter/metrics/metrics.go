// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metrics

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/utils"
	"github.com/juju/utils/fslock"
	corecharm "gopkg.in/juju/charm.v5"

	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

const spoolLockName string = "access"

var lockTimeout = time.Second * 5

// MetricBatch stores the information relevant to a single metrics batch.
type MetricBatch struct {
	CharmURL string         `json:"charmurl"`
	UUID     string         `json:"uuid"`
	Created  time.Time      `json:"created"`
	Metrics  []jujuc.Metric `json:"metrics"`
}

// APIMetricBatch converts the specified MetricBatch to a params.MetricBatch,
// which can then be sent to the state server.
func APIMetricBatch(batch MetricBatch) params.MetricBatch {
	metrics := make([]params.Metric, len(batch.Metrics))
	for i, metric := range batch.Metrics {
		metrics[i] = params.Metric{Key: metric.Key, Value: metric.Value, Time: metric.Time}
	}
	return params.MetricBatch{
		UUID:     batch.UUID,
		CharmURL: batch.CharmURL,
		Created:  batch.Created,
		Metrics:  metrics,
	}
}

// MetricsMetadata is used to store metadata for the current metric batch.
type MetricsMetadata struct {
	CharmURL string    `json:"charmurl"`
	UUID     string    `json:"uuid"`
	Created  time.Time `json:"created"`
}

// JSONMetricRecorder implements the MetricsRecorder interface
// and writes metrics to a spool directory for store-and-forward.
type JSONMetricRecorder struct {
	lock sync.Mutex

	path string

	validMetrics map[string]corecharm.Metric

	file io.Closer
	enc  *json.Encoder
}

// NewJSONMetricRecorder creates a new JSON metrics recorder.
// It checks if the metrics spool directory exists, if it does not - it is created. Then
// it tries to find an unused metric batch UUID 3 times.
func NewJSONMetricRecorder(spoolDir string, metrics map[string]corecharm.Metric, charmURL string) (rec *JSONMetricRecorder, rErr error) {
	lock, err := fslock.NewLock(spoolDir, spoolLockName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := lock.LockWithTimeout(lockTimeout, "initializing recorder"); err != nil {
		return nil, errors.Trace(err)
	}
	defer func() {
		err := lock.Unlock()
		if err != nil && rErr == nil {
			rErr = errors.Trace(err)
			rec = nil
		} else if err != nil {
			rErr = errors.Annotatef(err, "failed to unlock spool directory %q", spoolDir)
		}
	}()

	if err := checkSpoolDir(spoolDir); err != nil {
		return nil, errors.Trace(err)
	}

	mbUUID, err := utils.NewUUID()
	if err != nil {
		return nil, errors.Trace(err)
	}

	metaFile := filepath.Join(spoolDir, fmt.Sprintf("%s.meta", mbUUID.String()))
	dataFile := filepath.Join(spoolDir, mbUUID.String())
	if _, err := os.Stat(metaFile); !os.IsNotExist(err) {
		if err != nil {
			return nil, errors.Annotatef(err, "failed to stat file %s", metaFile)
		}
		return nil, errors.Errorf("file %s already exists", metaFile)
	}
	if _, err := os.Stat(dataFile); err != nil && !os.IsNotExist(err) {
		if err != nil {
			return nil, errors.Annotatef(err, "failed to stat file %s", dataFile)
		}
		return nil, errors.Errorf("file %s already exists", dataFile)
	}

	if err := recordMetaData(metaFile, charmURL, mbUUID.String()); err != nil {
		return nil, errors.Trace(err)
	}

	recorder := &JSONMetricRecorder{
		path:         dataFile,
		validMetrics: metrics,
	}
	if err := recorder.open(); err != nil {
		return nil, errors.Trace(err)
	}
	return recorder, nil
}

// Close implements the MetricsRecorder interface.
func (m *JSONMetricRecorder) Close() error {
	m.lock.Lock()
	defer m.lock.Unlock()
	return errors.Trace(m.file.Close())
}

// AddMetric implements the MetricsRecorder interface.
func (m *JSONMetricRecorder) AddMetric(key, value string, created time.Time) error {
	if _, ok := m.validMetrics[key]; !ok {
		return errors.Errorf("invalid metric key: %v", key)
	}

	m.lock.Lock()
	defer m.lock.Unlock()
	return errors.Trace(m.enc.Encode(jujuc.Metric{Key: key, Value: value, Time: created}))
}

func (m *JSONMetricRecorder) open() error {
	dataWriter, err := os.Create(m.path)
	if err != nil {
		return errors.Trace(err)
	}
	m.file = dataWriter
	m.enc = json.NewEncoder(dataWriter)
	return nil
}

func checkSpoolDir(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		err := os.MkdirAll(path, 0755)
		if err != nil {
			return errors.Trace(err)
		}
	} else if err != nil {
		return errors.Trace(err)
	}
	return nil
}

func recordMetaData(path string, charmURL, UUID string) error {
	metadata := MetricsMetadata{
		CharmURL: charmURL,
		UUID:     UUID,
		Created:  time.Now().UTC(),
	}
	metaWriter, err := os.Create(path)
	if err != nil {
		return errors.Trace(err)
	}
	defer metaWriter.Close()
	enc := json.NewEncoder(metaWriter)
	err = enc.Encode(metadata)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// JSONMetricsReader reads metrics batches stored in the spool directory.
type JSONMetricReader struct {
	dir  string
	lock *fslock.Lock
}

// NewJSONMetricsReader creates a new JSON metrics reader for the specified spool directory.
func NewJSONMetricReader(spoolDir string) (*JSONMetricReader, error) {
	if _, err := os.Stat(spoolDir); err != nil {
		return nil, errors.Annotatef(err, "failed to open spool directory %q", spoolDir)
	}
	lock, err := fslock.NewLock(spoolDir, spoolLockName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &JSONMetricReader{
		lock: lock,
		dir:  spoolDir,
	}, nil
}

// Read implements the MetricsReader interface.
// Due to the way the batches are stored in the file system,
// they will be returned in an arbitrary order. This does not affect the behavior.
func (r *JSONMetricReader) Read() ([]MetricBatch, error) {
	var batches []MetricBatch

	if err := r.lock.LockWithTimeout(lockTimeout, "reading"); err != nil {
		return nil, errors.Trace(err)
	}

	walker := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return errors.Trace(err)
		}
		if info.IsDir() && path != r.dir {
			return filepath.SkipDir
		} else if !strings.HasSuffix(info.Name(), ".meta") {
			return nil
		}

		batch, err := decodeBatch(path)
		if err != nil {
			return errors.Trace(err)
		}
		batch.Metrics, err = decodeMetrics(filepath.Join(r.dir, batch.UUID))
		if err != nil {
			return errors.Trace(err)
		}
		if len(batch.Metrics) > 0 {
			batches = append(batches, batch)
		}
		return nil
	}
	if err := filepath.Walk(r.dir, walker); err != nil {
		return nil, errors.Trace(err)
	}
	return batches, nil
}

// Remove implements the MetricsReader interface.
func (r *JSONMetricReader) Remove(uuid string) error {
	metaFile := filepath.Join(r.dir, fmt.Sprintf("%s.meta", uuid))
	dataFile := filepath.Join(r.dir, uuid)
	err := os.Remove(metaFile)
	if err != nil && !os.IsNotExist(err) {
		return errors.Trace(err)
	}
	err = os.Remove(dataFile)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Close implements the MetricsReader interface.
func (r *JSONMetricReader) Close() error {
	if r.lock.IsLockHeld() {
		return r.lock.Unlock()
	}
	return nil
}

func decodeBatch(file string) (MetricBatch, error) {
	var batch MetricBatch
	f, err := os.Open(file)
	if err != nil {
		return MetricBatch{}, errors.Trace(err)
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	err = dec.Decode(&batch)
	if err != nil {
		return MetricBatch{}, errors.Trace(err)
	}
	return batch, nil
}

func decodeMetrics(file string) ([]jujuc.Metric, error) {
	var metrics []jujuc.Metric
	f, err := os.Open(file)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	for {
		var metric jujuc.Metric
		err := dec.Decode(&metric)
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, errors.Trace(err)
		}
		metrics = append(metrics, metric)
	}
	return metrics, nil
}
