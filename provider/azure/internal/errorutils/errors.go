// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errorutils

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/services/graphrbac/1.6/graphrbac"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/common"
)

var logger = loggo.GetLogger("juju.provider.azure")

// ServiceError returns the *azure.ServiceError underlying the
// supplied error, if any, and a bool indicating whether one
// was found.
func ServiceError(err error) (*azure.ServiceError, bool) {
	if err == nil {
		return nil, false
	}
	err = errors.Cause(err)
	if d, ok := err.(autorest.DetailedError); ok {
		err = errors.Cause(d.Original)
	}
	if se, ok := err.(*azure.ServiceError); ok {
		return se, true
	}
	if r, ok := err.(*azure.RequestError); ok {
		return r.ServiceError, true
	}
	// The error Azure gives us back can also be a struct
	// not a pointer.
	if se, ok := err.(azure.ServiceError); ok {
		return &se, true
	}
	if r, ok := err.(azure.RequestError); ok {
		return r.ServiceError, true
	}
	return nil, false
}

// QuotaExceededError returns the relevant error message and true
// if the error is caused by a Quota Exceeded issue.
func QuotaExceededError(err error) (error, bool) {
	serviceErr, ok := ServiceError(err)
	if !ok {
		return err, false
	}
	if serviceErr.Code == "QuotaExceeded" {
		return errors.Errorf("%v", serviceErr.Message), true
	}
	for _, d := range serviceErr.Details {
		if code, ok := d["code"].(string); ok && code == "QuotaExceeded" {
			return errors.Errorf("%v", d["message"]), true
		}
	}
	return err, false
}

// IsConflictError returns true if the error is
// caused by a deployment Conflict.
func IsConflictError(err error) bool {
	serviceErr, ok := ServiceError(err)
	if !ok {
		return false
	}
	if serviceErr.Code == "Conflict" {
		return true
	}
	for _, d := range serviceErr.Details {
		if code, ok := d["code"].(string); ok && code == "Conflict" {
			return true
		}
	}
	return false
}

// HandleCredentialError determines if given error relates to invalid credential.
// If it is, the credential is invalidated.
// Original error is returned untouched.
func HandleCredentialError(err error, ctx context.ProviderCallContext) error {
	MaybeInvalidateCredential(err, ctx)
	return err
}

// MaybeInvalidateCredential determines if given error is related to authentication/authorisation failures.
// If an error is related to an invalid credential, then this call will try to invalidate that credential as well.
func MaybeInvalidateCredential(err error, ctx context.ProviderCallContext) bool {
	if ctx == nil {
		return false
	}
	if !HasDenialStatusCode(err) {
		return false
	}

	converted := common.CredentialNotValidf(err, "azure cloud denied access")
	invalidateErr := ctx.InvalidateCredential(converted.Error())
	if invalidateErr != nil {
		logger.Warningf("could not invalidate stored azure cloud credential on the controller: %v", invalidateErr)
	}
	return true
}

// HasDenialStatusCode returns true of the error has a status code
// meaning that the credential is invalid.
func HasDenialStatusCode(err error) bool {
	if err == nil {
		return false
	}

	if d, ok := errors.Cause(err).(autorest.DetailedError); ok {
		if d.Response != nil {
			return common.AuthorisationFailureStatusCodes.Contains(d.Response.StatusCode)
		}
		statusCode, _ := d.StatusCode.(int)
		return common.AuthorisationFailureStatusCodes.Contains(statusCode)
	}
	return false
}

// CheckForDetailedError attempts to unmarshal the body into a DetailedError.
// If this succeeds then the DetailedError is returned as an error,
// otherwise the response is passed on to the next Responder.
func CheckForDetailedError(r autorest.Responder) autorest.Responder {
	return autorest.ResponderFunc(func(resp *http.Response) error {
		if resp.Body != nil {
			defer resp.Body.Close()
			b, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				return errors.Trace(err)
			}
			if len(b) > 0 {
				resp.Body = ioutil.NopCloser(bytes.NewReader(b))
				// Remove any UTF-8 BOM, if present.
				b = bytes.TrimPrefix(b, []byte("\ufeff"))
				var de autorest.DetailedError
				if err := json.Unmarshal(b, &de); err == nil {
					return de
				}
			}
		}
		return r.Respond(resp)
	})
}

// CheckForGraphError attempts to unmarshal the body into a GraphError.
// If this succeeds then the GraphError is returned as an error,
// otherwise the response is passed on to the next Responder.
func CheckForGraphError(r autorest.Responder) autorest.Responder {
	return autorest.ResponderFunc(func(resp *http.Response) error {
		err, _ := maybeGraphError(resp)
		if err != nil {
			return errors.Trace(err)
		}
		return r.Respond(resp)
	})
}

func maybeGraphError(resp *http.Response) (error, bool) {
	if resp.Body != nil {
		defer resp.Body.Close()
		b, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return errors.Trace(err), false
		}
		resp.Body = ioutil.NopCloser(bytes.NewReader(b))

		// Remove any UTF-8 BOM, if present.
		b = bytes.TrimPrefix(b, []byte("\ufeff"))
		var ge graphrbac.GraphError
		if err := json.Unmarshal(b, &ge); err == nil {
			if ge.OdataError != nil && ge.Code != nil {
				return &GraphError{ge}, true
			}
		}
	}
	return nil, false
}

// GraphError is a go error that wraps the graphrbac.GraphError response
// type, which doesn't implement the error interface.
type GraphError struct {
	graphrbac.GraphError
}

// Code returns the code from the GraphError.
func (e *GraphError) Code() string {
	return *e.GraphError.Code
}

// Message returns the message from the GraphError.
func (e *GraphError) Message() string {
	if e.GraphError.OdataError == nil || e.GraphError.ErrorMessage == nil || e.GraphError.Message == nil {
		return ""
	}
	return *e.GraphError.Message
}

// Error implements the error interface.
func (e *GraphError) Error() string {
	s := e.Code()
	if m := e.Message(); m != "" {
		s += ": " + m
	}
	return s
}

// AsGraphError returns a GraphError if one is contained within the given
// error, otherwise it returns nil.
func AsGraphError(err error) *GraphError {
	err = errors.Cause(err)
	if de, ok := err.(autorest.DetailedError); ok {
		err = de.Original
	}
	if ge, _ := err.(*GraphError); ge != nil {
		return ge
	}
	if de, ok := err.(*azure.RequestError); ok {
		ge, ok := maybeGraphError(de.Response)
		if ok {
			return ge.(*GraphError)
		}
	}
	return nil
}
