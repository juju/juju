// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logtailer

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/hpcloud/tail"
	"github.com/juju/errors"
	"github.com/vallerion/rscanner"
	"gopkg.in/tomb.v2"

	corelogger "github.com/juju/juju/core/logger"
	internallogger "github.com/juju/juju/internal/logger"
)

var logger = internallogger.GetLogger("logger.tailer")

// LogTailer allows for retrieval of Juju's logs.
// It first returns any matching already recorded logs and
// then waits for additional matching logs as they appear.
type LogTailer interface {
	// Logs returns the channel through which the LogTailer returns Juju logs.
	// It will be closed when the tailer stops.
	Logs() <-chan corelogger.LogRecord

	// Dying returns a channel which will be closed as the LogTailer stops.
	Dying() <-chan struct{}

	// Kill implements worker.Kill.
	Kill()

	// Wait implements worker.Wait.
	Wait() error
}

// LogTailerParams specifies the filtering a LogTailer should
// apply to log records in order to decide which to return.
type LogTailerParams struct {
	StartTime     time.Time
	MinLevel      corelogger.Level
	InitialLines  int
	Firehose      bool
	NoTail        bool
	IncludeEntity []string
	ExcludeEntity []string
	IncludeModule []string
	ExcludeModule []string
	IncludeLabels map[string]string
	ExcludeLabels map[string]string
	FromTheStart  bool
}

// maxInitialLines limits the number of documents we will load into memory
// so that we can iterate them in the correct order.
var maxInitialLines = 10000

// NewLogTailer returns a LogTailer which returns lines from a
// log file and filters according to the parameters given.
func NewLogTailer(
	modelUUID string,
	logFile string, params LogTailerParams,
) (LogTailer, error) {
	t := &logTailer{
		modelUUID:       modelUUID,
		params:          params,
		logCh:           make(chan corelogger.LogRecord),
		maxInitialLines: maxInitialLines,
		logFile:         logFile,
	}
	t.tomb.Go(func() error {
		defer close(t.logCh)
		err := t.loop()
		return errors.Cause(err)
	})
	return t, nil
}

type logTailer struct {
	tomb            tomb.Tomb
	modelUUID       string
	params          LogTailerParams
	logCh           chan corelogger.LogRecord
	lastTime        time.Time
	maxInitialLines int

	logFile string
}

// Logs implements the LogTailer interface.
func (t *logTailer) Logs() <-chan corelogger.LogRecord {
	return t.logCh
}

// Dying implements the LogTailer interface.
func (t *logTailer) Dying() <-chan struct{} {
	return t.tomb.Dying()
}

// Kill implements worker.Kill.
func (t *logTailer) Kill() {
	t.tomb.Kill(nil)
}

// Wait implements worker.Wait.
func (t *logTailer) Wait() error {
	return t.tomb.Wait()
}

func (t *logTailer) loop() error {
	var seekTo *tail.SeekInfo
	if t.params.InitialLines > 0 && !t.params.FromTheStart {
		seekOffset, err := t.processInitialLines()
		if err != nil {
			return err
		}
		seekTo = &tail.SeekInfo{
			Offset: seekOffset,
			Whence: io.SeekStart,
		}
	}
	return t.tailFile(seekTo)
}

func (t *logTailer) processInitialLines() (int64, error) {
	if t.params.InitialLines > t.maxInitialLines {
		return -1, errors.Errorf("too many lines requested (%d) maximum is %d",
			t.params.InitialLines, maxInitialLines)
	}

	f, err := os.Open(t.logFile)
	if err != nil {
		return -1, errors.Annotatef(err, "opening file %q", t.logFile)
	}
	defer func() {
		_ = f.Close()
	}()

	seekTo, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return -1, errors.Trace(err)
	}
	fs, err := f.Stat()
	if err != nil {
		return -1, errors.Trace(err)
	}
	scanner := rscanner.NewScanner(f, fs.Size())

	queue := make([]corelogger.LogRecord, t.params.InitialLines)
	cur := t.params.InitialLines

	var failures int
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			continue
		}
		rec, err := logLineToRecord(t.modelUUID, line)
		if err != nil {
			if failures == 0 {
				logger.Warningf(context.Background(), "log deserialization failed, %v", err)
			}
			failures++
			continue
		}

		if failures > 1 {
			logger.Debugf(context.Background(), "total of %d log serialisation errors", failures)
		}
		failures = 0

		if !t.includeRecord(rec) {
			continue
		}
		select {
		case <-t.tomb.Dying():
			return -1, tomb.ErrDying
		default:
		}
		cur--
		queue[cur] = rec
		if cur == 0 {
			break
		}
	}
	if failures > 1 {
		logger.Debugf(context.Background(), "total of %d log serialisation errors", failures)
	}
	if err := scanner.Err(); err != nil {
		return -1, errors.Trace(err)
	}

	// We loaded the queue in reverse order, truncate it to just the actual
	// contents, and then return them in the correct order.
	queue = queue[cur:]
	for _, rec := range queue {
		select {
		case <-t.tomb.Dying():
			return -1, tomb.ErrDying
		case t.logCh <- rec:
			t.lastTime = rec.Time
		}
	}
	return seekTo, nil
}

func (t *logTailer) tailFile(seekTo *tail.SeekInfo) (err error) {
	follow := !t.params.NoTail
	tailer, err := tail.TailFile(t.logFile, tail.Config{
		Location: seekTo,
		ReOpen:   follow,
		Follow:   follow,
		Logger:   loggerAdaptor{},
	})
	if err != nil {
		return errors.Annotate(err, "running file tailer")
	}
	defer func() {
		_ = tailer.Killf("parent logger dying")
		// The tailer will send to its Line channel without checking
		// if the tomb is dying so we need to drain the channel here
		// just to be sure the tailer can exit,
		for {
			select {
			case _, ok := <-tailer.Lines:
				if !ok {
					return
				}
			case <-tailer.Dying():
				return
			}
		}
	}()

	// If we get a deserialisation error, write out the first failure,
	// but don't write out any additional errors until we either hit
	// a good value, or end the method.
	var failures int
	for {
		select {
		case <-t.tomb.Dying():
			return tomb.ErrDying
		case line, ok := <-tailer.Lines:
			if !ok {
				return errors.Annotate(tailer.Err(), "log tailer died")
			}
			if line.Text == "" {
				continue
			}

			rec, err := logLineToRecord(t.modelUUID, line.Text)
			if err != nil {
				if failures == 0 {
					logger.Warningf(context.Background(), "log deserialization failed, %v", err)
				}
				failures++
				continue
			}

			if failures > 1 {
				logger.Debugf(context.Background(), "total of %d log serialisation errors", failures)
			}
			failures = 0

			if !t.includeRecord(rec) {
				continue
			}
			select {
			case <-t.tomb.Dying():
				return tomb.ErrDying
			case t.logCh <- rec:
			}
		}
	}
}

func (t *logTailer) includeRecord(rec corelogger.LogRecord) bool {
	// If it's not firehose we need to check the model UUID.
	if !t.params.Firehose && rec.ModelUUID != t.modelUUID {
		return false
	}
	if rec.Time.Before(t.params.StartTime) {
		return false
	}
	if rec.Level < t.params.MinLevel {
		return false
	}
	if len(t.params.IncludeEntity) > 0 {
		match, err := regexp.MatchString(makeEntityPattern(t.params.IncludeEntity), rec.Entity)
		if !match || err != nil {
			return false
		}
	}
	if len(t.params.ExcludeEntity) > 0 {
		match, err := regexp.MatchString(makeEntityPattern(t.params.ExcludeEntity), rec.Entity)
		if match || err != nil {
			return false
		}
	}
	if len(t.params.IncludeModule) > 0 {
		match, err := regexp.MatchString(makeModulePattern(t.params.IncludeModule), rec.Module)
		if !match || err != nil {
			return false
		}
	}
	if len(t.params.ExcludeModule) > 0 {
		match, err := regexp.MatchString(makeModulePattern(t.params.ExcludeModule), rec.Module)
		if match || err != nil {
			return false
		}
	}
	if len(t.params.IncludeLabels) > 0 {
		anyMatch := false
		for k, v := range t.params.IncludeLabels {
			if val, ok := rec.Labels[k]; ok && v == val {
				anyMatch = true
				break
			}
		}
		if !anyMatch {
			return false
		}
	}
	if len(t.params.ExcludeLabels) > 0 {
		anyMatch := false
		for k, v := range t.params.IncludeLabels {
			if val, ok := rec.Labels[k]; ok && v == val {
				anyMatch = true
				break
			}
		}
		if anyMatch {
			return false
		}
	}
	return true
}

func makeEntityPattern(entities []string) string {
	var patterns []string
	for _, entity := range entities {
		// Convert * wildcard to the regex equivalent. This is safe
		// because * never appears in entity names. Escape any other regex.
		escaped := regexp.QuoteMeta(entity)
		unescapedWildcards := strings.Replace(escaped, regexp.QuoteMeta("*"), ".*", -1)
		patterns = append(patterns, unescapedWildcards)
	}
	return `^(` + strings.Join(patterns, "|") + `)$`
}

func makeModulePattern(modules []string) string {
	var patterns []string
	for _, module := range modules {
		patterns = append(patterns, regexp.QuoteMeta(module))
	}
	return `^(` + strings.Join(patterns, "|") + `)(\..+)?$`
}

func logLineToRecord(modelUUID string, line string) (corelogger.LogRecord, error) {
	var result corelogger.LogRecord
	err := json.Unmarshal([]byte(line), &result)
	return result, err
}
