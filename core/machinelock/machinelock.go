// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinelock

import (
	"context"
	"fmt"
	"runtime/debug"
	"sort"
	"sync"
	"time"

	"github.com/juju/collections/deque"
	"github.com/juju/lumberjack/v2"
	"github.com/juju/mutex/v2"
	"gopkg.in/yaml.v2"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/internal/errors"
)

// Filename represents the name of the logfile that is created in the LOG_DIR.
const Filename = "machine-lock.log"

// Lock is used to give external packages something to refer to.
type Lock interface {
	Acquire(spec Spec) (func(), error)
	Report(opts ...ReportOption) (string, error)
}

// Clock provides an interface for dealing with clocks.
type Clock interface {
	// After waits for the duration to elapse and then sends the
	// current time on the returned channel.
	After(time.Duration) <-chan time.Time

	// Now returns the current clock time.
	Now() time.Time
}

// Config defines the attributes needed to correctly construct
// a machine lock.
type Config struct {
	AgentName   string
	Clock       Clock
	Logger      logger.Logger
	LogFilename string
}

// Validate ensures that all the required config values are set.
func (c Config) Validate() error {
	if c.AgentName == "" {
		return errors.Errorf("missing AgentName %w", coreerrors.NotValid)
	}
	if c.Clock == nil {
		return errors.Errorf("missing Clock %w", coreerrors.NotValid)
	}
	if c.Logger == nil {
		return errors.Errorf("missing Logger %w", coreerrors.NotValid)
	}
	if c.LogFilename == "" {
		return errors.Errorf("missing LogFilename %w", coreerrors.NotValid)
	}
	return nil
}

// New creates a new machine lock.
func New(config Config) (*lock, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	if err := paths.PrimeLogFile(config.LogFilename); err != nil {
		// This isn't a fatal error so  continue if priming fails.
		_ = fmt.Sprintf("failed to create prime logfile in %s, because: %v", config.LogFilename, err)
	}
	lock := &lock{
		agent:       config.AgentName,
		clock:       config.Clock,
		logger:      config.Logger,
		logFilename: config.LogFilename,
		acquire:     mutex.Acquire,
		spec: mutex.Spec{
			Name:  "machine-lock",
			Clock: config.Clock,
			Delay: 250 * time.Millisecond,
			// Cancel is added in Acquire.
		},
		waiting: make(map[int]*info),
		history: deque.NewWithMaxLen(1000),
	}
	lock.setStartMessage()
	return lock, nil
}

func (c *lock) setStartMessage() {
	now := c.clock.Now().Format(timeFormat)
	// The reason that we don't attempt to write the start message out immediately
	// is that there may be multiple agents on the machine writing to the same log file.
	// The way that we control this is to open and close the file while we hold the
	// machine lock. We don't want to acquire the lock just to write out the agent
	// has started as this would potentially hold up the agent starting while another
	// agent holds the lock. However it is very useful to have the agent start time
	// recorded in the central file, so we save it and write it out the first time.
	c.startMessage = fmt.Sprintf("%s === agent %s started ===", now, c.agent)
}

// Spec is an argument struct for the `Acquire` method.
// It must have a Cancel channel and a Worker name defined.
type Spec struct {
	Cancel <-chan struct{}
	// The purpose of the NoCancel is to ensure that there isn't
	// an accidental forgetting of the cancel channel. The primary
	// use case for this is the reboot worker that doesn't want to
	// pass in a cancel channel because it really wants to reboot.
	NoCancel bool
	Worker   string
	Comment  string
	Group    string
}

// Validate ensures that a Cancel channel and a Worker name are defined.
func (s Spec) Validate() error {
	if s.Cancel == nil {
		if !s.NoCancel {
			return errors.Errorf("missing Cancel %w", coreerrors.NotValid)
		}
	}
	if s.Worker == "" {
		return errors.Errorf("missing Worker %w", coreerrors.NotValid)
	}
	return nil
}

// Acquire will attempt to acquire the machine hook execution lock.
// The method returns an error if the spec is invalid, or if the Cancel
// channel is signalled before the lock is acquired.
func (c *lock) Acquire(spec Spec) (func(), error) {
	if err := spec.Validate(); err != nil {
		return nil, errors.Capture(err)
	}
	current := &info{
		worker:    spec.Worker,
		comment:   spec.Comment,
		stack:     string(debug.Stack()),
		requested: c.clock.Now(),
	}
	c.mu.Lock()

	id := c.next
	c.next++
	c.waiting[id] = current

	mSpec := c.spec
	mSpec.Cancel = spec.Cancel
	if spec.Group != "" {
		mSpec.Name = fmt.Sprintf("%s-%s", mSpec.Name, spec.Group)
	}

	c.mu.Unlock()
	c.logger.Debugf(context.TODO(), "acquire machine lock %q for %s (%s)", mSpec.Name, spec.Worker, spec.Comment)
	releaser, err := c.acquire(mSpec)
	c.mu.Lock()
	defer c.mu.Unlock()
	// Remove from the waiting map.
	delete(c.waiting, id)

	if err != nil {
		return nil, errors.Capture(err)
	}
	c.logger.Debugf(context.TODO(), "machine lock %q acquired for %s (%s)", mSpec.Name, spec.Worker, spec.Comment)
	c.holder = current
	current.acquired = c.clock.Now()
	return func() {
		// We need to acquire the mutex before we call the releaser
		// to ensure that we move the current to the history before
		// another pending acquisition overwrites the current value.
		c.mu.Lock()
		defer c.mu.Unlock()
		// We write the log file entry before we release the execution
		// lock to ensure that no other agent is attempting to write to the
		// log file.
		current.released = c.clock.Now()
		c.writeLogEntry()
		c.logger.Debugf(context.TODO(), "machine lock %q released for %s (%s)", mSpec.Name, spec.Worker, spec.Comment)
		releaser.Release()
		c.history.PushFront(current)
		c.holder = nil
	}, nil
}

func (c *lock) writeLogEntry() {
	// At the time this method is called, the holder is still set and the lock's
	// mutex is held.
	writer := &lumberjack.Logger{
		Filename:   c.logFilename,
		MaxSize:    10, // megabytes
		MaxBackups: 5,
		Compress:   true,
	}
	c.logger.Debugf(context.TODO(), "created rotating log file %q with max size %d MB and max backups %d",
		writer.Filename, writer.MaxSize, writer.MaxBackups)
	defer func() { _ = writer.Close() }()

	if c.startMessage != "" {
		_, err := fmt.Fprintln(writer, c.startMessage)
		if err != nil {
			c.logger.Warningf(context.TODO(), "unable to write startMessage: %s", err.Error())
		}
		c.startMessage = ""
	}

	_, err := fmt.Fprintln(writer, simpleInfo(c.agent, c.holder, c.clock.Now()))
	if err != nil {
		c.logger.Warningf(context.TODO(), "unable to release message: %s", err.Error())
	}
}

type info struct {
	// worker is the worker that wants or has the lock.
	worker string
	// comment is provided by the worker to say what they are doing.
	comment string
	// stack trace for additional debugging
	stack string

	requested time.Time
	acquired  time.Time
	released  time.Time
}

type lock struct {
	agent        string
	clock        Clock
	logger       logger.Logger
	logFilename  string
	startMessage string

	acquire func(mutex.Spec) (mutex.Releaser, error)

	spec mutex.Spec

	mu      sync.Mutex
	next    int
	holder  *info
	waiting map[int]*info
	history *deque.Deque
}

type ReportOption int

const (
	ShowHistory ReportOption = iota
	ShowStack
	ShowDetailsYAML
)

func contains(opts []ReportOption, opt ReportOption) bool {
	for _, value := range opts {
		if value == opt {
			return true
		}
	}
	return false
}

type reportInfo struct {
	Worker  string `yaml:"worker"`
	Comment string `yaml:"comment,omitempty"`

	Requested string `yaml:"requested,omitempty"`
	Acquired  string `yaml:"acquired,omitempty"`
	Released  string `yaml:"released,omitempty"`

	WaitTime time.Duration `yaml:"wait-time,omitempty"`
	HoldTime time.Duration `yaml:"hold-time,omitempty"`

	Stack string `yaml:"stack,omitempty"`
}

type report struct {
	Holder  interface{}   `yaml:"holder"`
	Waiting []interface{} `yaml:"waiting,omitempty"`
	History []interface{} `yaml:"history,omitempty"`
}

func (c *lock) Report(opts ...ReportOption) (string, error) {
	includeStack := contains(opts, ShowStack)
	detailsYAML := contains(opts, ShowDetailsYAML)

	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.clock.Now()

	r := report{
		Holder: displayInfo(c.holder, includeStack, detailsYAML, now),
	}
	// Show the waiting with oldest first, which will have the smallest
	// map key.
	for _, key := range sortedKeys(c.waiting) {
		r.Waiting = append(r.Waiting, displayInfo(c.waiting[key], includeStack, detailsYAML, now))
	}
	if contains(opts, ShowHistory) {
		iter := c.history.Iterator()
		var v *info
		for iter.Next(&v) {
			r.History = append(r.History, displayInfo(v, includeStack, detailsYAML, now))
		}
	}

	output := map[string]report{c.agent: r}
	out, err := yaml.Marshal(output)
	if err != nil {
		return "", errors.Capture(err)
	}
	return string(out), nil
}

func sortedKeys(m map[int]*info) []int {
	values := make([]int, 0, len(m))
	for key := range m {
		values = append(values, key)
	}
	sort.Ints(values)
	return values
}

func timeOutput(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.String()
}

func displayInfo(info *info, includeStack, detailsYAML bool, now time.Time) interface{} {
	if !detailsYAML {
		return simpleInfo("", info, now)
	}
	if info == nil {
		return nil
	}
	output := reportInfo{
		Worker:    info.worker,
		Comment:   info.comment,
		Requested: timeOutput(info.requested),
		Acquired:  timeOutput(info.acquired),
		Released:  timeOutput(info.released),
	}
	var other time.Time
	if info.acquired.IsZero() {
		other = now
	} else {
		if info.released.IsZero() {
			other = now
		} else {
			other = info.released
		}
		output.HoldTime = other.Sub(info.acquired).Round(time.Second)
		// Now set other for the wait time.
		other = info.acquired
	}
	output.WaitTime = other.Sub(info.requested).Round(time.Second)
	return &output
}

func simpleInfo(agent string, info *info, now time.Time) string {
	if info == nil {
		return "none"
	}
	msg := info.worker
	if info.comment != "" {
		msg += " (" + info.comment + ")"
	}
	// We pass in agent when writing to the file, but not for the report.
	// This allows us to have the agent in the file but keep the first column
	// aligned for timestamps.
	if agent != "" {
		msg = agent + ": " + msg
	}
	if info.acquired.IsZero() {
		waiting := now.Sub(info.requested).Round(time.Second)
		return fmt.Sprintf("%s, waiting %s", msg, waiting)
	}
	if info.released.IsZero() {
		holding := now.Sub(info.acquired).Round(time.Second)
		return fmt.Sprintf("%s, holding %s", msg, holding)
	}
	ts := info.released.Format(timeFormat)
	waited := info.acquired.Sub(info.requested).Round(time.Second)
	held := info.released.Sub(info.acquired).Round(time.Second)
	return fmt.Sprintf("%s %s, waited %s, held %s", ts, msg, waited, held)
}

const timeFormat = "2006-01-02 15:04:05"
