// This file is based on code from Azure/azure-sdk-for-go,
// which is Copyright Microsoft Corporation. See the LICENSE
// file in this directory for details.
package resources

import (
	"context"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2018-05-01/resources"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/validation"
	"github.com/Azure/go-autorest/tracing"
)

// The package's fully qualified name.
const fqdn = "github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2018-05-01/resources"

// ResourcesClient wraps resources.Client, providing methods for
// dealing with generic resources.
//
// NOTE(axw) this wrapper is necessary only to work around an issue in
// the generated Client, which hard-codes the API version:
//     https://github.com/Azure/azure-sdk-for-go/issues/741
// When this issue has been resolved, we should drop this code and use
// the SDK's client directly.
type ResourcesClient struct {
	*resources.Client
}

// GetByID gets a resource by ID.
//
// See: resources.Client.GetByID.
func (client ResourcesClient) GetByID(ctx context.Context, resourceID, apiVersion string) (result resources.GenericResource, err error) {
	if tracing.IsEnabled() {
		ctx = tracing.StartSpan(ctx, fqdn+"/Client.GetByID")
		defer func() {
			sc := -1
			if result.Response.Response != nil {
				sc = result.Response.Response.StatusCode
			}
			tracing.EndSpan(ctx, sc, err)
		}()
	}
	req, err := client.GetByIDPreparer(ctx, resourceID)
	if err != nil {
		err = autorest.NewErrorWithError(err, "resources.Client", "GetByID", nil, "Failure preparing request")
		return
	}
	setAPIVersion(req, apiVersion)

	resp, err := client.GetByIDSender(req)
	if err != nil {
		result.Response = autorest.Response{Response: resp}
		err = autorest.NewErrorWithError(err, "resources.Client", "GetByID", resp, "Failure sending request")
		return
	}

	result, err = client.GetByIDResponder(resp)
	if err != nil {
		err = autorest.NewErrorWithError(err, "resources.Client", "GetByID", resp, "Failure responding to request")
	}

	return
}

// CreateOrUpdateByID creates a resource.
//
// See: resources.Client.CreateOrUpdateByID.
func (client ResourcesClient) CreateOrUpdateByID(ctx context.Context, resourceID string, parameters resources.GenericResource, apiVersion string) (result resources.CreateOrUpdateByIDFuture, err error) {
	if tracing.IsEnabled() {
		ctx = tracing.StartSpan(ctx, fqdn+"/Client.CreateOrUpdateByID")
		defer func() {
			sc := -1
			if result.Response() != nil {
				sc = result.Response().StatusCode
			}
			tracing.EndSpan(ctx, sc, err)
		}()
	}
	if err := validation.Validate([]validation.Validation{
		{TargetValue: parameters,
			Constraints: []validation.Constraint{{Target: "parameters.Kind", Name: validation.Null, Rule: false,
				Chain: []validation.Constraint{{Target: "parameters.Kind", Name: validation.Pattern, Rule: `^[-\w\._,\(\)]+$`, Chain: nil}}}}}}); err != nil {
		return result, validation.NewError("resources.Client", "CreateOrUpdateByID", err.Error())
	}

	req, err := client.CreateOrUpdateByIDPreparer(ctx, resourceID, parameters)
	if err != nil {
		err = autorest.NewErrorWithError(err, "resources.Client", "CreateOrUpdateByID", nil, "Failure preparing request")
		return
	}
	setAPIVersion(req, apiVersion)

	result, err = client.CreateOrUpdateByIDSender(req)
	if err != nil {
		err = autorest.NewErrorWithError(err, "resources.Client", "CreateOrUpdateByID", result.Response(), "Failure sending request")
		return
	}

	return
}

func setAPIVersion(req *http.Request, apiVersion string) {
	// Replace the API version.
	query := req.URL.Query()
	query.Set("api-version", apiVersion)
	req.URL.RawQuery = query.Encode()
}
