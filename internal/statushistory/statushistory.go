// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistory

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"time"

	"github.com/juju/clock"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
)

// Record represents a single record of status information.
type Record struct {
	Kind    status.HistoryKind
	ID      string
	Message string
	Status  string
	Time    string
	Data    map[string]any
}

// Recorder is an interface for recording status information.
type Recorder interface {
	// Record records the given status information.
	Record(context.Context, Record) error
}

// Namespace represents a namespace of the status we're recording.
type Namespace struct {
	Kind status.HistoryKind
	ID   string
}

// WithID returns a new namespace with the given ID.
func (n Namespace) WithID(id string) Namespace {
	return Namespace{
		Kind: n.Kind,
		ID:   id,
	}
}

func (n Namespace) String() string {
	if n.ID == "" {
		return n.Kind.String()
	}
	return n.Kind.String() + " (" + n.ID + ")"
}

// StatusHistory records status information into a generalized way.
type StatusHistory struct {
	recorder Recorder
	clock    clock.Clock
}

// NewStatusHistory creates a new StatusHistory.
func NewStatusHistory(recorder Recorder, clock clock.Clock) *StatusHistory {
	return &StatusHistory{
		recorder: recorder,
		clock:    clock,
	}
}

// RecordStatus records the given status information.
// If the status data cannot be marshalled, it will not be recorded, instead
// the error will be logged under the data_error key.
func (s *StatusHistory) RecordStatus(ctx context.Context, ns Namespace, status status.StatusInfo) error {
	var now time.Time
	if since := status.Since; since != nil && !since.IsZero() {
		now = *since
	} else {
		now = s.clock.Now()
	}

	return s.recorder.Record(ctx, Record{
		Kind:    ns.Kind,
		ID:      ns.ID,
		Message: status.Message,
		Status:  status.Status.String(),
		Time:    now.Format(time.RFC3339),
		Data:    status.Data,
	})
}

// HistoryRecord represents a single record of status information.
type HistoryRecord struct {
	ModelUUID model.UUID
	Kind      status.HistoryKind
	Tag       string
	Status    status.DetailedStatus
}

// Scanner is an interface for reading lines from a source.
type Scanner interface {
	Scan() bool
	Buffer() []byte
	Close() error
	Err() error
}

// StatusHistoryReader is a reader for status history records.
// It reads records from an io.Reader and unmarshals them into Record structs.
type StatusHistoryReader struct {
	modelUUID model.UUID
	scanner   Scanner
}

// NewStatusHistoryReader creates a new StatusHistoryReader that reads from the
// given io.Reader.
func NewStatusHistoryReader(modelUUID model.UUID, scanner Scanner) *StatusHistoryReader {
	return &StatusHistoryReader{
		modelUUID: modelUUID,
		scanner:   scanner,
	}
}

// StatusHistoryReaderFromFile creates a new StatusHistoryReader that reads from
// the given file path. It opens the file for reading and returns a
// StatusHistoryReader.
func ModelStatusHistoryReaderFromFile(modelUUID model.UUID, path string) (*StatusHistoryReader, error) {
	file, err := os.OpenFile(path, os.O_RDONLY, 0600)
	if err != nil {
		return nil, err
	}

	return NewStatusHistoryReader(modelUUID, scannerCloser{
		// bufio.NewScanner default behavior is to read line by line.
		Scanner: bufio.NewScanner(file),
		Closer:  file,
	}), nil
}

type jsonRecord struct {
	ModelUUID model.UUID        `json:"model-uuid"`
	Labels    map[string]string `json:"labels"`
}

// Walk reads the status history records from the reader and applies the
// given function to each record.
func (r *StatusHistoryReader) Walk(fn func(HistoryRecord) (bool, error)) error {
	kinds := status.AllHistoryKind()

	// Read each line of the log file and unmarshal it into a LogRecord.
	// Filter out records that do not match the requested entities.
	for r.scanner.Scan() {
		var rec jsonRecord
		if err := json.Unmarshal(r.scanner.Buffer(), &rec); err != nil {
			continue
		}

		// Check if the record belongs to the current model.
		if rec.ModelUUID == "" || rec.ModelUUID != r.modelUUID {
			continue
		}

		// If the record does not have the requested labels, skip it.
		if len(rec.Labels) == 0 || rec.Labels[categoryKey] != statusHistoryCategory {
			continue
		}

		kind := status.HistoryKind(rec.Labels[kindKey])
		if _, valid := kinds[kind]; !valid {
			continue
		}

		var data map[string]any
		if labelData := rec.Labels[dataKey]; len(labelData) > 0 {
			if err := json.Unmarshal([]byte(labelData), &data); err != nil {
				continue
			}
		}

		var since *time.Time
		if sinceStr := rec.Labels[sinceKey]; len(sinceStr) > 0 {
			if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
				since = &t
			}
		}

		record := status.DetailedStatus{
			Kind:   kind,
			Status: status.Status(rec.Labels[statusKey]),
			Info:   rec.Labels[messageKey],
			Since:  since,
			Data:   data,
		}

		if terminal, err := fn(HistoryRecord{
			ModelUUID: r.modelUUID,
			Kind:      kind,
			Tag:       rec.Labels[namespaceIDKey],
			Status:    record,
		}); err != nil {
			return err
		} else if terminal {
			return nil
		}
	}
	return r.scanner.Err()
}

func (r *StatusHistoryReader) Close() error {
	if r.scanner != nil {
		return r.scanner.Close()
	}
	return nil
}

type scannerCloser struct {
	Scanner *bufio.Scanner
	Closer  io.Closer
}

// Scan advances the Scanner to the next line.
func (s scannerCloser) Scan() bool {
	return s.Scanner.Scan()
}

// Buffer returns the bytes of the current line.
func (s scannerCloser) Buffer() []byte {
	return s.Scanner.Bytes()
}

// Err returns the first non-EOF error that was encountered by the Scanner.
func (s scannerCloser) Err() error {
	return s.Scanner.Err()
}

// Close closes the underlying Closer if it is not nil.
func (s scannerCloser) Close() error {
	if s.Closer != nil {
		return s.Closer.Close()
	}
	return nil
}
