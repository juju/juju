// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package querylogger

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

// WorkerConfig encapsulates the configuration options for the
// dbaccessor worker.
type WorkerConfig struct {
	LogDir        string
	Clock         clock.Clock
	Logger        Logger
	StackGatherer func() []byte
}

// Validate ensures that the config values are valid.
func (c *WorkerConfig) Validate() error {
	if c.LogDir == "" {
		return errors.NotValidf("missing LogDir")
	}
	if c.Clock == nil {
		return errors.NotValidf("missing Clock")
	}
	if c.Logger == nil {
		return errors.NotValidf("missing Logger")
	}
	if c.StackGatherer == nil {
		return errors.NotValidf("missing StackGatherer")
	}
	return nil
}

// loggerWorker is a logger that can be used to log slow operations at the
// database level. It will print out debug messages for slow queries.
type loggerWorker struct {
	tomb tomb.Tomb

	clock         clock.Clock
	logger        Logger
	stackGatherer func() []byte

	logDir string
	logs   chan payload
}

// newWorker creates a new Worker, which can be used to log
// slow queries.
func newWorker(cfg *WorkerConfig) (*loggerWorker, error) {
	var err error
	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	l := &loggerWorker{
		logDir:        cfg.LogDir,
		clock:         cfg.Clock,
		logger:        cfg.Logger,
		stackGatherer: cfg.StackGatherer,

		logs: make(chan payload),
	}
	l.tomb.Go(l.loop)
	return l, nil
}

// RecordSlowQuery the slow query, with the given arguments.
func (l *loggerWorker) RecordSlowQuery(msg, stmt string, args []any, duration float64) {
	// Record the stack.
	// TODO (stickupkid): Prune the stack to remove the first few frames.
	stack := l.stackGatherer()

	done := make(chan error)
	select {
	case l.logs <- payload{
		log: log{
			duration: duration,
			stmt:     stmt,
			stack:    stack,
		},
		done: done,
	}:
	case <-l.tomb.Dying():
		return
	}

	var err error
	select {
	case err = <-done:
	case <-l.tomb.Dying():
		return
	}

	if err != nil {
		// Failed to log the slow query, log it to the main logger.
		l.logger.Warningf("failed to log slow query: %v", err)
		l.logger.Warningf("slow query: "+msg+"\n%s", append(args, stack)...)
		return
	}

	l.logger.Warningf("slow query: "+msg, args...)
}

// Kill is part of the worker.Worker interface.
func (w *loggerWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *loggerWorker) Wait() error {
	return w.tomb.Wait()
}

func (l *loggerWorker) loop() error {
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
			l.logger.Errorf("failed to sync slow query log: %v", err)
		}
		if err := file.Close(); err != nil {
			l.logger.Errorf("failed to close slow query log: %v", err)
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
					return errors.Annotatef(err, "failed to sync slow query log")
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
