// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spool

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	corecharm "github.com/juju/charm/v7"
	"github.com/juju/errors"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
	"github.com/juju/loggo"
	"github.com/juju/utils"

	"github.com/juju/juju/apiserver/params"
)

var logger = loggo.GetLogger("juju.worker.uniter.metrics")

type errMetricsData struct {
	error
}

// IsMetricsDataError returns true if the error
// cause is errMetricsData.
func IsMetricsDataError(err error) bool {
	_, ok := errors.Cause(err).(*errMetricsData)
	return ok
}

type metricFile struct {
	*os.File
	finalName string
	encodeErr error
}

func createMetricFile(path string) (*metricFile, error) {
	dir, base := filepath.Dir(path), filepath.Base(path)
	if !filepath.IsAbs(dir) {
		return nil, errors.Errorf("not an absolute path: %q", path)
	}

	workUUID, err := utils.NewUUID()
	if err != nil {
		return nil, errors.Trace(err)
	}
	workName := filepath.Join(dir, fmt.Sprintf(".%s.inc-%s", base, workUUID.String()))

	f, err := os.Create(workName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &metricFile{File: f, finalName: path}, nil
}

// Close implements io.Closer.
func (f *metricFile) Close() error {
	err := f.File.Close()
	if err != nil {
		return errors.Trace(err)
	}
	// If the file contents are garbage, don't try and use it.
	if f.encodeErr != nil {
		return nil
	}
	ok, err := utils.MoveFile(f.Name(), f.finalName)
	if err != nil {
		// ok can be true even when there is an error completing the move, on
		// platforms that implement it in multiple steps that can fail
		// separately. POSIX for example, uses link(2) to claim the new
		// location atomically, followed by an unlink(2) to release the old
		// location.
		if !ok {
			return errors.Trace(err)
		}
		logger.Errorf("failed to remove temporary file %q: %v", f.Name(), err)
	}
	return nil
}

// MetricBatch stores the information relevant to a single metrics batch.
type MetricBatch struct {
	CharmURL string         `json:"charmurl"`
	UUID     string         `json:"uuid"`
	Created  time.Time      `json:"created"`
	Metrics  []jujuc.Metric `json:"metrics"`
	UnitTag  string         `json:"unit-tag"`
}

// APIMetricBatch converts the specified MetricBatch to a params.MetricBatch,
// which can then be sent to the controller.
func APIMetricBatch(batch MetricBatch) params.MetricBatchParam {
	metrics := make([]params.Metric, len(batch.Metrics))
	for i, metric := range batch.Metrics {
		metrics[i] = params.Metric{
			Key:    metric.Key,
			Value:  metric.Value,
			Time:   metric.Time,
			Labels: metric.Labels,
		}
	}
	return params.MetricBatchParam{
		Tag: batch.UnitTag,
		Batch: params.MetricBatch{
			UUID:     batch.UUID,
			CharmURL: batch.CharmURL,
			Created:  batch.Created,
			Metrics:  metrics,
		},
	}
}

// MetricMetadata is used to store metadata for the current metric batch.
type MetricMetadata struct {
	CharmURL string    `json:"charmurl"`
	UUID     string    `json:"uuid"`
	Created  time.Time `json:"created"`
	UnitTag  string    `json:"unit-tag"`
}

// JSONMetricRecorder implements the MetricsRecorder interface
// and writes metrics to a spool directory for store-and-forward.
type JSONMetricRecorder struct {
	spoolDir     string
	validMetrics map[string]corecharm.Metric
	charmURL     string
	uuid         utils.UUID
	created      time.Time
	unitTag      string

	lock sync.Mutex

	file io.Closer
	enc  *json.Encoder
}

// MetricRecorderConfig stores configuration data for a metrics recorder.
type MetricRecorderConfig struct {
	SpoolDir string
	Metrics  map[string]corecharm.Metric
	CharmURL string
	UnitTag  string
}

// NewJSONMetricRecorder creates a new JSON metrics recorder.
func NewJSONMetricRecorder(config MetricRecorderConfig) (rec *JSONMetricRecorder, rErr error) {
	mbUUID, err := utils.NewUUID()
	if err != nil {
		return nil, errors.Trace(err)
	}

	recorder := &JSONMetricRecorder{
		spoolDir: config.SpoolDir,
		uuid:     mbUUID,
		charmURL: config.CharmURL,
		// TODO(fwereade): 2016-03-17 lp:1558657
		created:      time.Now().UTC(),
		validMetrics: config.Metrics,
		unitTag:      config.UnitTag,
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

	err := m.file.Close()
	if err != nil {
		return errors.Trace(err)
	}

	// We have an exclusive lock on this metric batch here, because
	// metricsFile.Close was able to rename the final filename atomically.
	//
	// Now write the meta file so that JSONMetricReader discovers a finished
	// pair of files.
	err = m.recordMetaData()
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}

// AddMetric implements the MetricsRecorder interface.
func (m *JSONMetricRecorder) AddMetric(
	key, value string, created time.Time, labels map[string]string) (err error) {
	defer func() {
		if err != nil {
			err = &errMetricsData{err}
		}
	}()
	err = m.validateMetric(key, value)
	if err != nil {
		return errors.Trace(err)
	}
	m.lock.Lock()
	defer m.lock.Unlock()
	return errors.Trace(m.enc.Encode(jujuc.Metric{
		Key:    key,
		Value:  value,
		Time:   created,
		Labels: labels,
	}))
}

func (m *JSONMetricRecorder) validateMetric(key, value string) error {
	if !m.IsDeclaredMetric(key) {
		return errors.Errorf("metric key %q not declared by the charm", key)
	}
	// The largest number of digits that can be returned by strconv.FormatFloat is 24, so
	// choose an arbitrary limit somewhat higher than that.
	if len(value) > 30 {
		return fmt.Errorf("metric value is too large")
	}
	fValue, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fmt.Errorf("invalid value type: expected float, got %q", value)
	}
	if fValue < 0 {
		return fmt.Errorf("invalid value: value must be greater or equal to zero, got %v", value)
	}
	return nil
}

// IsDeclaredMetric returns true if the metric recorder is permitted to store this metric.
// Returns false if the uniter using this recorder doesn't define this metric.
func (m *JSONMetricRecorder) IsDeclaredMetric(key string) bool {
	_, ok := m.validMetrics[key]
	return ok
}

func (m *JSONMetricRecorder) open() error {
	dataFile := filepath.Join(m.spoolDir, m.uuid.String())
	if _, err := os.Stat(dataFile); err != nil && !os.IsNotExist(err) {
		if err != nil {
			return errors.Annotatef(err, "failed to stat file %s", dataFile)
		}
		return errors.Errorf("file %s already exists", dataFile)
	}

	dataWriter, err := createMetricFile(dataFile)
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

func (m *JSONMetricRecorder) recordMetaData() error {
	metaFile := filepath.Join(m.spoolDir, fmt.Sprintf("%s.meta", m.uuid.String()))
	if _, err := os.Stat(metaFile); !os.IsNotExist(err) {
		if err != nil {
			return errors.Annotatef(err, "failed to stat file %s", metaFile)
		}
		return errors.Errorf("file %s already exists", metaFile)
	}

	metadata := MetricMetadata{
		CharmURL: m.charmURL,
		UUID:     m.uuid.String(),
		Created:  m.created,
		UnitTag:  m.unitTag,
	}
	// The use of a metricFile here ensures that the JSONMetricReader will only
	// find a fully-written metafile.
	metaWriter, err := createMetricFile(metaFile)
	if err != nil {
		return errors.Trace(err)
	}
	defer metaWriter.Close()
	enc := json.NewEncoder(metaWriter)
	if err = enc.Encode(metadata); err != nil {
		metaWriter.encodeErr = err
		return errors.Trace(err)
	}
	return nil
}

// JSONMetricsReader reads metrics batches stored in the spool directory.
type JSONMetricReader struct {
	dir string
}

// NewJSONMetricsReader creates a new JSON metrics reader for the specified spool directory.
func NewJSONMetricReader(spoolDir string) (*JSONMetricReader, error) {
	if _, err := os.Stat(spoolDir); err != nil {
		return nil, errors.Annotatef(err, "failed to open spool directory %q", spoolDir)
	}
	return &JSONMetricReader{
		dir: spoolDir,
	}, nil
}

// Read implements the MetricsReader interface.
// Due to the way the batches are stored in the file system,
// they will be returned in an arbitrary order. This does not affect the behavior.
func (r *JSONMetricReader) Read() (_ []MetricBatch, err error) {
	defer func() {
		if err != nil {
			err = &errMetricsData{err}
		}
	}()

	var batches []MetricBatch

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
