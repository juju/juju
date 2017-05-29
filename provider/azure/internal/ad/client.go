// This file is based on code from Azure/azure-sdk-for-go
// and Azure/go-autorest, which are both
// Copyright Microsoft Corporation. See the LICENSE
// file in this directory for details.
//
// NOTE(axw) this file contains a client for a subset of the
// Microsoft Graph API, which is not currently supported by
// the Azure SDK. When it is, this will be deleted.

package ad

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"

	"github.com/juju/juju/version"
)

const (
	// APIVersion is the version of the Active Directory API
	APIVersion = "1.6"
)

var (
	// utf8BOM is the UTF-8 BOM, which may come at the beginning
	// of a UTF-8 stream. Since it has no effect on the stream,
	// it can be stripped off and ignored.
	utf8BOM = []byte("\ufeff")
)

func UserAgent() string {
	return fmt.Sprintf("Juju/%s arm-graphrbac/%s", version.Current, APIVersion)
}

type ManagementClient struct {
	autorest.Client
	BaseURI    string
	APIVersion string
}

func NewManagementClient(baseURI string) ManagementClient {
	return ManagementClient{
		Client:     autorest.NewClientWithUserAgent(UserAgent()),
		BaseURI:    baseURI,
		APIVersion: APIVersion,
	}
}

// WithOdataErrorUnlessStatusCode returns a RespondDecorator that emits an
// azure.RequestError by reading the response body unless the response HTTP status code
// is among the set passed.
//
// If there is a chance service may return responses other than the Azure error
// format and the response cannot be parsed into an error, a decoding error will
// be returned containing the response body. In any case, the Responder will
// return an error if the status code is not satisfied.
//
// If this Responder returns an error, the response body will be replaced with
// an in-memory reader, which needs no further closing.
//
// NOTE(axw) this function is based on go-autorest/autorest/azure.WithErrorUnlessStatusCode.
// The only difference is that we extract "odata.error", instead of "error",
// from the response body; and we do not extract the message, which is in a
// different format for odata.error, and irrelevant to us.
func WithOdataErrorUnlessStatusCode(codes ...int) autorest.RespondDecorator {
	return func(r autorest.Responder) autorest.Responder {
		return autorest.ResponderFunc(func(resp *http.Response) error {
			err := r.Respond(resp)
			if err == nil && !autorest.ResponseHasStatusCode(resp, codes...) {
				var oe odataRequestError
				defer resp.Body.Close()

				// Copy and replace the Body in case it does not contain an error object.
				// This will leave the Body available to the caller.
				data, readErr := ioutil.ReadAll(resp.Body)
				if readErr != nil {
					return readErr
				}
				resp.Body = ioutil.NopCloser(bytes.NewReader(data))
				decodeErr := json.Unmarshal(bytes.TrimPrefix(data, utf8BOM), &oe)

				if decodeErr != nil {
					return fmt.Errorf("ad: error response cannot be parsed: %q error: %v", data, decodeErr)
				} else if oe.ServiceError == nil {
					oe.ServiceError = &odataServiceError{Code: "Unknown"}
				}

				e := azure.RequestError{
					ServiceError: &azure.ServiceError{
						Code:    oe.ServiceError.Code,
						Message: oe.ServiceError.Message.Value,
					},
					RequestID: azure.ExtractRequestID(resp),
				}
				if e.StatusCode == nil {
					e.StatusCode = resp.StatusCode
				}
				err = &e
			}
			return err
		})
	}
}

// See https://msdn.microsoft.com/en-us/library/azure/ad/graph/howto/azure-ad-graph-api-error-codes-and-error-handling
type odataRequestError struct {
	ServiceError *odataServiceError `json:"odata.error"`
}

type odataServiceError struct {
	Code    string                   `json:"code"`
	Message odataServiceErrorMessage `json:"message"`
}

type odataServiceErrorMessage struct {
	Lang  string `json:"lang"`
	Value string `json:"value"`
}
