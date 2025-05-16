// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logtailer_test

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	stdtesting "testing"
	"time"

	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"

	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/logtailer"
	"github.com/juju/juju/internal/testhelpers"
	coretesting "github.com/juju/juju/internal/testing"
)

type TailerSuite struct {
	testhelpers.IsolationSuite
}

func TestTailerSuite(t *stdtesting.T) { tc.Run(t, &TailerSuite{}) }
func (s *TailerSuite) TestProcessForwardNoTail(c *tc.C) {
	testFileName := filepath.Join(c.MkDir(), "test.log")
	err := os.WriteFile(testFileName, []byte(createLogFileContent(c)), 0644)
	c.Assert(err, tc.ErrorIsNil)

	tailer, err := logtailer.NewLogTailer(coretesting.ModelTag.Id(), testFileName, logtailer.LogTailerParams{
		NoTail: true,
	})
	c.Assert(err, tc.ErrorIsNil)

	var records []corelogger.LogRecord
	logs := tailer.Logs()
	for {
		rec, ok := <-logs
		if !ok {
			break
		}
		records = append(records, rec)
	}
	c.Assert(records, tc.DeepEquals, logRecords)
}

func (s *TailerSuite) TestWithModelUUID(c *tc.C) {
	testFileName := filepath.Join(c.MkDir(), "test.log")
	err := os.WriteFile(testFileName, []byte(createLogFileContent(c)), 0644)
	c.Assert(err, tc.ErrorIsNil)

	tailer, err := logtailer.NewLogTailer("", testFileName, logtailer.LogTailerParams{
		NoTail:   true,
		Firehose: true,
	})
	c.Assert(err, tc.ErrorIsNil)

	var records []corelogger.LogRecord
	logs := tailer.Logs()
	for {
		rec, ok := <-logs
		if !ok {
			break
		}
		records = append(records, rec)
	}
	recordsWithModel := logRecords[:]
	for i, r := range recordsWithModel {
		r.ModelUUID = fmt.Sprintf("modelUUID%d", i+1)
	}
	c.Assert(records, tc.DeepEquals, recordsWithModel)
}

func (s *TailerSuite) TestProcessReverseNoTail(c *tc.C) {
	testFileName := filepath.Join(c.MkDir(), "test.log")
	err := os.WriteFile(testFileName, []byte(createLogFileContent(c)), 0644)
	c.Assert(err, tc.ErrorIsNil)

	tailer, err := logtailer.NewLogTailer(coretesting.ModelTag.Id(), testFileName, logtailer.LogTailerParams{
		NoTail:       true,
		InitialLines: 2,
	})
	c.Assert(err, tc.ErrorIsNil)

	var records []corelogger.LogRecord
	logs := tailer.Logs()
	for {
		rec, ok := <-logs
		if !ok {
			break
		}
		records = append(records, rec)
	}
	c.Assert(records, tc.DeepEquals, logRecords[2:])
}

func (s *TailerSuite) fetchLogs(tailer logtailer.LogTailer, expected int) []corelogger.LogRecord {
	var records []corelogger.LogRecord
	timeout := time.After(testhelpers.LongWait)
	for {
		select {
		case rec, ok := <-tailer.Logs():
			if !ok {
				return records
			}
			records = append(records, rec)
			if len(records) == expected {
				return records
			}
		case <-timeout:
			return records
		}
	}
}

func (s *TailerSuite) writeAdditionalLogs(c *tc.C, fileName string, lines []string) {
	go func() {
		f, err := os.OpenFile(fileName, os.O_APPEND|os.O_WRONLY, 0644)
		c.Assert(err, tc.ErrorIsNil)
		defer func() {
			_ = f.Close()
		}()

		_, _ = fmt.Fprintln(f, "")
		for _, l := range lines {
			if l == "" {
				continue
			}
			_, _ = fmt.Fprintln(f, l)
		}
	}()
}

func (s *TailerSuite) TestProcessForwardTail(c *tc.C) {
	logLines := strings.Split(createLogFileContent(c), "\n")
	testFileName := filepath.Join(c.MkDir(), "test.log")
	f, err := os.OpenFile(testFileName, os.O_CREATE|os.O_WRONLY, 0644)
	c.Assert(err, tc.ErrorIsNil)
	defer func() {
		_ = f.Close()
	}()
	_, err = io.WriteString(f, strings.Join(logLines[:2], "\n"))
	c.Assert(err, tc.ErrorIsNil)

	tailer, err := logtailer.NewLogTailer(coretesting.ModelTag.Id(), testFileName, logtailer.LogTailerParams{})
	c.Assert(err, tc.ErrorIsNil)

	// Allow logtailer to start up.
	time.Sleep(100 * time.Millisecond)

	s.writeAdditionalLogs(c, testFileName, logLines[2:])

	records := s.fetchLogs(tailer, 4)
	c.Assert(records, tc.DeepEquals, logRecords)
}

func (s *TailerSuite) TestProcessReverseTail(c *tc.C) {
	logLines := strings.Split(createLogFileContent(c), "\n")
	testFileName := filepath.Join(c.MkDir(), "test.log")
	f, err := os.OpenFile(testFileName, os.O_CREATE|os.O_WRONLY, 0644)
	c.Assert(err, tc.ErrorIsNil)

	_, err = io.WriteString(f, strings.Join(logLines[:3], "\n"))
	c.Assert(f.Close(), tc.ErrorIsNil)
	c.Assert(err, tc.ErrorIsNil)

	tailer, err := logtailer.NewLogTailer(coretesting.ModelTag.Id(), testFileName, logtailer.LogTailerParams{
		InitialLines: 2,
	})
	c.Assert(err, tc.ErrorIsNil)
	records := s.fetchLogs(tailer, 2)

	// Allow logtailer to start up.
	time.Sleep(100 * time.Millisecond)

	s.writeAdditionalLogs(c, testFileName, logLines[3:])

	newRecords := s.fetchLogs(tailer, 1)
	result := append(records, newRecords...)
	c.Assert(result, tc.DeepEquals, logRecords[1:])
}

func createLogFileContent(c *tc.C) string {
	buffer := new(strings.Builder)

	jsonEncoder := json.NewEncoder(buffer)
	for _, record := range logRecords {
		err := jsonEncoder.Encode(record)
		c.Assert(err, tc.ErrorIsNil)
	}

	return buffer.String()
}

var logRecords = []corelogger.LogRecord{
	{
		Time:      mustParseTime("2024-02-15 06:23:22"),
		ModelUUID: coretesting.ModelTag.Id(),
		Entity:    "machine-0",
		Level:     corelogger.DEBUG,
		Module:    "juju.worker.dependency",
		Location:  "engine.go:598",
		Message:   `"db-accessor" manifold worker started at 2024-02-15 06:23:23.006402802 +0000 UTC`,
	},
	{
		Time:      mustParseTime("2024-02-15 06:23:23"),
		ModelUUID: coretesting.ModelTag.Id(),
		Entity:    "machine-0",
		Level:     corelogger.INFO,
		Module:    "juju.worker.dbaccessor",
		Location:  "worker.go:518",
		Message:   "host is configured to use cloud-local address as a Dqlite node",
	},
	{
		Time:      mustParseTime("2024-02-15 06:23:24"),
		ModelUUID: coretesting.ModelTag.Id(),
		Entity:    "machine-1",
		Level:     corelogger.WARNING,
		Module:    "juju.worker.dependency",
		Location:  "engine.go:598",
		Message:   `"lease-manager" manifold worker started at 2024-02-15 06:23:23.016373586 +0000 UTC`,
	},
	{
		Time:      mustParseTime("2024-02-15 06:23:25"),
		ModelUUID: coretesting.ModelTag.Id(),
		Entity:    "machine-0",
		Level:     corelogger.CRITICAL,
		Module:    "juju.worker.dependency",
		Location:  "engine.go:598",
		Message:   `"change-stream" manifold worker started at 2024-02-15 06:23:23.01677874 +0000 UTC`,
	},
}

func mustParseTime(in string) time.Time {
	out, err := time.Parse("2006-01-02 15:04:05", in)
	if err != nil {
		panic(err)
	}
	return out
}

type LogFilterSuite struct {
	testhelpers.IsolationSuite
}

func TestLogFilterSuite(t *stdtesting.T) { tc.Run(t, &LogFilterSuite{}) }
func (s *LogFilterSuite) TestLevelFiltering(c *tc.C) {
	infoLevelRec := &corelogger.LogRecord{Level: corelogger.INFO}
	errorLevelRec := &corelogger.LogRecord{Level: corelogger.ERROR}
	logFile := filepath.Join(c.MkDir(), "logs.log")
	writeLogs := func() string {
		s.writeLogs(c, logFile, 1, &corelogger.LogRecord{Level: corelogger.DEBUG})
		s.writeLogs(c, logFile, 1, infoLevelRec)
		s.writeLogs(c, logFile, 1, errorLevelRec)
		return logFile
	}
	params := logtailer.LogTailerParams{
		MinLevel: corelogger.INFO,
	}
	assert := func(tailer logtailer.LogTailer) {
		s.assertTailer(c, tailer, infoLevelRec, errorLevelRec)
	}
	s.checkLogTailerFiltering(c, params, writeLogs, assert)
}

func (s *LogFilterSuite) TestIncludeEntity(c *tc.C) {
	machine0 := &corelogger.LogRecord{Entity: "machine-0"}
	foo0 := &corelogger.LogRecord{Entity: "unit-foo-0"}
	foo1 := &corelogger.LogRecord{Entity: "unit-foo-1"}
	logFile := filepath.Join(c.MkDir(), "logs.log")
	writeLogs := func() string {
		s.writeLogs(c, logFile, 3, machine0)
		s.writeLogs(c, logFile, 2, foo0)
		s.writeLogs(c, logFile, 1, foo1)
		s.writeLogs(c, logFile, 3, machine0)
		return logFile
	}
	params := logtailer.LogTailerParams{
		IncludeEntity: []string{
			"unit-foo-0",
			"unit-foo-1",
		},
	}
	assert := func(tailer logtailer.LogTailer) {
		s.assertTailer(c, tailer, foo0, foo0, foo1)
	}
	s.checkLogTailerFiltering(c, params, writeLogs, assert)
}

func (s *LogFilterSuite) TestIncludeEntityWildcard(c *tc.C) {
	machine0 := &corelogger.LogRecord{Entity: "machine-0"}
	foo0 := &corelogger.LogRecord{Entity: "unit-foo-0"}
	foo1 := &corelogger.LogRecord{Entity: "unit-foo-1"}
	logFile := filepath.Join(c.MkDir(), "logs.log")
	writeLogs := func() string {
		s.writeLogs(c, logFile, 3, machine0)
		s.writeLogs(c, logFile, 2, foo0)
		s.writeLogs(c, logFile, 1, foo1)
		s.writeLogs(c, logFile, 3, machine0)
		return logFile
	}
	params := logtailer.LogTailerParams{
		IncludeEntity: []string{
			"unit-foo*",
		},
	}
	assert := func(tailer logtailer.LogTailer) {
		s.assertTailer(c, tailer, foo0, foo0, foo1)
	}
	s.checkLogTailerFiltering(c, params, writeLogs, assert)
}

func (s *LogFilterSuite) TestExcludeEntity(c *tc.C) {
	machine0 := &corelogger.LogRecord{Entity: "machine-0"}
	foo0 := &corelogger.LogRecord{Entity: "unit-foo-0"}
	foo1 := &corelogger.LogRecord{Entity: "unit-foo-1"}
	logFile := filepath.Join(c.MkDir(), "logs.log")
	writeLogs := func() string {
		s.writeLogs(c, logFile, 3, machine0)
		s.writeLogs(c, logFile, 2, foo0)
		s.writeLogs(c, logFile, 1, foo1)
		s.writeLogs(c, logFile, 3, machine0)
		return logFile
	}
	params := logtailer.LogTailerParams{
		ExcludeEntity: []string{
			"machine-0",
			"unit-foo-0",
		},
	}
	assert := func(tailer logtailer.LogTailer) {
		s.assertTailer(c, tailer, foo1)
	}
	s.checkLogTailerFiltering(c, params, writeLogs, assert)
}

func (s *LogFilterSuite) TestExcludeEntityWildcard(c *tc.C) {
	machine0 := &corelogger.LogRecord{Entity: "machine-0"}
	foo0 := &corelogger.LogRecord{Entity: "unit-foo-0"}
	foo1 := &corelogger.LogRecord{Entity: "unit-foo-1"}
	logFile := filepath.Join(c.MkDir(), "logs.log")
	writeLogs := func() string {
		s.writeLogs(c, logFile, 3, machine0)
		s.writeLogs(c, logFile, 2, foo0)
		s.writeLogs(c, logFile, 1, foo1)
		s.writeLogs(c, logFile, 3, machine0)
		return logFile
	}
	params := logtailer.LogTailerParams{
		ExcludeEntity: []string{
			"machine*",
			"unit-*-0",
		},
	}
	assert := func(tailer logtailer.LogTailer) {
		s.assertTailer(c, tailer, foo1)
	}
	s.checkLogTailerFiltering(c, params, writeLogs, assert)
}

func (s *LogFilterSuite) TestIncludeModule(c *tc.C) {
	mod0 := &corelogger.LogRecord{Module: "foo.bar"}
	mod1 := &corelogger.LogRecord{Module: "juju.thing"}
	subMod1 := &corelogger.LogRecord{Module: "juju.thing.hai"}
	mod2 := &corelogger.LogRecord{Module: "elsewhere"}
	logFile := filepath.Join(c.MkDir(), "logs.log")
	writeLogs := func() string {
		s.writeLogs(c, logFile, 1, mod0)
		s.writeLogs(c, logFile, 1, mod1)
		s.writeLogs(c, logFile, 1, mod0)
		s.writeLogs(c, logFile, 1, subMod1)
		s.writeLogs(c, logFile, 1, mod0)
		s.writeLogs(c, logFile, 1, mod2)
		return logFile
	}
	params := logtailer.LogTailerParams{
		IncludeModule: []string{"juju.thing", "elsewhere"},
	}
	assert := func(tailer logtailer.LogTailer) {
		s.assertTailer(c, tailer, mod1, subMod1, mod2)
	}
	s.checkLogTailerFiltering(c, params, writeLogs, assert)
}

func (s *LogFilterSuite) TestExcludeModule(c *tc.C) {
	mod0 := &corelogger.LogRecord{Module: "foo.bar"}
	mod1 := &corelogger.LogRecord{Module: "juju.thing"}
	subMod1 := &corelogger.LogRecord{Module: "juju.thing.hai"}
	mod2 := &corelogger.LogRecord{Module: "elsewhere"}
	logFile := filepath.Join(c.MkDir(), "logs.log")
	writeLogs := func() string {
		s.writeLogs(c, logFile, 1, mod0)
		s.writeLogs(c, logFile, 1, mod1)
		s.writeLogs(c, logFile, 1, mod0)
		s.writeLogs(c, logFile, 1, subMod1)
		s.writeLogs(c, logFile, 1, mod0)
		s.writeLogs(c, logFile, 1, mod2)
		return logFile
	}
	params := logtailer.LogTailerParams{
		ExcludeModule: []string{"juju.thing", "elsewhere"},
	}
	assert := func(tailer logtailer.LogTailer) {
		s.assertTailer(c, tailer, mod0, mod0)
	}
	s.checkLogTailerFiltering(c, params, writeLogs, assert)
}

func (s *LogFilterSuite) TestIncludeExcludeModule(c *tc.C) {
	foo := &corelogger.LogRecord{Module: "foo"}
	bar := &corelogger.LogRecord{Module: "bar"}
	barSub := &corelogger.LogRecord{Module: "bar.thing"}
	baz := &corelogger.LogRecord{Module: "baz"}
	qux := &corelogger.LogRecord{Module: "qux"}
	logFile := filepath.Join(c.MkDir(), "logs.log")
	writeLogs := func() string {
		s.writeLogs(c, logFile, 1, foo)
		s.writeLogs(c, logFile, 1, bar)
		s.writeLogs(c, logFile, 1, barSub)
		s.writeLogs(c, logFile, 1, baz)
		s.writeLogs(c, logFile, 1, qux)
		return logFile
	}
	params := logtailer.LogTailerParams{
		IncludeModule: []string{"foo", "bar", "qux"},
		ExcludeModule: []string{"foo", "bar"},
	}
	assert := func(tailer logtailer.LogTailer) {
		// Except just "qux" because "foo" and "bar" were included and
		// then excluded.
		s.assertTailer(c, tailer, qux)
	}
	s.checkLogTailerFiltering(c, params, writeLogs, assert)
}

func (s *LogFilterSuite) checkLogTailerFiltering(
	c *tc.C,
	params logtailer.LogTailerParams,
	writeLogs func() string,
	assertTailer func(logtailer.LogTailer),
) {
	logFile := writeLogs()
	tailer, err := logtailer.NewLogTailer(coretesting.ModelTag.Id(), logFile, params)
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.CleanKill(c, tailer)
	assertTailer(tailer)
}

func (s *LogFilterSuite) assertTailer(c *tc.C, tailer logtailer.LogTailer, template ...*corelogger.LogRecord) {
	c.Assert(template, tc.Not(tc.HasLen), 0)
	timeout := time.After(testhelpers.LongWait)
	count := 0
	for {
		select {
		case log, ok := <-tailer.Logs():
			if !ok {
				c.Fatalf("tailer died unexpectedly: %v", tailer.Wait())
			}
			rec := s.normaliseLogTemplate(template[0])
			template = template[1:]

			c.Assert(log.Entity, tc.Equals, rec.Entity)
			c.Assert(log.Module, tc.Equals, rec.Module)
			c.Assert(log.Location, tc.Equals, rec.Location)
			c.Assert(log.Level, tc.Equals, rec.Level)
			c.Assert(log.Message, tc.Equals, rec.Message)
			c.Assert(log.Labels, tc.DeepEquals, rec.Labels)
			count++
			if len(template) == 0 {
				return
			}
		case <-timeout:
			c.Fatalf("timed out waiting for logs (received %d)", count)
		}
	}
}

func (s *LogFilterSuite) normaliseLogTemplate(template *corelogger.LogRecord) *corelogger.LogRecord {
	rec := *template
	if rec.Entity == "" {
		rec.Entity = "not-a-tag"
	}
	if rec.Module == "" {
		rec.Module = "module"
	}
	if rec.Location == "" {
		rec.Location = "loc"
	}
	if rec.Level == corelogger.UNSPECIFIED {
		rec.Level = corelogger.INFO
	}
	if rec.Message == "" {
		rec.Message = "message"
	}
	if rec.ModelUUID == "" {
		rec.ModelUUID = coretesting.ModelTag.Id()
	}
	return &rec
}

// writeLogs creates count log messages at the current time using
// the supplied template.
func (s *LogFilterSuite) writeLogs(c *tc.C, logFie string, count int, template *corelogger.LogRecord) {
	t := coretesting.ZeroTime()
	s.writeLogsT(c, logFie, t, t, count, template)
}

// writeLogsT creates count log messages between startTime and
// endTime using the supplied template
func (s *LogFilterSuite) writeLogsT(c *tc.C, logFile string, startTime, endTime time.Time, count int, template *corelogger.LogRecord) {
	f, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	c.Assert(err, tc.ErrorIsNil)
	defer func() {
		_ = f.Close()
	}()

	interval := endTime.Sub(startTime) / time.Duration(count)
	t := startTime

	buffer := new(strings.Builder)
	jsonEncoder := json.NewEncoder(buffer)
	for range count {
		rec := s.normaliseLogTemplate(template)
		err := jsonEncoder.Encode(rec)
		c.Assert(err, tc.ErrorIsNil)

		t = t.Add(interval)
	}

	_, err = io.WriteString(f, buffer.String())
	c.Assert(err, tc.ErrorIsNil)
}
