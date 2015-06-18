// Copyright 2015 Canonical Ltd. All rights reserved.

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/juju/errors"
	"github.com/juju/persistent-cookiejar"
	"gopkg.in/macaroon-bakery.v0/httpbakery"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/charms"
)

var registerMetricsURL = ""

type metricRegistrationPost struct {
	EnvironmentUUID string `json:"env-uuid"`
	CharmURL        string `json:"charm-url"`
	ServiceName     string `json:"service-name"`
}

func registerMeteredCharm(state *api.State, jar *cookiejar.Jar, charmURL string, serviceName, environmentUUID string) error {
	charmsClient := charms.NewClient(state)
	defer charmsClient.Close()
	metered, err := charmsClient.IsMetered(charmURL)
	if err != nil {
		return err
	}
	if metered {
		httpClient := httpbakery.NewHTTPClient()
		httpClient.Jar = jar
		credentials, err := registerMetrics(environmentUUID, charmURL, serviceName, httpClient, httpbakery.OpenWebBrowser)
		if err != nil {
			logger.Infof("failed to register metrics: %v", err)
			return err
		}

		api, cerr := getMetricCredentialsAPI(state)
		if cerr != nil {
			logger.Infof("failed to get the metrics credentials setter: %v", cerr)
		}
		err = api.SetMetricCredentials(serviceName, credentials)
		if err != nil {
			logger.Infof("failed to set metric credentials: %v", err)
			return err
		}
		api.Close()
	}
	return nil
}

func registerMetrics(environmentUUID, charmURL, serviceName string, client *http.Client, visitWebPage func(*url.URL) error) ([]byte, error) {
	if registerMetricsURL == "" {
		return nil, errors.Errorf("no metric registration url is specified")
	}
	registerURL, err := url.Parse(registerMetricsURL)
	if err != nil {
		return nil, errors.Trace(err)
	}

	registrationPost := metricRegistrationPost{
		EnvironmentUUID: environmentUUID,
		CharmURL:        charmURL,
		ServiceName:     serviceName,
	}

	buff := &bytes.Buffer{}
	encoder := json.NewEncoder(buff)
	err = encoder.Encode(registrationPost)
	if err != nil {
		return nil, errors.Trace(err)
	}

	req, err := http.NewRequest("POST", registerURL.String(), nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	req.Header.Set("Content-Type", "application/json")

	bodyGetter := httpbakery.SeekerBody(bytes.NewReader(buff.Bytes()))

	response, err := httpbakery.DoWithBody(client, req, bodyGetter, visitWebPage)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer discardClose(response)

	if response.StatusCode != http.StatusOK {
		return nil, errors.Errorf("failed to register metrics: http response is %d", response.StatusCode)
	}

	var m *macaroon.Macaroon
	decoder := json.NewDecoder(response.Body)
	err = decoder.Decode(&m)
	if err != nil {
		return nil, errors.Annotatef(err, "failed to unmarshal the response")
	}

	ms := macaroon.Slice{m}
	return json.Marshal(ms)
}

// discardClose reads any remaining data from the response body and closes it.
func discardClose(response *http.Response) {
	if response == nil || response.Body == nil {
		return
	}
	io.Copy(ioutil.Discard, response.Body)
	response.Body.Close()
}
