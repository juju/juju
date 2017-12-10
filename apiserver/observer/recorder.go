// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package observer

import (
	"encoding/json"

	"github.com/juju/errors"

	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/rpc"
)

// NewRecorderFactory makes a new rpc.RecorderFactory to make
// recorders that that will update the observer and the auditlog
// recorder when it records a request or reply. The auditlog recorder
// can be nil.
func NewRecorderFactory(observerFactory rpc.ObserverFactory, recorder *auditlog.Recorder) rpc.RecorderFactory {
	return func() rpc.Recorder {
		return &combinedRecorder{
			observer: observerFactory.RPCObserver(),
			recorder: recorder,
		}
	}
}

// combinedRecorder wraps an observer (which might be a multiplexer)
// up with an auditlog recorder into an rpc.Recorder.
type combinedRecorder struct {
	observer rpc.Observer
	recorder *auditlog.Recorder
}

// HandleRequest implements rpc.Recorder.
func (cr *combinedRecorder) HandleRequest(hdr *rpc.Header, body interface{}) error {
	cr.observer.ServerRequest(hdr, body)
	if cr.recorder == nil {
		return nil
	}
	// TODO(babbageclunk): make this configurable.
	jsonArgs, err := json.Marshal(body)
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(cr.recorder.AddRequest(auditlog.RequestArgs{
		RequestID: hdr.RequestId,
		Facade:    hdr.Request.Type,
		Method:    hdr.Request.Action,
		Version:   hdr.Request.Version,
		Args:      string(jsonArgs),
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
