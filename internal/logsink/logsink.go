// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/juju/loggo/v2"
	"gopkg.in/tomb.v2"
)

// LogSink is a loggo.Writer that writes log messages to a file.
type LogSink struct {
	tomb tomb.Tomb

	dir       string
	name      string
	batchSize int

	in  chan loggo.Entry
	out chan []LogRecord

	pool sync.Pool
}

// NewLogSink creates a new log sink that writes log messages to a file.
func NewLogSink(dir string, name string, batchSize int) *LogSink {
	w := &LogSink{
		dir:       dir,
		name:      name,
		batchSize: batchSize,

		in:  make(chan loggo.Entry),
		out: make(chan []LogRecord),

		pool: sync.Pool{
			New: func() any {
				return LogRecord{}
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
	// Open file for writing.
	file, err := w.openFile()
	if err != nil {
		return err
	}
	defer file.Close()

	buffer := new(bytes.Buffer)
	encoder := json.NewEncoder(buffer)

	ticker := time.NewTicker(time.Second * 30)

	entries := make([]loggo.Entry, 0, w.batchSize)

	in := w.in
	var out chan []LogRecord

	w.tomb.Go(func() error {
		for {
			select {
			case <-w.tomb.Dying():
				return tomb.ErrDying
			case records := <-w.out:
				if len(records) == 0 {
					continue
				}

				for _, record := range records {
					if err := encoder.Encode(record); err != nil {
						fmt.Fprintf(os.Stderr, "failed to encode log message: %v", err)
					}
				}

				if _, err := file.Write(buffer.Bytes()); err != nil {
					fmt.Fprintf(os.Stderr, "failed to write log message: %v", err)
				}

				buffer.Reset()

				for i := range records {
					w.pool.Put(records[i])
				}
			}
		}
	})

	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying

		case entry := <-in:
			// Consume log entries until we have a full batch.
			entries = append(entries, entry)

			// If we have a full batch, send it to the output channel and
			// force the in channel to nil to avoid reading from it. This
			// will allow the out channel to be read from. This allows us to
			// write the batch to the file and reset the entries slice.
			if len(entries) >= w.batchSize {
				in = nil
				out = w.out
			}

		case <-ticker.C:
			// If we have entries, send them to the output channel and force
			// the in channel to nil to avoid reading from it. This will allow
			// the out channel to be read from. This allows us to write the
			// batch to the file and reset the entries slice.
			if len(entries) > 0 {
				in = nil
				out = w.out
			}

		case out <- w.records(entries):
			in = w.in
			out = nil
			entries = entries[:0]
		}
	}
}

func (w *LogSink) openFile() (*os.File, error) {
	return os.OpenFile(filepath.Join(w.dir, w.name), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
}

func (w *LogSink) records(entries []loggo.Entry) []LogRecord {
	records := make([]LogRecord, len(entries))
	for i, entry := range entries {
		records[i] = w.pool.Get().(LogRecord)
		records[i].Time = entry.Timestamp
		records[i].Module = entry.Module
		records[i].Location = fmt.Sprintf("%s:%d", filepath.Base(entry.Filename), entry.Line)
		records[i].Level = entry.Level
		records[i].Message = entry.Message
		records[i].Labels = entry.Labels
	}
	return records
}

type LogRecord struct {
	Time     time.Time         `json:"time"`
	Module   string            `json:"module"`
	Location string            `json:"location"`
	Level    loggo.Level       `json:"level"`
	Message  string            `json:"message"`
	Labels   map[string]string `json:"labels"`
}
