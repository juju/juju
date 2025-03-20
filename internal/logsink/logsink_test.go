// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logsink

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"sort"
	"time"

	"github.com/juju/loggo/v2"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/logger"
)

type logSinkSuite struct {
	testing.IsolationSuite

	states chan string
}

var _ = gc.Suite(&logSinkSuite{})

func (s *logSinkSuite) TestWriteWithNoBatching(c *gc.C) {
	sink, buffer := s.newLogSink(c, 1)
	defer workertest.DirtyKill(c, sink)

	sink.Write(loggo.Entry{
		Level:   loggo.INFO,
		Message: "hello",
	})

	s.expectFlush(c)

	lines := parseLog(c, buffer)
	c.Assert(lines, gc.HasLen, 1)
	c.Check(lines, gc.DeepEquals, []logRecord{{
		Level:   loggo.INFO.String(),
		Message: "hello",
	}})

	workertest.CleanKill(c, sink)
}

func (s *logSinkSuite) TestWriteWithMultiline(c *gc.C) {
	sink, buffer := s.newLogSink(c, 1)
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

	lines := parseLog(c, buffer)
	c.Assert(lines, gc.HasLen, 1)
	c.Check(lines, gc.DeepEquals, []logRecord{{
		Level:   loggo.INFO.String(),
		Message: "h\n\t\t\nello\n\nwo\n\nrld\n",
	}})

	workertest.CleanKill(c, sink)
}

func (s *logSinkSuite) TestWriteWithLargeBatching(c *gc.C) {
	// This forces the ticker to flush the batch.

	sink, buffer := s.newLogSink(c, 100)
	defer workertest.DirtyKill(c, sink)

	sink.Write(loggo.Entry{
		Level:   loggo.INFO,
		Message: "hello",
	})

	s.expectTick(c)
	s.expectFlush(c)

	lines := parseLog(c, buffer)
	c.Assert(lines, gc.HasLen, 1)
	c.Check(lines, gc.DeepEquals, []logRecord{{
		Level:   loggo.INFO.String(),
		Message: "hello",
	}})

	workertest.CleanKill(c, sink)
}

func (s *logSinkSuite) TestWriteWithLogsBatching(c *gc.C) {
	// Send more than two batches of logs, but less than the batch size.
	// This will force two flushes and an additional tick and a flush.

	sink, buffer := s.newLogSink(c, 50)
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

	lines := parseLog(c, buffer)
	c.Assert(lines, gc.HasLen, total, gc.Commentf("expected %d lines, got %d", total, len(lines)))

	expected := make([]logRecord, total)
	for k, entry := range entries {
		expected[k] = logRecord{
			Time:     entry.Timestamp,
			Level:    entry.Level.String(),
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
	// This leans on the timer to send all the logs.

	sink, buffer := s.newLogSink(c, 1000)
	defer workertest.DirtyKill(c, sink)

	total := (rand.Intn(100) + 1) + 100

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
	s.expectMinNumOfFlushes(c, 1)

	lines := parseLog(c, buffer)
	c.Assert(lines, gc.HasLen, total, gc.Commentf("expected %d lines, got %d", total, len(lines)))

	expected := make([]logRecord, total)
	for k, entry := range entries {
		expected[k] = logRecord{
			Time:     entry.Timestamp,
			Level:    entry.Level.String(),
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
	// Flood the sink with logs from multiple goroutines. We don't care about
	// the order of the logs, just that they all get written. All logs will be
	// localised to the original goroutine.

	sink, buffer := s.newLogSink(c, 100)
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

	lines := parseLog(c, buffer)
	c.Assert(lines, gc.HasLen, total, gc.Commentf("expected %d lines, got %d", total, len(lines)))

	expected := make([]logRecord, total)
	for k, entry := range entries {
		expected[k] = logRecord{
			Time:     entry.Timestamp,
			Level:    entry.Level.String(),
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

/////////////////////////////////////////////////////////////////////////////////

func (s *logSinkSuite) TestLogWithNoBatching(c *gc.C) {
	sink, buffer := s.newLogSink(c, 1)
	defer workertest.DirtyKill(c, sink)

	sink.Log([]logger.LogRecord{{
		Level:   logger.INFO,
		Message: "hello",
	}})

	s.expectFlush(c)

	lines := parseLog(c, buffer)
	c.Assert(lines, gc.HasLen, 1)
	c.Check(lines, gc.DeepEquals, []logRecord{{
		Level:   loggo.INFO.String(),
		Message: "hello",
	}})

	workertest.CleanKill(c, sink)
}

func (s *logSinkSuite) TestLogWithMultiline(c *gc.C) {
	sink, buffer := s.newLogSink(c, 1)
	defer workertest.DirtyKill(c, sink)

	sink.Log([]logger.LogRecord{{
		Level: logger.INFO,
		Message: `h
		
ello

wo

rld
`}})

	s.expectFlush(c)

	lines := parseLog(c, buffer)
	c.Assert(lines, gc.HasLen, 1)
	c.Check(lines, gc.DeepEquals, []logRecord{{
		Level:   loggo.INFO.String(),
		Message: "h\n\t\t\nello\n\nwo\n\nrld\n",
	}})

	workertest.CleanKill(c, sink)
}

func (s *logSinkSuite) TestLogWithLargeBatching(c *gc.C) {
	// This forces the ticker to flush the batch.

	sink, buffer := s.newLogSink(c, 100)
	defer workertest.DirtyKill(c, sink)

	sink.Log([]logger.LogRecord{{
		Level:   logger.INFO,
		Message: "hello",
	}})

	s.expectTick(c)
	s.expectFlush(c)

	lines := parseLog(c, buffer)
	c.Assert(lines, gc.HasLen, 1)
	c.Check(lines, gc.DeepEquals, []logRecord{{
		Level:   loggo.INFO.String(),
		Message: "hello",
	}})

	workertest.CleanKill(c, sink)
}

func (s *logSinkSuite) TestLogWithLogsBatching(c *gc.C) {
	// Send more than two batches of logs, but less than the batch size.
	// This will force two flushes and an additional tick and a flush.

	sink, buffer := s.newLogSink(c, 50)
	defer workertest.DirtyKill(c, sink)

	total := (rand.Intn(48) + 1) + 100

	now := time.Now().UTC()

	entries := make([]logger.LogRecord, total)
	for i := range total {
		entries[i] = logger.LogRecord{
			Time:      now.Add(time.Duration(i) * time.Second),
			Level:     logger.INFO,
			Message:   fmt.Sprintf("hello-%d", i),
			Module:    "module",
			Location:  fmt.Sprintf("file.go:%d", i),
			ModelUUID: "uuid",
			Labels: map[string]string{
				"foo": "bar",
			},
		}
	}

	sink.Log(entries)

	// We only expect 1 flush, as batching using the Log method, doesn't break
	// the logs into smaller chunks.
	s.expectFlush(c)

	lines := parseLog(c, buffer)
	c.Assert(lines, gc.HasLen, total, gc.Commentf("expected %d lines, got %d", total, len(lines)))

	expected := make([]logRecord, total)
	for k, entry := range entries {
		expected[k] = logRecord{
			Time:     entry.Time,
			Level:    entry.Level.String(),
			Message:  entry.Message,
			Module:   entry.Module,
			Location: entry.Location,
			Labels: map[string]string{
				"foo": "bar",
			},
			ModelUUID: "uuid",
		}
	}
	c.Check(lines, gc.DeepEquals, expected)

	workertest.CleanKill(c, sink)
}

func (s *logSinkSuite) TestLogWithLogsUnderBatchSize(c *gc.C) {
	// This leans on the timer to send all the logs.

	sink, buffer := s.newLogSink(c, 1000)
	defer workertest.DirtyKill(c, sink)

	total := (rand.Intn(100) + 1) + 100

	now := time.Now().UTC()

	entries := make([]logger.LogRecord, total)
	for i := range total {
		entries[i] = logger.LogRecord{
			Time:      now.Add(time.Duration(i) * time.Second),
			Level:     logger.INFO,
			Message:   fmt.Sprintf("hello-%d", i),
			Module:    "module",
			Location:  fmt.Sprintf("file.go:%d", i),
			ModelUUID: "uuid",
			Labels: map[string]string{
				"foo": "bar",
			},
		}
	}

	sink.Log(entries)

	s.expectTick(c)
	s.expectMinNumOfFlushes(c, 1)

	lines := parseLog(c, buffer)
	c.Assert(lines, gc.HasLen, total, gc.Commentf("expected %d lines, got %d", total, len(lines)))

	expected := make([]logRecord, total)
	for k, entry := range entries {
		expected[k] = logRecord{
			Time:     entry.Time,
			Level:    entry.Level.String(),
			Message:  entry.Message,
			Module:   entry.Module,
			Location: entry.Location,
			Labels: map[string]string{
				"foo": "bar",
			},
			ModelUUID: "uuid",
		}
	}
	c.Check(lines, gc.DeepEquals, expected)

	workertest.CleanKill(c, sink)
}

func (s *logSinkSuite) TestLogLogsConcurrently(c *gc.C) {
	// Flood the sink with logs from multiple goroutines. We don't care about
	// the order of the logs, just that they all get written. All logs will be
	// localised to the original goroutine.

	sink, buffer := s.newLogSink(c, 100)
	defer workertest.DirtyKill(c, sink)

	total := 10000
	division := 100
	amount := total / division

	now := time.Now().UTC()

	entries := make([]logger.LogRecord, total)
	for i := range total {
		entries[i] = logger.LogRecord{
			Time:      now.Add(time.Duration(i) * time.Second),
			Level:     logger.INFO,
			Message:   fmt.Sprintf("hello-%d", i),
			Module:    "module",
			Location:  fmt.Sprintf("file.go:%d", i),
			ModelUUID: "uuid",
			Labels: map[string]string{
				"foo": "bar",
			},
		}
	}

	for i := range division {
		go func(i int, entries []logger.LogRecord) {
			sink.Log(entries)
		}(i, entries[i*amount:(i*amount)+amount])
	}

	// Wait for all the flushes to complete.
	s.expectNumOfFlushes(c, division)

	lines := parseLog(c, buffer)
	c.Assert(lines, gc.HasLen, total, gc.Commentf("expected %d lines, got %d", total, len(lines)))

	expected := make([]logRecord, total)
	for k, entry := range entries {
		expected[k] = logRecord{
			Time:     entry.Time,
			Level:    entry.Level.String(),
			Message:  entry.Message,
			Module:   entry.Module,
			Location: entry.Location,
			Labels: map[string]string{
				"foo": "bar",
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

func (s *logSinkSuite) TestLogAndWriteInterleaved(c *gc.C) {
	// Send more than two batches of logs, but less than the batch size.
	// This will force two flushes and an additional tick and a flush.

	sink, buffer := s.newLogSink(c, 50)
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

	for i, entry := range entries {
		if i%2 == 0 {
			sink.Write(entry)
		} else {
			sink.Log([]logger.LogRecord{{
				Time:      entry.Timestamp,
				ModelUUID: "uuid",
				Module:    entry.Module,
				Location:  fmt.Sprintf("%s:%d", entry.Filename, entry.Line),
				Level:     logger.Level(entry.Level),
				Message:   entry.Message,
				Labels:    entry.Labels,
			}})
		}
	}

	// We should see 2 flushes, and flush via the remaining entries.
	s.expectNumOfFlushes(c, 2)
	s.expectTick(c)
	s.expectFlush(c)

	lines := parseLog(c, buffer)
	c.Assert(lines, gc.HasLen, total, gc.Commentf("expected %d lines, got %d", total, len(lines)))

	expected := make([]logRecord, total)
	for k, entry := range entries {
		expected[k] = logRecord{
			Time:     entry.Timestamp,
			Level:    entry.Level.String(),
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

func (s *logSinkSuite) newLogSink(c *gc.C, batchSize int) (*LogSink, *bytes.Buffer) {
	s.states = make(chan string, 1)

	buffer := new(bytes.Buffer)

	sink := newLogSink(buffer, batchSize, time.Millisecond*100, s.states)
	return sink, buffer
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

func (s *logSinkSuite) expectMinNumOfFlushes(c *gc.C, expected int) {
	var flushes int
LOOP:
	for {
		select {
		case state := <-s.states:
			if state == stateFlushed {
				flushes++
			}
		case <-time.After(time.Second):
			break LOOP
		}
	}
	c.Assert(flushes >= expected, jc.IsTrue, gc.Commentf("expected more than 1 flush, got %d", flushes))
}

func (s *logSinkSuite) expectTick(c *gc.C) {
	select {
	case state := <-s.states:
		c.Assert(state, gc.Equals, stateTicked)
	case <-time.After(testing.ShortWait * 10):
		c.Fatalf("timed out waiting for startup")
	}
}

func parseLog(c *gc.C, reader io.Reader) []logRecord {
	var records []logRecord

	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		var record logRecord
		err := json.Unmarshal(scanner.Bytes(), &record)
		c.Assert(err, jc.ErrorIsNil)
		records = append(records, record)
	}

	return records
}
