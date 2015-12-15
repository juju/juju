// Copyright 2015 Canonical Ltd. All rights reserved.

package commands

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/juju/errors"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/charms"
	"github.com/juju/juju/apiserver/params"
)

type metricRegistrationPost struct {
	EnvironmentUUID string `json:"env-uuid"`
	CharmURL        string `json:"charm-url"`
	ServiceName     string `json:"service-name"`
	PlanURL         string `json:"plan-url"`
}

// RegisterMeteredCharm implements the DeployStep interface.
type RegisterMeteredCharm struct {
	Plan        string
	RegisterURL string
	QueryURL    string
}

func (r *RegisterMeteredCharm) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&r.Plan, "plan", "", "plan to deploy charm under")
}

func (r *RegisterMeteredCharm) Run(state api.Connection, client *http.Client, deployInfo DeploymentInfo) error {
	charmsClient := charms.NewClient(state)
	defer charmsClient.Close()
	metered, err := charmsClient.IsMetered(deployInfo.CharmURL.String())
	if params.IsCodeNotImplemented(err) {
		// The state server is too old to support metering.  Warn
		// the user, but don't return an error.
		logger.Tracef("current state server version does not support charm metering")
		return nil
	} else if err != nil {
		return err
	}
	if !metered {
		return nil
	}

	bakeryClient := httpbakery.Client{Client: client, VisitWebPage: httpbakery.OpenWebBrowser}

	if r.Plan == "" {
		r.Plan, err = r.getDefaultPlan(client, deployInfo.CharmURL.String())
		if err != nil {
			logger.Errorf("could not determine default plan")
			return err
		}
	}

	credentials, err := r.registerMetrics(deployInfo.EnvUUID, deployInfo.CharmURL.String(), deployInfo.ServiceName, &bakeryClient)
	if err != nil {
		logger.Infof("failed to obtain plan authorization: %v", err)
		return err
	}

	api, cerr := getMetricCredentialsAPI(state)
	if cerr != nil {
		logger.Infof("failed to get the metrics credentials setter: %v", cerr)
		return cerr
	}
	defer api.Close()

	err = api.SetMetricCredentials(deployInfo.ServiceName, credentials)
	if params.IsCodeNotImplemented(err) {
		// The state server is too old to support metering.  Warn
		// the user, but don't return an error.
		logger.Warningf("current state server version does not support charm metering")
		return nil
	} else if err != nil {
		logger.Infof("failed to set metric credentials: %v", err)
		return err
	}

	return nil
}

func (r *RegisterMeteredCharm) getDefaultPlan(client *http.Client, cURL string) (string, error) {
	if r.QueryURL == "" {
		return "", errors.Errorf("no plan query url specified")
	}
	qURL, err := url.Parse(r.RegisterURL)
	if err != nil {
		return "", errors.Trace(err)
	}

	query := qURL.Query()
	query.Set("charm-url", cURL)
	qURL.RawQuery = query.Encode()

	req, err := http.NewRequest("GET", qURL.String(), nil)
	if err != nil {
		return "", errors.Trace(err)
	}

	response, err := client.Do(req)
	if err != nil {
		return "", errors.Trace(err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", errors.Errorf("failed to query default plan: http response is %d", response.StatusCode)
	}

	var planInfo struct {
		URL string `json:"url"`
	}
	dec := json.NewDecoder(response.Body)
	err = dec.Decode(&planInfo)
	if err != nil {
		return "", errors.Trace(err)
	}
	return planInfo.URL, nil
}

func (r *RegisterMeteredCharm) registerMetrics(environmentUUID, charmURL, serviceName string, client *httpbakery.Client) ([]byte, error) {
	if r.RegisterURL == "" {
		return nil, errors.Errorf("no metric registration url is specified")
	}
	registerURL, err := url.Parse(r.RegisterURL)
	if err != nil {
		return nil, errors.Trace(err)
	}

	registrationPost := metricRegistrationPost{
		EnvironmentUUID: environmentUUID,
		CharmURL:        charmURL,
		ServiceName:     serviceName,
		PlanURL:         r.Plan,
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

	response, err := client.DoWithBody(req, bytes.NewReader(buff.Bytes()))
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, errors.Errorf("failed to register metrics: http response is %d", response.StatusCode)
	}

	b, err := ioutil.ReadAll(response.Body)
	if err != nil {
		return nil, errors.Annotatef(err, "failed to read the response")
	}
	return b, nil
}
