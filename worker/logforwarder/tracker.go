// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logforwarder

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	logfwdapi "github.com/juju/juju/api/logfwd"
	"github.com/juju/juju/logfwd"
)

// TrackingSinkArgs holds the args to OpenTrackingSender.
type TrackingSinkArgs struct {
	// AllModels indicates that the tracker is handling all models.
	AllModels bool

	// Config is the logging config that will be used.
	Config LoggingConfig

	// Caller is the API caller that will be used.
	Caller base.APICaller

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

	sink.SendCloser = &trackingSender{
		SendCloser: sink,
		tracker:    newLastSentTracker(sink.Name, args.Caller),
	}
	return sink, nil
}

type trackingSender struct {
	SendCloser
	tracker   *lastSentTracker
	allModels bool
}

// Send implements Sender.
func (s trackingSender) Send(rec logfwd.Record) error {
	if err := s.SendCloser.Send(rec); err != nil {
		return errors.Trace(err)
	}
	model := rec.Origin.ModelUUID
	if s.allModels {
		model = ""
	}
	if err := s.tracker.setOne(model, rec.ID); err != nil {
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

// TODO(ericsnow) We need to leverage the bulk API here.

func (lst lastSentTracker) setOne(model string, recID int64) error {
	var modelTag names.ModelTag
	if model != "" {
		if !names.IsValidModel(model) {
			return errors.Errorf("bad model UUID %q", model)
		}
		modelTag = names.NewModelTag(model)
	}

	results, err := lst.client.SetList([]logfwdapi.LastSentInfo{{
		LastSentID: logfwdapi.LastSentID{
			Model: modelTag,
			Sink:  lst.sink,
		},
		RecordID: recID,
	}})
	if err != nil {
		return errors.Trace(err)
	}
	if err := results[0].Error; err != nil {
		return errors.Trace(err)
	}
	return nil
}
