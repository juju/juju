// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"gopkg.in/tomb.v2"
)

const (
	filename = "slow-query.log"

	PollInterval = time.Second
)

// Logger is the interface that is used to log issues with the slow query
// logger.
type Logger interface {
	Warningf(string, ...interface{})
}

// SlowQueryLogger is a logger that can be used to log slow operations at the
// database level. It will print out debug messages for slow queries.
type SlowQueryLogger struct {
	tomb tomb.Tomb

	clock  clock.Clock
	logger Logger

	logDir string
	logs   chan payload
}

// NewSlowQueryLogger creates a new SlowQueryLogger, which can be used to log
// slow queries.
func NewSlowQueryLogger(logDir string, clock clock.Clock, logger Logger) *SlowQueryLogger {
	l := &SlowQueryLogger{
		logDir: logDir,
		clock:  clock,
		logs:   make(chan payload),
	}
	l.tomb.Go(l.loop)
	return l
}

// Log the slow query, with the given arguments.
func (l *SlowQueryLogger) Log(msg string, duration float64, stmt string, stack []byte) error {
	done := make(chan error)
	select {
	case l.logs <- payload{
		log: log{
			msg:      msg,
			duration: duration,
			stmt:     stmt,
			stack:    stack,
		},
		done: done,
	}:
	case <-l.tomb.Dying():
		return tomb.ErrDying
	}

	select {
	case err := <-done:
		return errors.Trace(err)
	case <-l.tomb.Dying():
		return tomb.ErrDying
	}
}

// Close the logger.
func (l *SlowQueryLogger) Close() error {
	l.tomb.Kill(nil)
	return l.tomb.Wait()
}

func (l *SlowQueryLogger) loop() error {
	// Open the log file.
	file, err := os.OpenFile(filepath.Join(l.logDir, filename), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return errors.Trace(err)
	}
	// Ensure we sync and then close it when we're done, so we don't loose any
	// potential writes.
	defer func() {
		// Don't return early, just log and continue.
		if err := file.Sync(); err != nil {
			l.logger.Warningf("failed to sync slow query log: %v", err)
		}
		if err := file.Close(); err != nil {
			l.logger.Warningf("failed to close slow query log: %v", err)
		}
	}()

	timer := l.clock.NewTimer(PollInterval)
	defer timer.Stop()

	var syncRequired bool
	for {
		select {
		case <-l.tomb.Dying():
			return tomb.ErrDying

		case payload := <-l.logs:
			_, err := file.WriteString(payload.log.String())
			select {
			case payload.done <- err:
				syncRequired = err == nil
			case <-l.tomb.Dying():
				return tomb.ErrDying
			}

		case <-timer.Chan():
			if syncRequired {
				if err := file.Sync(); err != nil {
					return errors.Trace(err)
				}
			}
			syncRequired = false

			timer.Reset(PollInterval)
		}
	}
}

type payload struct {
	log  log
	done chan<- error
}

type log struct {
	msg      string
	duration float64
	stmt     string
	stack    []byte
}

func (l log) String() string {
	return fmt.Sprintf(`slow query took %0.3fs for statement: %s
stack trace:
%s

`, l.duration, l.stmt, string(l.stack))
}
