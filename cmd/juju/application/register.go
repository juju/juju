// Copyright 2015 Canonical Ltd. All rights reserved.

package application

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
	"github.com/juju/gnuflag"
	"gopkg.in/macaroon-bakery.v2-unstable/httpbakery"
)

type metricRegistrationPost struct {
	ModelUUID       string `json:"env-uuid"`
	CharmURL        string `json:"charm-url"`
	ApplicationName string `json:"service-name"`
	PlanURL         string `json:"plan-url"`
	IncreaseBudget  int    `json:"increase-budget"`
}

// RegisterMeteredCharm implements the DeployStep interface.
type RegisterMeteredCharm struct {
	Plan           string
	IncreaseBudget int
	RegisterPath   string
	QueryPath      string
	PlanURL        string
	credentials    []byte
}

// SetFlags implements DeployStep.
func (r *RegisterMeteredCharm) SetFlags(f *gnuflag.FlagSet) {
	f.IntVar(&r.IncreaseBudget, "increase-budget", 0, "increase model budget allocation by this amount")
	f.StringVar(&r.Plan, "plan", "", "plan to deploy charm under")
}

// SetPlanURL implements DeployStep.
func (r *RegisterMeteredCharm) SetPlanURL(planURL string) {
	r.PlanURL = planURL
}

// RunPre obtains authorization to deploy this charm. The authorization, if received is not
// sent to the controller, rather it is kept as an attribute on RegisterMeteredCharm.
func (r *RegisterMeteredCharm) RunPre(api DeployStepAPI, bakeryClient *httpbakery.Client, ctx *cmd.Context, deployInfo DeploymentInfo) error {
	if r.IncreaseBudget < 0 {
		return errors.Errorf("invalid budget increase %d", r.IncreaseBudget)
	}
	var err error
	// if the deployInfo specifies an application plan it means
	// that it is to be deployed as part of the bundle and should
	// be deployed using the specified plan: the bundle author
	// clearly marked it as a metered application so there's no need
	// to check.
	if deployInfo.ApplicationPlan != "" {
		r.Plan = deployInfo.ApplicationPlan
		if r.Plan == "default" {
			r.Plan, err = r.getDefaultPlan(bakeryClient, deployInfo.CharmID.URL.String())
			if err != nil {
				return errors.Trace(err)
			}
		}
	} else {
		// otherwise we check if the charm is metered
		metered, err := api.IsMetered(deployInfo.CharmID.URL.String())
		if err != nil {
			return errors.Trace(err)
		}
		if !metered {
			return nil
		}

		info := deployInfo.CharmInfo
		if r.Plan == "" && info.Metrics != nil && !info.Metrics.PlanRequired() {
			return nil
		}

		// if the plan was not specified and this is a charmstore charm we
		// check if the charm has a default plan
		if r.Plan == "" && deployInfo.CharmID.URL.Schema == "cs" {
			r.Plan, err = r.getDefaultPlan(bakeryClient, deployInfo.CharmID.URL.String())
			if err != nil {
				if isNoDefaultPlanError(err) {
					options, err1 := r.getCharmPlans(bakeryClient, deployInfo.CharmID.URL.String())
					if err1 != nil {
						return err1
					}
					charmURL := deployInfo.CharmID.URL.String()
					if len(options) > 0 {
						return errors.Errorf(`%v has no default plan. Try "juju deploy --plan <plan-name> with one of %v"`, charmURL, strings.Join(options, ", "))
					} else {
						return errors.Errorf("no plans available for %v.", charmURL)
					}
				}
				return err
			}
		}
	}

	r.credentials, err = r.registerMetrics(
		deployInfo.ModelUUID,
		deployInfo.CharmID.URL.String(),
		deployInfo.ApplicationName,
		bakeryClient,
	)
	if err != nil {
		if deployInfo.CharmID.URL.Schema == "cs" {
			logger.Infof("failed to obtain plan authorization: %v", err)
			return err
		}
		logger.Debugf("no plan authorization: %v", err)
	}
	return nil
}

// RunPost sends credentials obtained during the call to RunPre to the controller.
func (r *RegisterMeteredCharm) RunPost(api DeployStepAPI, bakeryClient *httpbakery.Client, ctx *cmd.Context, deployInfo DeploymentInfo, prevErr error) error {
	if prevErr != nil {
		return nil
	}
	if r.credentials == nil {
		return nil
	}
	err := api.SetMetricCredentials(deployInfo.ApplicationName, r.credentials)
	if err != nil {
		logger.Warningf("failed to set metric credentials: %v", err)
		return errors.Trace(err)
	}

	return nil
}

type noDefaultPlanError struct {
	cURL string
}

func (e *noDefaultPlanError) Error() string {
	return fmt.Sprintf("%v has no default plan", e.cURL)
}

func isNoDefaultPlanError(e error) bool {
	_, ok := e.(*noDefaultPlanError)
	return ok
}

func (r *RegisterMeteredCharm) getDefaultPlan(client *httpbakery.Client, cURL string) (string, error) {
	if r.PlanURL == "" {
		return "", errors.Errorf("no plan query url specified")
	}
	qURL, err := url.Parse(r.PlanURL + r.QueryPath + "/default")
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

func (r *RegisterMeteredCharm) getCharmPlans(client *httpbakery.Client, cURL string) ([]string, error) {
	if r.PlanURL == "" {
		return nil, errors.Errorf("no plan query url specified")
	}
	qURL, err := url.Parse(r.PlanURL + r.QueryPath)
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

func (r *RegisterMeteredCharm) registerMetrics(modelUUID, charmURL, applicationName string, client *httpbakery.Client) ([]byte, error) {
	if r.PlanURL == "" {
		return nil, errors.Errorf("no plan query url specified")
	}
	registerURL, err := url.Parse(r.PlanURL + r.RegisterPath)
	if err != nil {
		return nil, errors.Trace(err)
	}

	registrationPost := metricRegistrationPost{
		ModelUUID:       modelUUID,
		CharmURL:        charmURL,
		ApplicationName: applicationName,
		PlanURL:         r.Plan,
		IncreaseBudget:  r.IncreaseBudget,
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

	if response.StatusCode == http.StatusOK {
		b, err := ioutil.ReadAll(response.Body)
		if err != nil {
			return nil, errors.Annotatef(err, "failed to read the response")
		}
		return b, nil
	}
	var respError struct {
		Error string `json:"error"`
	}
	err = json.NewDecoder(response.Body).Decode(&respError)
	if err != nil {
		return nil, errors.Errorf("authorization failed: http response is %d", response.StatusCode)
	}
	return nil, errors.Errorf("authorization failed: %s", respError.Error)
}
