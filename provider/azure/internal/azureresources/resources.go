// This file is based on code from Azure/azure-sdk-for-go,
// which is Copyright Microsoft Corporation. See the LICENSE
// file in this directory for details.
package resources

import (
	"net/http"

	"github.com/Azure/azure-sdk-for-go/arm/resources/resources"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/validation"
)

// ResourcesClient wraps resources.GroupClient, providing methods for
// dealing with generic resources.
//
// NOTE(axw) this wrapper is necessary only to work around an issue in
// the generated GroupClient, which hard-codes the API version:
//     https://github.com/Azure/azure-sdk-for-go/issues/741
// When this issue has been resolved, we should drop this code and use
// the SDK's client directly.
type ResourcesClient struct {
	*resources.GroupClient
}

// GetByID gets a resource by ID.
//
// See: resources.GroupClient.GetByID.
func (client ResourcesClient) GetByID(resourceID, apiVersion string) (result resources.GenericResource, err error) {
	req, err := client.GetByIDPreparer(resourceID)
	if err != nil {
		err = autorest.NewErrorWithError(err, "resources.GroupClient", "GetByID", nil, "Failure preparing request")
		return
	}
	setAPIVersion(req, apiVersion)

	resp, err := client.GetByIDSender(req)
	if err != nil {
		result.Response = autorest.Response{Response: resp}
		err = autorest.NewErrorWithError(err, "resources.GroupClient", "GetByID", resp, "Failure sending request")
		return
	}

	result, err = client.GetByIDResponder(resp)
	if err != nil {
		err = autorest.NewErrorWithError(err, "resources.GroupClient", "GetByID", resp, "Failure responding to request")
	}
	return
}

// CreateOrUpdateByID creates a resource.
//
// See: resources.GroupClient.CreateOrUpdateByID.
func (client ResourcesClient) CreateOrUpdateByID(
	resourceID string,
	parameters resources.GenericResource,
	cancel <-chan struct{},
	apiVersion string,
) (<-chan resources.GenericResource, <-chan error) {
	resultChan := make(chan resources.GenericResource, 1)
	errChan := make(chan error, 1)
	if err := validation.Validate([]validation.Validation{
		{TargetValue: parameters,
			Constraints: []validation.Constraint{{Target: "parameters.Kind", Name: validation.Null, Rule: false,
				Chain: []validation.Constraint{{Target: "parameters.Kind", Name: validation.Pattern, Rule: `^[-\w\._,\(\)]+$`, Chain: nil}}}}}}); err != nil {
		errChan <- validation.NewErrorWithValidationError(err, "resources.GroupClient", "CreateOrUpdateByID")
		close(errChan)
		close(resultChan)
		return resultChan, errChan
	}

	go func() {
		var err error
		var result resources.GenericResource
		defer func() {
			resultChan <- result
			errChan <- err
			close(resultChan)
			close(errChan)
		}()
		req, err := client.CreateOrUpdateByIDPreparer(resourceID, parameters, cancel)
		if err != nil {
			err = autorest.NewErrorWithError(err, "resources.GroupClient", "CreateOrUpdateByID", nil, "Failure preparing request")
			return
		}
		setAPIVersion(req, apiVersion)

		resp, err := client.CreateOrUpdateByIDSender(req)
		if err != nil {
			result.Response = autorest.Response{Response: resp}
			err = autorest.NewErrorWithError(err, "resources.GroupClient", "CreateOrUpdateByID", resp, "Failure sending request")
			return
		}

		result, err = client.CreateOrUpdateByIDResponder(resp)
		if err != nil {
			err = autorest.NewErrorWithError(err, "resources.GroupClient", "CreateOrUpdateByID", resp, "Failure responding to request")
		}
	}()
	return resultChan, errChan
}

func setAPIVersion(req *http.Request, apiVersion string) {
	// Replace the API version.
	query := req.URL.Query()
	query.Set("api-version", apiVersion)
	req.URL.RawQuery = query.Encode()
}
