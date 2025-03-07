// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/juju/loggo/v2"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"
)

type logSinkSuite struct {
	testing.IsolationSuite

	states chan string
}

var _ = gc.Suite(&logSinkSuite{})

func (s *logSinkSuite) TestWriteWithNoBatching(c *gc.C) {
	dir := c.MkDir()

	sink := s.newLogSink(c, 1, dir)
	defer workertest.DirtyKill(c, sink)

	sink.Write(loggo.Entry{
		Level:   loggo.INFO,
		Message: "hello",
	})

	s.expectFlush(c)

	log := s.readLog(c, dir)
	lines := parseLog(c, log)
	c.Assert(lines, gc.HasLen, 1)
	c.Check(lines, gc.DeepEquals, []LogRecord{{
		Level:   loggo.INFO,
		Message: "hello",
	}})

	workertest.CleanKill(c, sink)
}

func (s *logSinkSuite) TestWriteWithMultiline(c *gc.C) {
	dir := c.MkDir()

	sink := s.newLogSink(c, 1, dir)
	defer workertest.DirtyKill(c, sink)

	sink.Write(loggo.Entry{
		Level: loggo.INFO,
		Message: `h
		
ello

wo

rld
`,
	})

	s.expectFlush(c)

	log := s.readLog(c, dir)
	lines := parseLog(c, log)
	c.Assert(lines, gc.HasLen, 1)
	c.Check(lines, gc.DeepEquals, []LogRecord{{
		Level:   loggo.INFO,
		Message: "h\n\t\t\nello\n\nwo\n\nrld\n",
	}})

	workertest.CleanKill(c, sink)
}

func (s *logSinkSuite) TestWriteWithLargeBatching(c *gc.C) {
	dir := c.MkDir()

	// This forces the ticker to flush the batch.

	sink := s.newLogSink(c, 100, dir)
	defer workertest.DirtyKill(c, sink)

	sink.Write(loggo.Entry{
		Level:   loggo.INFO,
		Message: "hello",
	})

	s.expectTick(c)
	s.expectFlush(c)

	log := s.readLog(c, dir)
	lines := parseLog(c, log)
	c.Assert(lines, gc.HasLen, 1)
	c.Check(lines, gc.DeepEquals, []LogRecord{{
		Level:   loggo.INFO,
		Message: "hello",
	}})

	workertest.CleanKill(c, sink)
}

func (s *logSinkSuite) TestWriteWithLogsBatching(c *gc.C) {
	dir := c.MkDir()

	// Send more than two batches of logs, but less than the batch size.
	// This will force two flushes and an additional tick and a flush.

	sink := s.newLogSink(c, 50, dir)
	defer workertest.DirtyKill(c, sink)

	total := (rand.Intn(48) + 1) + 100

	now := time.Now().UTC()

	entries := make([]loggo.Entry, total)
	for i := range total {
		entries[i] = loggo.Entry{
			Timestamp: now.Add(time.Duration(i) * time.Second),
			Level:     loggo.INFO,
			Message:   fmt.Sprintf("hello-%d", i),
			Module:    "module",
			Filename:  "file.go",
			Line:      i,
			Labels: map[string]string{
				"model-uuid": "uuid",
			},
		}
	}

	for _, entry := range entries {
		sink.Write(entry)
	}

	// We should see 2 flushes, and flush via the remaining entries.
	s.expectNumOfFlushes(c, 2)
	s.expectTick(c)
	s.expectFlush(c)

	log := s.readLog(c, dir)
	lines := parseLog(c, log)
	c.Assert(lines, gc.HasLen, total, gc.Commentf("expected %d lines, got %d", total, len(lines)))

	expected := make([]LogRecord, total)
	for k, entry := range entries {
		expected[k] = LogRecord{
			Time:     entry.Timestamp,
			Level:    entry.Level,
			Message:  entry.Message,
			Module:   entry.Module,
			Location: fmt.Sprintf("%s:%d", entry.Filename, entry.Line),
			Labels: map[string]string{
				"model-uuid": "uuid",
			},
			ModelUUID: "uuid",
		}
	}
	c.Check(lines, gc.DeepEquals, expected)

	workertest.CleanKill(c, sink)
}

func (s *logSinkSuite) TestWriteWithLogsUnderBatchSize(c *gc.C) {
	dir := c.MkDir()

	// This leans on the timer to send all the logs.

	sink := s.newLogSink(c, 1000, dir)
	defer workertest.DirtyKill(c, sink)

	total := rand.Intn(100) + 100

	now := time.Now().UTC()

	entries := make([]loggo.Entry, total)
	for i := range total {
		entries[i] = loggo.Entry{
			Timestamp: now.Add(time.Duration(i) * time.Second),
			Level:     loggo.INFO,
			Message:   fmt.Sprintf("hello-%d", i),
			Module:    "module",
			Filename:  "file.go",
			Line:      i,
			Labels: map[string]string{
				"model-uuid": "uuid",
			},
		}
	}

	for _, entry := range entries {
		sink.Write(entry)
	}

	s.expectTick(c)
	s.expectFlush(c)

	log := s.readLog(c, dir)
	lines := parseLog(c, log)
	c.Assert(lines, gc.HasLen, total, gc.Commentf("expected %d lines, got %d", total, len(lines)))

	expected := make([]LogRecord, total)
	for k, entry := range entries {
		expected[k] = LogRecord{
			Time:     entry.Timestamp,
			Level:    entry.Level,
			Message:  entry.Message,
			Module:   entry.Module,
			Location: fmt.Sprintf("%s:%d", entry.Filename, entry.Line),
			Labels: map[string]string{
				"model-uuid": "uuid",
			},
			ModelUUID: "uuid",
		}
	}
	c.Check(lines, gc.DeepEquals, expected)

	workertest.CleanKill(c, sink)
}

func (s *logSinkSuite) TestWriteLogsConcurrently(c *gc.C) {
	dir := c.MkDir()

	// Flood the sink with logs from multiple goroutines. We don't care about
	// the order of the logs, just that they all get written. All logs will be
	// localised to the original goroutine.

	sink := s.newLogSink(c, 100, dir)
	defer workertest.DirtyKill(c, sink)

	total := 10000
	division := 100
	amount := total / division

	now := time.Now().UTC()

	entries := make([]loggo.Entry, total)
	for i := range total {
		entries[i] = loggo.Entry{
			Timestamp: now.Add(time.Duration(i) * time.Second),
			Level:     loggo.INFO,
			Message:   fmt.Sprintf("hello-%d", i),
			Module:    "module",
			Filename:  "file.go",
			Line:      i,
			Labels: map[string]string{
				"model-uuid": "uuid",
			},
		}
	}

	for i := range division {
		go func(i int, entries []loggo.Entry) {
			for _, entry := range entries {
				sink.Write(entry)
			}
		}(i, entries[i*amount:(i*amount)+amount])
	}

	// Wait for all the flushes to complete.
	s.expectNumOfFlushes(c, division)

	log := s.readLog(c, dir)
	lines := parseLog(c, log)
	c.Assert(lines, gc.HasLen, total, gc.Commentf("expected %d lines, got %d", total, len(lines)))

	expected := make([]LogRecord, total)
	for k, entry := range entries {
		expected[k] = LogRecord{
			Time:     entry.Timestamp,
			Level:    entry.Level,
			Message:  entry.Message,
			Module:   entry.Module,
			Location: fmt.Sprintf("%s:%d", entry.Filename, entry.Line),
			Labels: map[string]string{
				"model-uuid": "uuid",
			},
			ModelUUID: "uuid",
		}
	}

	// We can't guarantee the order of the entries written in the test, so we
	// need to sort them before comparing.
	sort.Slice(lines, func(i, j int) bool {
		return lines[i].Time.Before(lines[j].Time)
	})

	c.Check(lines, gc.DeepEquals, expected)

	workertest.CleanKill(c, sink)
}

func (s *logSinkSuite) newLogSink(c *gc.C, batchSize int, dir string) *LogSink {
	s.states = make(chan string, 1)

	sink := newLogSink(dir, "logsink.log", batchSize, time.Millisecond*100, s.states)
	return sink
}

func (s *logSinkSuite) expectFlush(c *gc.C) {
	select {
	case state := <-s.states:
		c.Assert(state, gc.Equals, stateFlushed)
	case <-time.After(testing.ShortWait * 10):
		c.Fatalf("timed out waiting for startup")
	}
}

func (s *logSinkSuite) expectNumOfFlushes(c *gc.C, flushes int) {
	for {
		select {
		case state := <-s.states:
			if state == stateFlushed {
				flushes--
				if flushes == 0 {
					return
				}
			}
		case <-time.After(testing.LongWait):
			c.Fatalf("timed out waiting for %d flushes", flushes)
		}
	}
}

func (s *logSinkSuite) expectTick(c *gc.C) {
	select {
	case state := <-s.states:
		c.Assert(state, gc.Equals, stateTicked)
	case <-time.After(testing.ShortWait * 10):
		c.Fatalf("timed out waiting for startup")
	}
}

func (s *logSinkSuite) readLog(c *gc.C, dir string) string {
	logFile := filepath.Join(dir, "logsink.log")
	content, err := os.ReadFile(logFile)
	c.Assert(err, gc.IsNil)
	return string(content)
}

func parseLog(c *gc.C, log string) []LogRecord {
	var records []LogRecord

	scanner := bufio.NewScanner(bytes.NewBufferString(log))
	for scanner.Scan() {
		var record LogRecord
		err := json.Unmarshal(scanner.Bytes(), &record)
		c.Assert(err, jc.ErrorIsNil)
		records = append(records, record)
	}

	return records
}
