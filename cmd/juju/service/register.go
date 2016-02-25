// Copyright 2015 Canonical Ltd. All rights reserved.

package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/charms"
)

type metricRegistrationPost struct {
	ModelUUID   string `json:"env-uuid"`
	CharmURL    string `json:"charm-url"`
	ServiceName string `json:"service-name"`
	PlanURL     string `json:"plan-url"`
	Budget      string `json:"budget"`
	Limit       string `json:"limit"`
}

// RegisterMeteredCharm implements the DeployStep interface.
type RegisterMeteredCharm struct {
	AllocateBudget
	Plan        string
	RegisterURL string
	QueryURL    string
	credentials []byte
}

func (r *RegisterMeteredCharm) SetFlags(f *gnuflag.FlagSet) {
	r.AllocateBudget.SetFlags(f)
	f.StringVar(&r.Plan, "plan", "", "plan to deploy charm under")
}

// RunPre obtains authorization to deploy this charm. The authorization, if received is not
// sent to the controller, rather it is kept as an attribute on RegisterMeteredCharm.
func (r *RegisterMeteredCharm) RunPre(state api.Connection, client *http.Client, ctx *cmd.Context, deployInfo DeploymentInfo) error {
	err := r.AllocateBudget.RunPre(state, client, ctx, deployInfo)
	if err != nil {
		return errors.Annotate(err, "failed to register metrics")
	}
	charmsClient := charms.NewClient(state)
	metered, err := charmsClient.IsMetered(deployInfo.CharmURL.String())
	if err != nil {
		return err
	}
	if !metered {
		return nil
	}

	bakeryClient := httpbakery.Client{Client: client, VisitWebPage: httpbakery.OpenWebBrowser}

	if r.Plan == "" && deployInfo.CharmURL.Schema == "cs" {
		r.Plan, err = r.getDefaultPlan(client, deployInfo.CharmURL.String())
		if err != nil {
			if isNoDefaultPlanError(err) {
				options, err1 := r.getCharmPlans(client, deployInfo.CharmURL.String())
				if err1 != nil {
					return err1
				}
				charmUrl := deployInfo.CharmURL.String()
				return errors.Errorf(`%v has no default plan. Try "juju deploy --plan <plan-name> with one of %v"`, charmUrl, strings.Join(options, ", "))
			}
			return err
		}
	}

	r.credentials, err = r.registerMetrics(
		deployInfo.ModelUUID,
		deployInfo.CharmURL.String(),
		deployInfo.ServiceName,
		r.AllocateBudget.Budget,
		r.AllocateBudget.Limit,
		&bakeryClient)
	if err != nil {
		if deployInfo.CharmURL.Schema == "cs" {
			logger.Infof("failed to obtain plan authorization: %v", err)
			return err
		}
		logger.Debugf("no plan authorization: %v", err)
	}
	return nil
}

// RunPost sends credentials obtained during the call to RunPre to the controller.
func (r *RegisterMeteredCharm) RunPost(state api.Connection, client *http.Client, ctx *cmd.Context, deployInfo DeploymentInfo, prevErr error) error {
	err := r.AllocateBudget.RunPost(state, client, ctx, deployInfo, prevErr)
	if err != nil {
		return errors.Trace(err)
	}
	if prevErr != nil {
		return nil
	}
	if r.credentials == nil {
		return nil
	}
	api, cerr := getMetricCredentialsAPI(state)
	if cerr != nil {
		logger.Infof("failed to get the metrics credentials setter: %v", cerr)
		return cerr
	}
	defer api.Close()

	err = api.SetMetricCredentials(deployInfo.ServiceName, r.credentials)
	if err != nil {
		logger.Infof("failed to set metric credentials: %v", err)
		return err
	}

	return nil
}

type noDefaultPlanError struct {
	cUrl string
}

func (e *noDefaultPlanError) Error() string {
	return fmt.Sprintf("%v has no default plan", e.cUrl)
}

func isNoDefaultPlanError(e error) bool {
	_, ok := e.(*noDefaultPlanError)
	return ok
}

func (r *RegisterMeteredCharm) getDefaultPlan(client *http.Client, cURL string) (string, error) {
	if r.QueryURL == "" {
		return "", errors.Errorf("no plan query url specified")
	}

	qURL, err := url.Parse(r.QueryURL + "/default")
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

	if response.StatusCode == http.StatusNotFound {
		return "", &noDefaultPlanError{cURL}
	}
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

func (r *RegisterMeteredCharm) getCharmPlans(client *http.Client, cURL string) ([]string, error) {
	if r.QueryURL == "" {
		return nil, errors.Errorf("no plan query url specified")
	}
	qURL, err := url.Parse(r.QueryURL)
	if err != nil {
		return nil, errors.Trace(err)
	}

	query := qURL.Query()
	query.Set("charm-url", cURL)
	qURL.RawQuery = query.Encode()

	req, err := http.NewRequest("GET", qURL.String(), nil)
	if err != nil {
		return nil, errors.Trace(err)
	}

	response, err := client.Do(req)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, errors.Errorf("failed to query plans: http response is %d", response.StatusCode)
	}

	var planInfo []struct {
		URL string `json:"url"`
	}
	dec := json.NewDecoder(response.Body)
	err = dec.Decode(&planInfo)
	if err != nil {
		return nil, errors.Trace(err)
	}
	info := make([]string, len(planInfo))
	for i, p := range planInfo {
		info[i] = p.URL
	}
	return info, nil
}

func (r *RegisterMeteredCharm) registerMetrics(environmentUUID, charmURL, serviceName, budget, limit string, client *httpbakery.Client) ([]byte, error) {
	if r.RegisterURL == "" {
		return nil, errors.Errorf("no metric registration url is specified")
	}
	registerURL, err := url.Parse(r.RegisterURL)
	if err != nil {
		return nil, errors.Trace(err)
	}

	registrationPost := metricRegistrationPost{
		ModelUUID:   environmentUUID,
		CharmURL:    charmURL,
		ServiceName: serviceName,
		PlanURL:     r.Plan,
		Budget:      budget,
		Limit:       limit,
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
