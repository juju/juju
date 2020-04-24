// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logforwarder

import (
	"io"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/worker/v2/catacomb"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/logfwd"
)

// logger is here to stop the desire of creating a package level logger.
// Don't do this, instead use the one passed as manifold config.
var logger interface{}

// LogStream streams log entries from a log source (e.g. the Juju controller).
type LogStream interface {
	// Next returns the next batch of log records from the stream.
	Next() ([]logfwd.Record, error)
}

// LogStreamFn is a function that opens a log stream.
type LogStreamFn func(_ base.APICaller, _ params.LogStreamConfig, controllerUUID string) (LogStream, error)

// SendCloser is responsible for sending log records to a log sink.
type SendCloser interface {
	sender
	io.Closer
}

type sender interface {
	// Send sends the records to its log sink. It is also responsible
	// for notifying the controller that record was forwarded.
	Send([]logfwd.Record) error
}

// TODO(ericsnow) It is likely that eventually we will want to support
// multiplexing to multiple senders, each in its own goroutine (or worker).

// LogForwarder is a worker that forwards log records from a source
// to a sender.
type LogForwarder struct {
	catacomb  catacomb.Catacomb
	args      OpenLogForwarderArgs
	enabledCh chan bool
	mu        sync.Mutex
	enabled   bool
}

// OpenLogForwarderArgs holds the info needed to open a LogForwarder.
type OpenLogForwarderArgs struct {
	// ControllerUUID identifies the controller.
	ControllerUUID string

	// LogForwardConfig is the API used to access log forwarding config.
	LogForwardConfig LogForwardConfig

	// Caller is the API caller that will be used.
	Caller base.APICaller

	// Name is the name given to the log sink.
	Name string

	// OpenSink is the function that opens the underlying log sink that
	// will be wrapped.
	OpenSink LogSinkFn

	// OpenLogStream is the function that will be used to for the
	// log stream.
	OpenLogStream LogStreamFn

	Logger Logger
}

// processNewConfig acts on a new syslog forward config change.
func (lf *LogForwarder) processNewConfig(currentSender SendCloser) (SendCloser, error) {
	lf.mu.Lock()
	defer lf.mu.Unlock()

	closeExisting := func() error {
		lf.enabled = false
		// If we are already sending, close the current sender.
		if currentSender != nil {
			return currentSender.Close()
		}
		return nil
	}

	// Get the new config and set up log forwarding if enabled.
	cfg, ok, err := lf.args.LogForwardConfig.LogForwardConfig()
	if err != nil {
		closeExisting()
		return nil, errors.Trace(err)
	}
	if !ok || !cfg.Enabled {
		lf.args.Logger.Infof("config change - log forwarding not enabled")
		return nil, closeExisting()
	}
	// If the config is not valid, we don't want to exit with an error
	// and bounce the worker; we'll just log the issue and wait for another
	// config change to come through.
	// We'll continue sending using the current sink.
	if err := cfg.Validate(); err != nil {
		lf.args.Logger.Errorf("invalid log forward config change: %v", err)
		return currentSender, nil
	}

	// Shutdown the existing sink since we need to now create a new one.
	if err := closeExisting(); err != nil {
		return nil, errors.Trace(err)
	}
	sink, err := OpenTrackingSink(TrackingSinkArgs{
		Name:     lf.args.Name,
		Config:   cfg,
		Caller:   lf.args.Caller,
		OpenSink: lf.args.OpenSink,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	lf.enabledCh <- true
	return sink, nil
}

// waitForEnabled returns true if streaming is enabled.
// Otherwise if blocks and waits for enabled to be true.
func (lf *LogForwarder) waitForEnabled() (bool, error) {
	lf.mu.Lock()
	enabled := lf.enabled
	lf.mu.Unlock()
	if enabled {
		return true, nil
	}

	select {
	case <-lf.catacomb.Dying():
		return false, tomb.ErrDying
	case enabled = <-lf.enabledCh:
	}
	lf.mu.Lock()
	defer lf.mu.Unlock()

	if !lf.enabled && enabled {
		lf.args.Logger.Infof("log forward enabled, starting to stream logs to syslog sink")
	}
	lf.enabled = enabled
	return enabled, nil
}

// NewLogForwarder returns a worker that forwards logs received from
// the stream to the sender.
func NewLogForwarder(args OpenLogForwarderArgs) (*LogForwarder, error) {
	lf := &LogForwarder{
		args:      args,
		enabledCh: make(chan bool, 1),
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &lf.catacomb,
		Work: func() error {
			return errors.Trace(lf.loop())
		},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return lf, nil
}

func (lf *LogForwarder) loop() error {
	configWatcher, err := lf.args.LogForwardConfig.WatchForLogForwardConfigChanges()
	if err != nil {
		return errors.Trace(err)
	}
	if err := lf.catacomb.Add(configWatcher); err != nil {
		return errors.Trace(err)
	}

	records := make(chan []logfwd.Record)
	var stream LogStream
	go func() {
		for {
			enabled, err := lf.waitForEnabled()
			if err == tomb.ErrDying {
				return
			}
			if !enabled {
				continue
			}
			// Lazily create log streamer if needed.
			if stream == nil {
				streamCfg := params.LogStreamConfig{
					Sink: lf.args.Name,
					// TODO(wallyworld) - this should be configurable via lf.args.LogForwardConfig
					MaxLookbackRecords: 100,
				}
				stream, err = lf.args.OpenLogStream(lf.args.Caller, streamCfg, lf.args.ControllerUUID)
				if err != nil {
					lf.catacomb.Kill(errors.Annotate(err, "creating log stream"))
					break
				}

			}
			rec, err := stream.Next()
			if err != nil {
				lf.catacomb.Kill(errors.Annotate(err, "getting next log record"))
				break
			}
			select {
			case <-lf.catacomb.Dying():
				return
			case records <- rec: // Wait until the last one is sent.
			}
		}
	}()

	var sender SendCloser
	defer func() {
		if sender != nil {
			sender.Close()
		}
	}()

	for {
		select {
		case <-lf.catacomb.Dying():
			return lf.catacomb.ErrDying()
		case _, ok := <-configWatcher.Changes():
			if !ok {
				return errors.New("syslog configuration watcher closed")
			}
			if sender, err = lf.processNewConfig(sender); err != nil {
				return errors.Trace(err)
			}
		case rec := <-records:
			if sender == nil {
				continue
			}
			if err := sender.Send(rec); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

// Kill implements Worker.Kill()
func (lf *LogForwarder) Kill() {
	lf.catacomb.Kill(nil)
}

// Wait implements Worker.Wait()
func (lf *LogForwarder) Wait() error {
	return lf.catacomb.Wait()
}
