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

func (client ServicePrincipalsClient) CreateOrUpdate(parameters ServicePrincipal, cancel <-chan struct{}) (result autorest.Response, err error) {
	req, err := client.CreateOrUpdatePreparer(parameters, cancel)
	if err != nil {
		return result, autorest.NewErrorWithError(err, "ad.ServicePrincipalsClient", "CreateOrUpdate", nil, "Failure preparing request")
	}

	resp, err := client.CreateOrUpdateSender(req)
	if err != nil {
		result.Response = resp
		return result, autorest.NewErrorWithError(err, "ad.ServicePrincipalsClient", "CreateOrUpdate", nil, "Failure sending request")
	}

	result, err = client.CreateOrUpdateResponder(resp)
	if err != nil {
		err = autorest.NewErrorWithError(err, "ad.ServicePrincipalsClient", "CreateOrUpdate", nil, "Failure responding to request")
	}

	return
}

func (client ServicePrincipalsClient) CreateOrUpdatePreparer(parameters ServicePrincipal, cancel <-chan struct{}) (*http.Request, error) {
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

func (client ServicePrincipalsClient) CreateOrUpdateSender(req *http.Request) (*http.Response, error) {
	return autorest.SendWithSender(client,
		req,
		azure.DoPollForAsynchronous(client.PollingDelay))
}

func (client ServicePrincipalsClient) CreateOrUpdateResponder(resp *http.Response) (result autorest.Response, err error) {
	err = autorest.Respond(
		resp,
		client.ByInspecting(),
		azure.WithErrorUnlessStatusCode(http.StatusOK, http.StatusCreated),
		autorest.ByClosing())
	result.Response = resp
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
		azure.WithErrorUnlessStatusCode(http.StatusOK),
		autorest.ByUnmarshallingJSON(&result),
		autorest.ByClosing())
	result.Response = autorest.Response{Response: resp}
	return
}
