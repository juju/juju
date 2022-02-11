// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package observer

import (
	"encoding/json"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/core/auditlog"
	"github.com/juju/juju/rpc"
	"github.com/juju/juju/rpc/params"
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
		responseErrors = extractErrors(body)
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

func extractErrors(body interface{}) []*auditlog.Error {
	// To find errors in the API responses, we look for a struct where
	// there is an attribute that is:
	// * a slice of structs that have an attribute that is *Error or
	// * a plain old *Error
	// I thought we'd need to handle a []*Error as well, but it turns
	// out we don't use that in the API.
	value := reflect.ValueOf(body)
	if value.Kind() != reflect.Struct {
		return nil
	}

	// Prefer a slice of structs with Errors.
	for i := 0; i < value.NumField(); i++ {
		attr := value.Field(i)
		if errors, ok := tryStructSliceErrors(attr); ok {
			return convertErrors(errors)
		}
	}

	for i := 0; i < value.NumField(); i++ {
		attr := value.Field(i)
		if err, ok := tryErrorPointer(attr); ok {
			return convertErrors([]*params.Error{err})
		}
	}
	return nil
}

func tryErrorPointer(value reflect.Value) (*params.Error, bool) {
	if !value.CanInterface() {
		return nil, false
	}
	err, ok := value.Interface().(*params.Error)
	return err, ok
}

func tryStructSliceErrors(value reflect.Value) ([]*params.Error, bool) {
	if value.Kind() != reflect.Slice {
		return nil, false
	}
	itemType := value.Type().Elem()
	if itemType.Kind() != reflect.Struct {
		return nil, false
	}
	errorField, found := findErrorField(itemType)
	if !found {
		return nil, false
	}

	result := make([]*params.Error, value.Len())
	for i := 0; i < value.Len(); i++ {
		item := value.Index(i)
		// We know item's a struct.
		errorValue := item.Field(errorField)
		// This will assign nil if we couldn't extract the field (it
		// wasn't exported for example), but that's OK.
		result[i], _ = tryErrorPointer(errorValue)
	}
	return result, true
}

var errorType = reflect.TypeOf(params.Error{})

func findErrorField(itemType reflect.Type) (int, bool) {
	for i := 0; i < itemType.NumField(); i++ {
		field := itemType.Field(i)
		if field.Type.Kind() != reflect.Ptr {
			continue
		}
		if field.Type.Elem() == errorType {
			return i, true
		}
	}
	return 0, false
}

func convertErrors(errors []*params.Error) []*auditlog.Error {
	result := make([]*auditlog.Error, len(errors))
	for i, err := range errors {
		if err == nil {
			continue
		}
		result[i] = &auditlog.Error{
			Message: err.Message,
			Code:    err.Code,
		}
	}
	return result
}
