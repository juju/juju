// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azureauth

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/Azure/azure-sdk-for-go/services/graphrbac/1.6/graphrbac"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/juju/errors"
)

// ResourceManagerResourceId returns the resource ID for the
// Azure Resource Manager application to use in auth requests,
// based on the given core endpoint URI (e.g. https://core.windows.net).
//
// The core endpoint URI is the same as given in "storage-endpoint"
// in Azure cloud definitions, which serves as the suffix for blob
// storage URLs.
func ResourceManagerResourceId(coreEndpointURI string) (string, error) {
	u, err := url.Parse(coreEndpointURI)
	if err != nil {
		return "", err
	}
	u.Host = "management." + u.Host
	return TokenResource(u.String()), nil
}

// TokenResource returns a resource value suitable for auth tokens, based on
// an endpoint URI.
func TokenResource(uri string) string {
	resource := uri
	if !strings.HasSuffix(resource, "/") {
		resource += "/"
	}
	return resource
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
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return errors.Trace(err), false
		}
		resp.Body = io.NopCloser(bytes.NewReader(b))

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
