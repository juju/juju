// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logforwarder

import (
	"io"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/logfwd"
	"github.com/juju/juju/worker/catacomb"
)

var logger = loggo.GetLogger("juju.worker.logforwarder")

// LogStream streams log entries from a log source (e.g. the Juju
// controller).
type LogStream interface {
	// Next returns the next log record from the stream.
	Next() (logfwd.Record, error)
}

// LogStreamFn is a function that opens a log stream.
type LogStreamFn func(_ base.APICaller, _ params.LogStreamConfig, controllerUUID string) (LogStream, error)

// SendCloser is responsible for sending log records to a log sink.
type SendCloser interface {
	sender
	io.Closer
}

type sender interface {
	// Send sends the record to its log sink. It is also responsible
	// for notifying the controller that record was forwarded.
	Send(logfwd.Record) error
}

// TODO(ericsnow) It is likely that eventually we will want to support
// multiplexing to multiple senders, each in its own goroutine (or worker).

// LogForwarder is a worker that forwards log records from a source
// to a sender.
type LogForwarder struct {
	catacomb catacomb.Catacomb
	stream   LogStream
	sender   sender
}

// OpenLogForwarderArgs holds the info needed to open a LogForwarder.
type OpenLogForwarderArgs struct {
	// AllModels indicates that the tracker is handling all models.
	AllModels bool

	// ControllerUUID identifies the controller.
	ControllerUUID string

	// Config is the logging config that will be used.
	Config LoggingConfig

	// Caller is the API caller that will be used.
	Caller base.APICaller

	// OpenSink is the function that opens the underlying log sink that
	// will be wrapped.
	OpenSink LogSinkFn

	// OpenLogStream is the function that will be used to for the
	// log stream.
	OpenLogStream LogStreamFn
}

func openLogForwarder(args OpenLogForwarderArgs) (*LogForwarder, error) {
	sink, err := OpenTrackingSink(TrackingSinkArgs{
		AllModels: args.AllModels,
		Config:    args.Config,
		Caller:    args.Caller,
		OpenSink:  args.OpenSink,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	streamCfg := params.LogStreamConfig{
		AllModels: args.AllModels,
		Sink:      sink.Name,
	}
	stream, err := args.OpenLogStream(args.Caller, streamCfg, args.ControllerUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	lf, err := NewLogForwarder(stream, sink)
	return lf, errors.Trace(err)
}

// NewLogForwarder returns a worker that forwards logs received from
// the stream to the sender.
func NewLogForwarder(stream LogStream, sender SendCloser) (*LogForwarder, error) {
	lf := &LogForwarder{
		stream: stream,
		sender: sender,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &lf.catacomb,
		Work: func() error {
			defer sender.Close()

			if stream == nil {
				logger.Debugf("log forwarding not enabled")
				return nil
			}

			err := lf.loop()
			return errors.Trace(err)
		},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return lf, nil
}

func (lf *LogForwarder) loop() error {
	records := make(chan logfwd.Record)
	go func() {
		for {
			rec, err := lf.stream.Next()
			if err != nil {
				lf.catacomb.Kill(errors.Trace(err))
				break
			}

			select {
			case <-lf.catacomb.Dying():
				break
			case records <- rec: // Wait until the last one is sent.
			}
		}
	}()

	for {
		select {
		case <-lf.catacomb.Dying():
			return lf.catacomb.ErrDying()
		case rec := <-records:
			if err := lf.sender.Send(rec); err != nil {
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
