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
	"github.com/Azure/go-autorest/autorest/azure"
)

type ServicePrincipalsClient struct {
	ManagementClient
}

func (client ServicePrincipalsClient) Create(parameters ServicePrincipalCreateParameters, cancel <-chan struct{}) (result ServicePrincipal, err error) {
	req, err := client.CreatePreparer(parameters, cancel)
	if err != nil {
		return result, autorest.NewErrorWithError(err, "ad.ServicePrincipalsClient", "Create", nil, "Failure preparing request")
	}

	resp, err := client.CreateSender(req)
	if err != nil {
		result.Response = autorest.Response{Response: resp}
		return result, autorest.NewErrorWithError(err, "ad.ServicePrincipalsClient", "Create", nil, "Failure sending request")
	}

	result, err = client.CreateResponder(resp)
	if err != nil {
		err = autorest.NewErrorWithError(err, "ad.ServicePrincipalsClient", "Create", nil, "Failure responding to request")
	}

	return
}

func (client ServicePrincipalsClient) CreatePreparer(parameters ServicePrincipalCreateParameters, cancel <-chan struct{}) (*http.Request, error) {
	queryParameters := map[string]interface{}{
		"api-version": client.APIVersion,
	}

	preparer := autorest.CreatePreparer(
		autorest.AsJSON(),
		autorest.AsPost(),
		autorest.WithBaseURL(client.BaseURI),
		autorest.WithPath("/servicePrincipals"),
		autorest.WithJSON(parameters),
		autorest.WithQueryParameters(queryParameters))
	return preparer.Prepare(&http.Request{Cancel: cancel})
}

func (client ServicePrincipalsClient) CreateSender(req *http.Request) (*http.Response, error) {
	return autorest.SendWithSender(client,
		req,
		azure.DoPollForAsynchronous(client.PollingDelay))
}

func (client ServicePrincipalsClient) CreateResponder(resp *http.Response) (result ServicePrincipal, err error) {
	err = autorest.Respond(
		resp,
		client.ByInspecting(),
		WithOdataErrorUnlessStatusCode(http.StatusOK, http.StatusCreated),
		autorest.ByUnmarshallingJSON(&result),
		autorest.ByClosing())
	result.Response = autorest.Response{Response: resp}
	return
}

func (client ServicePrincipalsClient) List(filter string) (result ServicePrincipalListResult, err error) {
	req, err := client.ListPreparer(filter)
	if err != nil {
		return result, autorest.NewErrorWithError(err, "ad.ServicePrincipalsClient", "List", nil, "Failure preparing request")
	}

	resp, err := client.ListSender(req)
	if err != nil {
		result.Response = autorest.Response{Response: resp}
		return result, autorest.NewErrorWithError(err, "ad.ServicePrincipalsClient", "List", nil, "Failure sending request")
	}

	result, err = client.ListResponder(resp)
	if err != nil {
		err = autorest.NewErrorWithError(err, "ad.ServicePrincipalsClient", "List", nil, "Failure responding to request")
	}

	return
}

func (client ServicePrincipalsClient) ListPreparer(filter string) (*http.Request, error) {
	queryParameters := map[string]interface{}{
		"api-version": client.APIVersion,
	}
	if filter != "" {
		queryParameters["$filter"] = autorest.Encode("query", filter)
	}

	preparer := autorest.CreatePreparer(
		autorest.AsGet(),
		autorest.WithBaseURL(client.BaseURI),
		autorest.WithPath("/servicePrincipals"),
		autorest.WithQueryParameters(queryParameters))
	return preparer.Prepare(&http.Request{})
}

func (client ServicePrincipalsClient) ListSender(req *http.Request) (*http.Response, error) {
	return autorest.SendWithSender(client, req)
}

func (client ServicePrincipalsClient) ListResponder(resp *http.Response) (result ServicePrincipalListResult, err error) {
	err = autorest.Respond(
		resp,
		client.ByInspecting(),
		WithOdataErrorUnlessStatusCode(http.StatusOK),
		autorest.ByUnmarshallingJSON(&result),
		autorest.ByClosing())
	result.Response = autorest.Response{Response: resp}
	return
}

func (client ServicePrincipalsClient) ListPasswordCredentials(objectId string) (result PasswordCredentialsListResult, err error) {
	req, err := client.ListPasswordCredentialsPreparer(objectId)
	if err != nil {
		return result, autorest.NewErrorWithError(err, "ad.ServicePrincipalsClient", "ListPasswordCredentials", nil, "Failure preparing request")
	}

	resp, err := client.ListPasswordCredentialsSender(req)
	if err != nil {
		result.Response = autorest.Response{Response: resp}
		return result, autorest.NewErrorWithError(err, "ad.ServicePrincipalsClient", "ListPasswordCredentials", nil, "Failure sending request")
	}

	result, err = client.ListPasswordCredentialsResponder(resp)
	if err != nil {
		err = autorest.NewErrorWithError(err, "ad.ServicePrincipalsClient", "ListPasswordCredentials", nil, "Failure responding to request")
	}

	return
}

func (client ServicePrincipalsClient) ListPasswordCredentialsPreparer(objectId string) (*http.Request, error) {
	pathParameters := map[string]interface{}{
		"objectId": autorest.Encode("path", objectId),
	}
	queryParameters := map[string]interface{}{
		"api-version": client.APIVersion,
	}

	preparer := autorest.CreatePreparer(
		autorest.AsGet(),
		autorest.WithBaseURL(client.BaseURI),
		autorest.WithPathParameters("/servicePrincipals/{objectId}/passwordCredentials", pathParameters),
		autorest.WithQueryParameters(queryParameters))
	return preparer.Prepare(&http.Request{})
}

func (client ServicePrincipalsClient) ListPasswordCredentialsSender(req *http.Request) (*http.Response, error) {
	return autorest.SendWithSender(client, req)
}

func (client ServicePrincipalsClient) ListPasswordCredentialsResponder(resp *http.Response) (result PasswordCredentialsListResult, err error) {
	err = autorest.Respond(
		resp,
		client.ByInspecting(),
		WithOdataErrorUnlessStatusCode(http.StatusOK),
		autorest.ByUnmarshallingJSON(&result),
		autorest.ByClosing())
	result.Response = autorest.Response{Response: resp}
	return
}

func (client ServicePrincipalsClient) UpdatePasswordCredentials(objectId string, parameters PasswordCredentialsUpdateParameters) (result autorest.Response, err error) {
	req, err := client.UpdatePasswordCredentialsPreparer(objectId, parameters)
	if err != nil {
		return result, autorest.NewErrorWithError(err, "ad.ServicePrincipalsClient", "UpdatePasswordCredentials", nil, "Failure preparing request")
	}

	resp, err := client.UpdatePasswordCredentialsSender(req)
	if err != nil {
		result.Response = resp
		return result, autorest.NewErrorWithError(err, "ad.ServicePrincipalsClient", "UpdatePasswordCredentials", nil, "Failure sending request")
	}

	result, err = client.UpdatePasswordCredentialsResponder(resp)
	if err != nil {
		err = autorest.NewErrorWithError(err, "ad.ServicePrincipalsClient", "UpdatePasswordCredentials", nil, "Failure responding to request")
	}

	return
}

func (client ServicePrincipalsClient) UpdatePasswordCredentialsPreparer(objectId string, parameters PasswordCredentialsUpdateParameters) (*http.Request, error) {
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
		autorest.WithPathParameters("/servicePrincipals/{objectId}/passwordCredentials", pathParameters),
		autorest.WithJSON(parameters),
		autorest.WithQueryParameters(queryParameters))
	return preparer.Prepare(&http.Request{})
}

func (client ServicePrincipalsClient) UpdatePasswordCredentialsSender(req *http.Request) (*http.Response, error) {
	return autorest.SendWithSender(client,
		req,
		azure.DoPollForAsynchronous(client.PollingDelay))
}

func (client ServicePrincipalsClient) UpdatePasswordCredentialsResponder(resp *http.Response) (result autorest.Response, err error) {
	err = autorest.Respond(
		resp,
		client.ByInspecting(),
		WithOdataErrorUnlessStatusCode(http.StatusOK, http.StatusNoContent),
		autorest.ByClosing())
	result.Response = resp
	return
}
