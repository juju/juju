// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/juju/clock"
	"github.com/juju/loggo/v2"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/errors"
)

const (
	stateFlushed = "flushed"
	stateTicked  = "ticked"
)

// LogSink is a loggo.Writer that writes log messages to a file.
type LogSink struct {
	tomb           tomb.Tomb
	internalStates chan string

	writer io.WriteCloser

	batchSize     int
	flushInterval time.Duration

	inLogEntry   chan loggo.Entry
	inLogRecords chan []logger.LogRecord

	out chan []logRecord

	clock clock.Clock
}

// NewLogSink creates a new log sink that writes log messages to a file. There
// can only be one writer writing to the same file at a time, otherwise bytes
// will be written to the file in an interleaved manner (junk data). LogSink
// writer will write log messages as JSON objects, one per line, even if the log
// message is multiline. The batchSize parameter specifies the minimum number of
// log messages to batch before writing to the underlying writer. The number of
// entires can far exceed the batchSize if the log messages are large.
// LogSink will take ownership of the writer, and will close it when the worker
// is killed.
func NewLogSink(writer io.WriteCloser, batchSize int, flushInterval time.Duration, clock clock.Clock) *LogSink {
	return newLogSink(writer, batchSize, flushInterval, clock, nil)
}

// newLogSink creates a new log sink that writes log messages to a file.
func newLogSink(
	writer io.WriteCloser,
	batchSize int, flushInterval time.Duration,
	clock clock.Clock,
	internalStates chan string,
) *LogSink {
	w := &LogSink{
		internalStates: internalStates,

		writer: writer,

		batchSize:     batchSize,
		flushInterval: flushInterval,

		inLogEntry:   make(chan loggo.Entry),
		inLogRecords: make(chan []logger.LogRecord),

		out: make(chan []logRecord),

		clock: clock,
	}
	w.tomb.Go(w.loop)
	return w
}

// Write sends a new log message to the writer.
// This implements the loggo.Writer interface.
func (w *LogSink) Write(entry loggo.Entry) {
	select {
	case <-w.tomb.Dying():
		return
	case w.inLogEntry <- entry:
	}
}

// Log writes the given log records to the logger's storage.
func (w *LogSink) Log(records []logger.LogRecord) error {
	select {
	case <-w.tomb.Dying():
		return tomb.ErrDying
	case w.inLogRecords <- records:
		return nil
	}
}

// Kill stops the writer.
func (w *LogSink) Kill() {
	w.tomb.Kill(nil)
}

// Wait blocks until the writer has stopped.
func (w *LogSink) Wait() error {
	return w.tomb.Wait()
}

// Close stops LogSink and closes the underlying writer.
// This is a convenience method that calls Kill and Wait, preventing the
// exposing of the worker type.
func (w *LogSink) Close() error {
	w.Kill()
	return w.Wait()
}

func (w *LogSink) loop() error {
	// When all is said and done we need to close the writer. The LogSink has
	// taken ownership of the writer, and will close it when the worker is
	// killed.
	defer func() { _ = w.writer.Close() }()

	closing := make(chan struct{})
	w.tomb.Go(func() error {
		defer close(closing)

		buffer := new(bytes.Buffer)
		encoder := json.NewEncoder(buffer)

		for {
			select {
			case <-w.tomb.Dying():
				return tomb.ErrDying
			case records := <-w.out:
				if err := w.write(buffer, encoder, records); err != nil {
					return err
				}
			}
		}
	})

	timer := time.NewTimer(w.flushInterval)

	// It would be nice to create a fixed size for all the entries, but that
	// requires splitting the log records into multiple batches. That creates
	// more complexity, when we can just pipe the log entries to the writer in a
	// single batch.
	var entries []logRecord

	// Tick-toc the in and out channels, to ensure that we can send the batch
	// of log messages to the underlying writer.
	inLogEntry := w.inLogEntry
	inLogRecords := w.inLogRecords

	var out chan []logRecord
	var switchToRead = func() {
		inLogEntry = w.inLogEntry
		inLogRecords = w.inLogRecords

		out = nil
	}
	var switchToWrite = func() {
		inLogEntry = nil
		inLogRecords = nil

		out = w.out

		timer.Reset(w.flushInterval)
	}

	for {
		select {
		case <-w.tomb.Dying():
			if len(entries) > 0 {
				// Ensure that we write the remaining log messages before we
				// exit the loop. It requires the write goroutine to be
				// finished before we attempt to write any remaining log
				// messages.
				// If the write goroutine is still writing after a second, we
				// can't wait forever, so we return an error.
				select {
				case <-closing:
				case <-w.clock.After(time.Second):
					return errors.Errorf("failed to write %d log messages: %w", len(entries), tomb.ErrDying)
				}

				// To prevent a race, we need our own buffer and encoder to
				// write the remaining log messages that weren't written yet.
				buffer := new(bytes.Buffer)
				encoder := json.NewEncoder(buffer)
				if err := w.write(buffer, encoder, entries); err != nil {
					return err
				}
			}
			return tomb.ErrDying

		case entry := <-inLogEntry:
			// Consume log entries until we have a full batch.
			entries = append(entries, w.convertLogEntry(entry))
			if len(entries) < w.batchSize {
				continue
			}
			switchToWrite()

		case records := <-inLogRecords:
			// Consume the log records, there is a higher chance that the
			// entries will be larger than the batch size. In that case we
			// just have a larger batch size for the log messages.
			entries = append(entries, w.convertLogRecords(records)...)
			if len(entries) < w.batchSize {
				continue
			}
			switchToWrite()

		case <-timer.C:
			if len(entries) == 0 {
				continue
			}
			switchToWrite()

			w.reportInternalState(stateTicked)

		case out <- entries:
			switchToRead()

			entries = nil
		}
	}
}

func (w *LogSink) convertLogEntry(entry loggo.Entry) logRecord {
	var location string
	if entry.Filename != "" {
		location = fmt.Sprintf("%s:%d", entry.Filename, entry.Line)
	}

	rec := logRecord{
		Time:     entry.Timestamp,
		Module:   entry.Module,
		Location: location,
		Level:    entry.Level.String(),
		Message:  entry.Message,
	}

	if entry.Labels != nil {
		rec.Labels = entry.Labels
		rec.ModelUUID = entry.Labels["model-uuid"]
	}

	return rec
}

func (w *LogSink) convertLogRecords(records []logger.LogRecord) []logRecord {
	var recs []logRecord
	for _, record := range records {
		recs = append(recs, logRecord{
			Time:      record.Time,
			Module:    record.Module,
			Entity:    record.Entity,
			Location:  record.Location,
			Level:     record.Level.String(),
			Message:   record.Message,
			Labels:    record.Labels,
			ModelUUID: record.ModelUUID,
		})
	}
	return recs
}

func (w *LogSink) write(buffer *bytes.Buffer, encoder *json.Encoder, records []logRecord) error {
	if len(records) == 0 {
		return nil
	}

	// Encode all log messages in the batch. In theory it's possible
	// to encode all the records in one go, but that would then
	// require the dropping of the first and last characters of the
	// buffer, to remove the leading and trailing square brackets.
	// This is a simpler approach for now, and the overhead of
	// encoding overhead beats the complexity of the alternative.
	for _, record := range records {
		if err := encoder.Encode(record); err != nil {
			fmt.Fprintf(os.Stderr, "failed to encode log message: %v", err)
		}
	}

	// Write the encoded log messages to the underlying writer.
	if _, err := w.writer.Write(buffer.Bytes()); err != nil {
		// We can't logout to loggo here, as we are the loggo writer. This
		// creates log message loops. Write to stderr instead.
		fmt.Fprintf(os.Stderr, "failed to write log message: %v", err)
	}

	// Reset the buffer for the next batch of log messages.
	buffer.Reset()

	w.reportInternalState(stateFlushed)

	return nil
}

func (w *LogSink) reportInternalState(state string) {
	if w.internalStates == nil {
		return
	}
	select {
	case <-w.tomb.Dying():
	case w.internalStates <- state:
	}
}

type logRecord struct {
	Time      time.Time         `json:"time"`
	Module    string            `json:"module"`
	Entity    string            `json:"entity,omitempty"`
	Location  string            `json:"location,omitempty"`
	Level     string            `json:"level"`
	Message   string            `json:"message"`
	Labels    map[string]string `json:"labels,omitempty"`
	ModelUUID string            `json:"model-uuid,omitempty"`
}
