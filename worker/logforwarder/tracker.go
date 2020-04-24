// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logforwarder

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	logfwdapi "github.com/juju/juju/api/logfwd"
	"github.com/juju/juju/logfwd"
	"github.com/juju/juju/logfwd/syslog"
)

// TrackingSinkArgs holds the args to OpenTrackingSender.
type TrackingSinkArgs struct {
	// Config is the logging config that will be used.
	Config *syslog.RawConfig

	// Caller is the API caller that will be used.
	Caller base.APICaller

	// Name is the name given to the log sink.
	Name string

	// OpenSink is the function that opens the underlying log sink that
	// will be wrapped.
	OpenSink LogSinkFn
}

// OpenTrackingSink opens a log record sender to use with a worker.
// The sender also tracks records that were successfully sent.
func OpenTrackingSink(args TrackingSinkArgs) (*LogSink, error) {
	sink, err := args.OpenSink(args.Config)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &LogSink{
		&trackingSender{
			SendCloser: sink,
			tracker:    newLastSentTracker(args.Name, args.Caller),
		},
	}, nil
}

type trackingSender struct {
	SendCloser
	tracker *lastSentTracker
}

// Send implements Sender.
func (s *trackingSender) Send(records []logfwd.Record) error {
	if err := s.SendCloser.Send(records); err != nil {
		return errors.Trace(err)
	}
	if err := s.tracker.setLastSent(records); err != nil {
		return errors.Trace(err)
	}
	return nil
}

type lastSentTracker struct {
	sink   string
	client *logfwdapi.LastSentClient
}

func newLastSentTracker(sink string, caller base.APICaller) *lastSentTracker {
	client := logfwdapi.NewLastSentClient(func(name string) logfwdapi.FacadeCaller {
		return base.NewFacadeCaller(caller, name)
	})
	return &lastSentTracker{
		sink:   sink,
		client: client,
	}
}

func (lst lastSentTracker) setLastSent(records []logfwd.Record) error {
	// The records are received and sent in order, so we only need to
	// call SetLastSent for the last record.
	if len(records) == 0 {
		return nil
	}
	rec := records[len(records)-1]
	model := rec.Origin.ModelUUID
	if !names.IsValidModel(model) {
		return errors.Errorf("bad model UUID %q", model)
	}
	modelTag := names.NewModelTag(model)
	results, err := lst.client.SetLastSent([]logfwdapi.LastSentInfo{{
		LastSentID: logfwdapi.LastSentID{
			Model: modelTag,
			Sink:  lst.sink,
		},
		RecordID:        rec.ID,
		RecordTimestamp: rec.Timestamp,
	}})
	if err != nil {
		return errors.Trace(err)
	}
	if err := results[0].Error; err != nil {
		return errors.Trace(err)
	}
	return nil
}
