// NOTE(axw) this file contains a client for a subset of the
// Microsoft Graph API, which is not currently supported by
// the Azure SDK. When it is, this will be deleted.

package ad

import (
	"net/http"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"
)

type ApplicationsClient struct {
	ManagementClient
}

func (client ApplicationsClient) CreateOrUpdate(parameters Application, cancel <-chan struct{}) (result autorest.Response, err error) {
	req, err := client.CreateOrUpdatePreparer(parameters, cancel)
	if err != nil {
		return result, autorest.NewErrorWithError(err, "ad.ApplicationsClient", "CreateOrUpdate", nil, "Failure preparing request")
	}

	resp, err := client.CreateOrUpdateSender(req)
	if err != nil {
		result.Response = resp
		return result, autorest.NewErrorWithError(err, "ad.ApplicationsClient", "CreateOrUpdate", nil, "Failure sending request")
	}

	result, err = client.CreateOrUpdateResponder(resp)
	if err != nil {
		err = autorest.NewErrorWithError(err, "ad.ApplicationsClient", "CreateOrUpdate", nil, "Failure responding to request")
	}

	return
}

func (client ApplicationsClient) CreateOrUpdatePreparer(parameters Application, cancel <-chan struct{}) (*http.Request, error) {
	queryParameters := map[string]interface{}{
		"api-version": client.APIVersion,
	}

	preparer := autorest.CreatePreparer(
		autorest.AsJSON(),
		autorest.AsPost(),
		autorest.WithBaseURL(client.BaseURI),
		autorest.WithPath("/applications"),
		autorest.WithJSON(parameters),
		autorest.WithQueryParameters(queryParameters))
	return preparer.Prepare(&http.Request{Cancel: cancel})
}

func (client ApplicationsClient) CreateOrUpdateSender(req *http.Request) (*http.Response, error) {
	return autorest.SendWithSender(client,
		req,
		azure.DoPollForAsynchronous(client.PollingDelay))
}

func (client ApplicationsClient) CreateOrUpdateResponder(resp *http.Response) (result autorest.Response, err error) {
	err = autorest.Respond(
		resp,
		client.ByInspecting(),
		azure.WithErrorUnlessStatusCode(http.StatusOK, http.StatusCreated),
		autorest.ByClosing())
	result.Response = resp
	return
}

func (client ApplicationsClient) List(filter string) (result ApplicationListResult, err error) {
	req, err := client.ListPreparer(filter)
	if err != nil {
		return result, autorest.NewErrorWithError(err, "ad.ApplicationsClient", "List", nil, "Failure preparing request")
	}

	resp, err := client.ListSender(req)
	if err != nil {
		result.Response = autorest.Response{Response: resp}
		return result, autorest.NewErrorWithError(err, "ad.ApplicationsClient", "List", nil, "Failure sending request")
	}

	result, err = client.ListResponder(resp)
	if err != nil {
		err = autorest.NewErrorWithError(err, "ad.ApplicationsClient", "List", nil, "Failure responding to request")
	}

	return
}

func (client ApplicationsClient) ListPreparer(filter string) (*http.Request, error) {
	queryParameters := map[string]interface{}{
		"api-version": client.APIVersion,
	}
	if filter != "" {
		queryParameters["$filter"] = autorest.Encode("query", filter)
	}

	preparer := autorest.CreatePreparer(
		autorest.AsGet(),
		autorest.WithBaseURL(client.BaseURI),
		autorest.WithPath("/applications"),
		autorest.WithQueryParameters(queryParameters))
	return preparer.Prepare(&http.Request{})
}

func (client ApplicationsClient) ListSender(req *http.Request) (*http.Response, error) {
	return autorest.SendWithSender(client, req)
}

func (client ApplicationsClient) ListResponder(resp *http.Response) (result ApplicationListResult, err error) {
	err = autorest.Respond(
		resp,
		client.ByInspecting(),
		azure.WithErrorUnlessStatusCode(http.StatusOK),
		autorest.ByUnmarshallingJSON(&result),
		autorest.ByClosing())
	result.Response = autorest.Response{Response: resp}
	return
}

func (client ApplicationsClient) SetPassword(objectId string, parameters PasswordCredential) (result autorest.Response, err error) {
	req, err := client.SetPasswordPreparer(objectId, parameters)
	if err != nil {
		return result, autorest.NewErrorWithError(err, "ad.ApplicationsClient", "SetPassword", nil, "Failure preparing request")
	}

	resp, err := client.SetPasswordSender(req)
	if err != nil {
		result.Response = resp
		return result, autorest.NewErrorWithError(err, "ad.ApplicationsClient", "SetPassword", nil, "Failure sending request")
	}

	result, err = client.SetPasswordResponder(resp)
	if err != nil {
		err = autorest.NewErrorWithError(err, "ad.ApplicationsClient", "SetPassword", nil, "Failure responding to request")
	}

	return
}

func (client ApplicationsClient) SetPasswordPreparer(objectId string, parameters PasswordCredential) (*http.Request, error) {
	pathParameters := map[string]interface{}{
		"objectId": autorest.Encode("path", objectId),
	}

	queryParameters := map[string]interface{}{
		"api-version": client.APIVersion,
	}

	preparer := autorest.CreatePreparer(
		autorest.AsJSON(),
		autorest.AsPatch(),
		autorest.WithBaseURL(client.BaseURI),
		autorest.WithPathParameters("/applications/{objectId}", pathParameters),
		autorest.WithJSON(Application{
			PasswordCredentials: []PasswordCredential{parameters},
		}),
		autorest.WithQueryParameters(queryParameters))
	return preparer.Prepare(&http.Request{})
}

func (client ApplicationsClient) SetPasswordSender(req *http.Request) (*http.Response, error) {
	return autorest.SendWithSender(client,
		req,
		azure.DoPollForAsynchronous(client.PollingDelay))
}

func (client ApplicationsClient) SetPasswordResponder(resp *http.Response) (result autorest.Response, err error) {
	err = autorest.Respond(
		resp,
		client.ByInspecting(),
		azure.WithErrorUnlessStatusCode(http.StatusOK, http.StatusNoContent),
		autorest.ByClosing())
	result.Response = resp
	return
}
