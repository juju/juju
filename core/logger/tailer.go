// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/hpcloud/tail"
	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	"github.com/vallerion/rscanner"
	"gopkg.in/tomb.v2"
)

var logger = loggo.GetLogger("logger.tailer")

// LogTailer allows for retrieval of Juju's logs.
// It first returns any matching already recorded logs and
// then waits for additional matching logs as they appear.
type LogTailer interface {
	// Logs returns the channel through which the LogTailer returns Juju logs.
	// It will be closed when the tailer stops.
	Logs() <-chan *LogRecord

	// Dying returns a channel which will be closed as the LogTailer stops.
	Dying() <-chan struct{}

	// Stop is used to request that the LogTailer stops.
	// It blocks until the LogTailer has stopped.
	Stop() error

	// Err returns the error that caused the LogTailer to stopped.
	// If it hasn't stopped or stopped without error nil will be returned.
	Err() error
}

// LogTailerParams specifies the filtering a LogTailer should
// apply to log records in order to decide which to return.
type LogTailerParams struct {
	// TODO(debug-log) - remove StartID
	StartID       int64
	StartTime     time.Time
	MinLevel      loggo.Level
	InitialLines  int
	NoTail        bool
	IncludeEntity []string
	ExcludeEntity []string
	IncludeModule []string
	ExcludeModule []string
	IncludeLabel  []string
	ExcludeLabel  []string
}

// maxInitialLines limits the number of documents we will load into memory
// so that we can iterate them in the correct order.
var maxInitialLines = 10000

// NewLogTailer returns a LogTailer which returns lines from a
// log file and filters according to the parameters given.
func NewLogTailer(
	modelUUID, logFile string, params LogTailerParams,
) (LogTailer, error) {
	t := &logTailer{
		modelUUID:       modelUUID,
		params:          params,
		logCh:           make(chan *LogRecord),
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
	logCh           chan *LogRecord
	lastTime        time.Time
	maxInitialLines int

	logFile string
}

// Logs implements the LogTailer interface.
func (t *logTailer) Logs() <-chan *LogRecord {
	return t.logCh
}

// Dying implements the LogTailer interface.
func (t *logTailer) Dying() <-chan struct{} {
	return t.tomb.Dying()
}

// Stop implements the LogTailer interface.
func (t *logTailer) Stop() error {
	t.tomb.Kill(nil)
	return t.tomb.Wait()
}

// Err implements the LogTailer interface.
func (t *logTailer) Err() error {
	return t.tomb.Err()
}

func (t *logTailer) loop() error {
	// NOTE: don't trace or annotate the errors returned
	// from this method as the error may be tomb.ErrDying, and
	// the tomb code is sensitive about equality.

	var seekTo *tail.SeekInfo
	if t.params.InitialLines > 0 {
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

	queue := make([]*LogRecord, t.params.InitialLines)
	cur := t.params.InitialLines

	deserialisationFailures := 0
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			continue
		}
		rec, err := logLineToRecord(t.modelUUID, line)
		if err != nil {
			if deserialisationFailures == 0 {
				logger.Warningf("log deserialization failed, %v", err)
			}
			deserialisationFailures++
			continue
		} else {
			if deserialisationFailures > 1 {
				logger.Debugf("total of %d log serialisation errors", deserialisationFailures)
			}
			deserialisationFailures = 0
		}
		if !t.includeRecord(rec) {
			continue
		}
		select {
		case <-t.tomb.Dying():
			return -1, errors.Trace(tomb.ErrDying)
		default:
		}
		cur--
		queue[cur] = rec
		if cur == 0 {
			break
		}
	}
	if deserialisationFailures > 1 {
		logger.Debugf("total of %d log serialisation errors", deserialisationFailures)
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

type loggerAdaptor struct{}

func (l loggerAdaptor) Fatal(args ...interface{}) {
	logger.Criticalf(fmt.Sprint(args))
}

func (l loggerAdaptor) Fatalf(msg string, args ...interface{}) {
	logger.Criticalf(msg, args)
}

func (l loggerAdaptor) Fatalln(args ...interface{}) {
	logger.Criticalf(fmt.Sprint(args))
}

func (l loggerAdaptor) Panic(args ...interface{}) {
	logger.Criticalf(fmt.Sprint(args))
}

func (l loggerAdaptor) Panicf(msg string, args ...interface{}) {
	logger.Criticalf(msg, args)
}

func (l loggerAdaptor) Panicln(args ...interface{}) {
	logger.Criticalf(fmt.Sprint(args))
}

func (l loggerAdaptor) Print(args ...interface{}) {
	logger.Infof(fmt.Sprint(args))
}

func (l loggerAdaptor) Printf(msg string, args ...interface{}) {
	logger.Infof(msg, args)
}

func (l loggerAdaptor) Println(args ...interface{}) {
	logger.Infof(fmt.Sprint(args))
}

func (t *logTailer) tailFile(seekTo *tail.SeekInfo) (err error) {
	follow := !t.params.NoTail
	tailer, err := tail.TailFile(t.logFile, tail.Config{
		Location: seekTo,
		ReOpen:   follow,
		Poll:     true,
		Follow:   follow,
		Logger:   loggerAdaptor{},
	})
	if err != nil {
		return errors.Annotate(err, "running file tailer")
	}
	defer func() {
		_ = tailer.Killf("parent log tailer dying")
	}()

	// If we get a deserialisation error, write out the first failure,
	// but don't write out any additional errors until we either hit
	// a good value, or end the method.
	deserialisationFailures := 0
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
				if deserialisationFailures == 0 {
					logger.Warningf("log deserialization failed, %v", err)
				}
				deserialisationFailures++
				continue
			} else {
				if deserialisationFailures > 1 {
					logger.Debugf("total of %d log serialisation errors", deserialisationFailures)
				}
				deserialisationFailures = 0
			}
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

func (t *logTailer) includeRecord(rec *LogRecord) bool {
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
	// TODO(debug-log) - implement label filtering when json formatting is added
	if len(t.params.IncludeLabel) > 0 {
	}
	if len(t.params.ExcludeLabel) > 0 {
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

func logLineToRecord(modelUUID string, line string) (*LogRecord, error) {
	parts := strings.SplitN(line, " ", 7)
	if len(parts) < 7 {
		return nil, errors.Errorf("invalid log line %q", line)
	}
	level, ok := loggo.ParseLevel(parts[3])
	if !ok {
		return nil, errors.Errorf("unrecognized log level %q", parts[3])
	}
	timeStr := parts[1] + " " + parts[2]
	timeStamp, err := time.Parse("2006-01-02 15:04:05", timeStr)
	if err != nil {
		return nil, errors.Annotatef(err, "invalid log timestamp %q", timeStr)
	}

	rec := &LogRecord{
		Time: timeStamp.UTC(), // not worth preserving TZ

		ModelUUID: modelUUID,
		Entity:    parts[0],

		Level:    level,
		Module:   parts[4],
		Location: parts[5],
		Message:  parts[6],
		// TODO(debug-log) - add labels when log lines are json formatted
		//Labels:   doc.Labels,
	}
	return rec, nil
}
