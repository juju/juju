// This file is based on code from Azure/azure-sdk-for-go,
// which is Copyright Microsoft Corporation. See the LICENSE
// file in this directory for details.
//
// NOTE(axw) this file contains a client for a subset of the
// Microsoft Graph API, which is not currently supported by
// the Azure SDK. When it is, this will be deleted.

package ad

import (
	"net/http"

	"github.com/Azure/go-autorest/autorest"
)

type UsersClient struct {
	ManagementClient
}

func (client UsersClient) GetCurrentUser() (result AADObject, err error) {
	req, err := client.GetCurrentUserPreparer()
	if err != nil {
		return result, autorest.NewErrorWithError(err, "ad.UsersClient", "GetCurrentUser", nil, "Failure preparing request")
	}

	resp, err := client.GetCurrentUserSender(req)
	if err != nil {
		result.Response = autorest.Response{Response: resp}
		return result, autorest.NewErrorWithError(err, "ad.UsersClient", "GetCurrentUser", nil, "Failure sending request")
	}

	result, err = client.GetCurrentUserResponder(resp)
	if err != nil {
		err = autorest.NewErrorWithError(err, "ad.UsersClient", "GetCurrentUser", nil, "Failure responding to request")
	}

	return
}

func (client UsersClient) GetCurrentUserPreparer() (*http.Request, error) {
	queryParameters := map[string]interface{}{
		"api-version": client.APIVersion,
	}

	preparer := autorest.CreatePreparer(
		autorest.AsGet(),
		autorest.WithBaseURL(client.BaseURI),
		autorest.WithPath("/me"),
		autorest.WithQueryParameters(queryParameters))
	return preparer.Prepare(&http.Request{})
}

func (client UsersClient) GetCurrentUserSender(req *http.Request) (*http.Response, error) {
	return autorest.SendWithSender(client, req)
}

func (client UsersClient) GetCurrentUserResponder(resp *http.Response) (result AADObject, err error) {
	err = autorest.Respond(
		resp,
		client.ByInspecting(),
		WithOdataErrorUnlessStatusCode(http.StatusOK),
		autorest.ByUnmarshallingJSON(&result),
		autorest.ByClosing())
	result.Response = autorest.Response{Response: resp}
	return
}
