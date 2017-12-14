// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package observer

import (
	"encoding/json"

	"github.com/juju/errors"

	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/rpc"
)

const (
	// CaptureArgs means we'll serialize the API arguments and store
	// them in the audit log.
	CaptureArgs = true

	// NoCaptureArgs means don't do that.
	NoCaptureArgs = false
)

// NewRecorderFactory makes a new rpc.RecorderFactory to make
// recorders that that will update the observer and the auditlog
// recorder when it records a request or reply. The auditlog recorder
// can be nil.
func NewRecorderFactory(
	observerFactory rpc.ObserverFactory,
	recorder *auditlog.Recorder,
	captureArgs bool,
) rpc.RecorderFactory {
	return func() rpc.Recorder {
		return &combinedRecorder{
			observer:    observerFactory.RPCObserver(),
			recorder:    recorder,
			captureArgs: captureArgs,
		}
	}
}

// combinedRecorder wraps an observer (which might be a multiplexer)
// up with an auditlog recorder into an rpc.Recorder.
type combinedRecorder struct {
	observer    rpc.Observer
	recorder    *auditlog.Recorder
	captureArgs bool
}

// HandleRequest implements rpc.Recorder.
func (cr *combinedRecorder) HandleRequest(hdr *rpc.Header, body interface{}) error {
	cr.observer.ServerRequest(hdr, body)
	if cr.recorder == nil {
		return nil
	}
	var args string
	if cr.captureArgs {
		jsonArgs, err := json.Marshal(body)
		if err != nil {
			return errors.Trace(err)
		}
		args = string(jsonArgs)
	}
	return errors.Trace(cr.recorder.AddRequest(auditlog.RequestArgs{
		RequestID: hdr.RequestId,
		Facade:    hdr.Request.Type,
		Method:    hdr.Request.Action,
		Version:   hdr.Request.Version,
		Args:      args,
	}))
}

// HandleReply implements rpc.Recorder.
func (cr *combinedRecorder) HandleReply(req rpc.Request, replyHdr *rpc.Header, body interface{}) error {
	cr.observer.ServerReply(req, replyHdr, body)
	if cr.recorder == nil {
		return nil
	}
	var responseErrors []*auditlog.Error
	if replyHdr.Error == "" {
		var err error
		responseErrors, err = extractErrors(body)
		if err != nil {
			return errors.Trace(err)
		}
	} else {
		responseErrors = []*auditlog.Error{{
			Message: replyHdr.Error,
			Code:    replyHdr.ErrorCode,
		}}
	}
	return errors.Trace(cr.recorder.AddResponse(auditlog.ResponseErrorsArgs{
		RequestID: replyHdr.RequestId,
		Errors:    responseErrors,
	}))
}

func extractErrors(body interface{}) ([]*auditlog.Error, error) {
	// TODO(babbageclunk): use reflection to find errors in the response body.
	return nil, nil
}
