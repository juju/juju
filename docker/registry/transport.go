// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package registry

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/juju/docker"
)

type basicTransport struct {
	transport   http.RoundTripper
	repoDetails *docker.ImageRepoDetails
}

func newBasicTransport(transport http.RoundTripper, repoDetails *docker.ImageRepoDetails) http.RoundTripper {
	return &basicTransport{
		transport:   transport,
		repoDetails: repoDetails,
	}
}

func (t basicTransport) username() string {
	return t.repoDetails.Username
}

func (t basicTransport) password() string {
	return t.repoDetails.Password
}

func (t basicTransport) authToken() string {
	return t.repoDetails.Auth
}

func (basicTransport) scheme() string {
	return "Basic"
}

func (t basicTransport) authorizeRequest(req *http.Request) error {
	if t.authToken() != "" {
		req.Header.Set(t.scheme(), t.authToken())
		return nil
	}
	if t.username() != "" || t.password() != "" {
		req.SetBasicAuth(t.username(), t.password())
		return nil
	}
	return errors.NotValidf("no basic auth credentials for %q", t.repoDetails.Repository)
}

func (t basicTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.authorizeRequest(req); err != nil {
		return nil, errors.Trace(err)
	}
	resp, err := t.transport.RoundTrip(req)
	return resp, errors.Trace(err)
}

type registryTokenTransport struct {
	transport   http.RoundTripper
	repoDetails *docker.ImageRepoDetails
}

func newRegistryTokenTransport(
	transport http.RoundTripper, repoDetails *docker.ImageRepoDetails,
) http.RoundTripper {
	return &registryTokenTransport{
		transport:   transport,
		repoDetails: repoDetails,
	}
}

func (t registryTokenTransport) registryToken() string {
	return t.repoDetails.RegistryToken
}

func (registryTokenTransport) scheme() string {
	return "Bearer"
}

func (t *registryTokenTransport) authorizeRequest(req *http.Request) error {
	req.Header.Set("Authorization", fmt.Sprintf("%s %s", t.scheme(), t.registryToken()))
	return nil
}

func (t registryTokenTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.authorizeRequest(req); err != nil {
		return nil, errors.Trace(err)
	}
	resp, err := t.transport.RoundTrip(req)
	return resp, errors.Trace(err)
}

type errorTransport struct {
	transport http.RoundTripper
}

func (t errorTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	resp, err := t.transport.RoundTrip(request)
	if err != nil {
		return resp, err
	}

	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, errors.Annotatef(err, "reading bad response body with status code %d", resp.StatusCode)
		}

		return nil, errors.New(fmt.Sprintf("non-successful response (status=%d body=%q)", resp.StatusCode, body))
	}

	return resp, err
}

func wrapTransport(r *registry) error {
	defer func() {
		r.client.Transport = errorTransport{r.client.Transport}
	}()

	transport := r.client.Transport
	if !r.repoDetails.BasicAuthConfig.Empty() {
		r.client.Transport = newBasicTransport(transport, r.repoDetails)
		return nil
	}
	if r.repoDetails.TokenAuthConfig.Empty() {
		return nil
	}
	if r.repoDetails.TokenAuthConfig.RegistryToken != "" {
		r.client.Transport = newRegistryTokenTransport(transport, r.repoDetails)
		return nil
	}
	if r.repoDetails.TokenAuthConfig.IdentityToken != "" {
		return errors.NotSupportedf("IdentityToken for %q", r.repoDetails.ServerAddress)
		// TODO: implement identityTokenTransport.
		// r.client.Transport = identityTokenTransport{
		// 	transport:   transport,
		// 	repoDetails: r.repoDetails,
		// }
	}
	return nil
}
