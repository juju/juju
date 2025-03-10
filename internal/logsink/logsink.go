// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/juju/loggo/v2"
	"gopkg.in/tomb.v2"
)

const (
	stateFlushed = "flushed"
	stateTicked  = "ticked"
)

var (
	zeroTime = time.Time{}
)

// NewWriterFunc is a function that creates a new writer.
type NewWriterFunc func() (io.WriteCloser, error)

// LogSink is a loggo.Writer that writes log messages to a file.
type LogSink struct {
	tomb           tomb.Tomb
	internalStates chan string

	writer io.Writer

	batchSize     int
	flushInterval time.Duration

	in  chan loggo.Entry
	out chan []logRecord

	pool sync.Pool
}

// NewLogSink creates a new log sink that writes log messages to a file. There
// can only be one writer writing to the same file at a time, otherwise bytes
// will be written to the file in an interleaved manner (junk data).
// LogSink writer will write log messages as JSON objects, one per line, even
// if the log message is multiline.
func NewLogSink(writer io.Writer, batchSize int, flushInterval time.Duration) *LogSink {
	return newLogSink(writer, batchSize, flushInterval, nil)
}

// newLogSink creates a new log sink that writes log messages to a file.
func newLogSink(writer io.Writer, batchSize int, flushInterval time.Duration, internalStates chan string) *LogSink {
	w := &LogSink{
		internalStates: internalStates,

		writer: writer,

		batchSize:     batchSize,
		flushInterval: flushInterval,

		in:  make(chan loggo.Entry, batchSize),
		out: make(chan []logRecord, batchSize),

		pool: sync.Pool{
			New: func() any {
				return logRecord{}
			},
		},
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
	case w.in <- entry:
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

func (w *LogSink) loop() error {
	w.tomb.Go(func() error {
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
	entries := make([]loggo.Entry, 0, w.batchSize)

	// Tick-toc the in and out channels, to ensure that we can send the batch
	// of log messages to the underlying writer.
	in := w.in
	var out chan []logRecord

	for {
		select {
		case <-w.tomb.Dying():
			if len(entries) > 0 {
				// To prevent a race, we need our own buffer and encoder to
				// write the remaining log messages that weren't written yet.
				buffer := new(bytes.Buffer)
				encoder := json.NewEncoder(buffer)
				if err := w.write(buffer, encoder, w.records(entries)); err != nil {
					return err
				}
			}
			return tomb.ErrDying

		case entry := <-in:
			// Consume log entries until we have a full batch.
			entries = append(entries, entry)
			if len(entries) < w.batchSize {
				continue
			}

			in = nil
			out = w.out

			timer.Reset(w.flushInterval)

		case <-timer.C:
			if len(entries) == 0 {
				continue
			}

			in = nil
			out = w.out

			timer.Reset(w.flushInterval)

			w.reportInternalState(stateTicked)

		case out <- w.records(entries):
			in = w.in
			out = nil
			entries = entries[:0]
		}
	}
}

func (w *LogSink) records(entries []loggo.Entry) []logRecord {
	if len(entries) == 0 {
		return nil
	}

	records := make([]logRecord, len(entries))
	for i, entry := range entries {
		var location string
		if entry.Filename != "" {
			location = fmt.Sprintf("%s:%d", entry.Filename, entry.Line)
		}

		records[i] = w.pool.Get().(logRecord)
		records[i].Time = entry.Timestamp
		records[i].Module = entry.Module
		records[i].Location = location
		records[i].Level = entry.Level
		records[i].Message = entry.Message

		if entry.Labels == nil {
			continue
		}

		records[i].Labels = entry.Labels
		records[i].ModelUUID = entry.Labels["model-uuid"]
	}
	return records
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

	for i := range records {
		// Remove any information from the pooled logRecord, this
		// mimics a fresh record.
		records[i].Time = zeroTime
		records[i].Module = ""
		records[i].Location = ""
		records[i].Level = loggo.Level(0)
		records[i].Message = ""
		records[i].Labels = nil
		records[i].ModelUUID = ""

		w.pool.Put(records[i])
	}

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
	Location  string            `json:"location,omitempty"`
	Level     loggo.Level       `json:"level"`
	Message   string            `json:"message"`
	Labels    map[string]string `json:"labels,omitempty"`
	ModelUUID string            `json:"model-uuid,omitempty"`
}
