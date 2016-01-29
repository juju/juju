// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/romulus/api/budget"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/charms"
	"github.com/juju/juju/apiserver/params"
)

var budgetWithLimitRe = regexp.MustCompile(`^[a-zA-Z0-9\-]+:[1-9][0-9]*$`)

// AllocateBudget implements the DeployStep interface.
type AllocateBudget struct {
	AllocationSpec string
	APIClient      apiClient
	allocated      bool
}

// SetFlags is part of the DeployStep interface.
func (a *AllocateBudget) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&a.AllocationSpec, "budget", "", "budget and allocation limit")
}

// RunPre is part of the DeployStep interface.
func (a *AllocateBudget) RunPre(state api.Connection, client *http.Client, ctx *cmd.Context, deployInfo DeploymentInfo) error {
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

	allocBudget, allocLimit, err := parseBudgetWithLimit(a.AllocationSpec)
	if err != nil {
		return errors.Trace(err)
	}
	a.APIClient, err = getApiClient(client)
	if err != nil {
		return errors.Trace(err)
	}
	resp, err := a.APIClient.CreateAllocation(allocBudget, allocLimit, deployInfo.EnvUUID, []string{deployInfo.ServiceName})
	if err != nil {
		return errors.Annotate(err, "could not create budget allocation")
	}
	a.allocated = true
	fmt.Fprintf(ctx.Stdout, "%s\n", resp)
	return nil
}

func (a *AllocateBudget) RunPost(_ api.Connection, client *http.Client, ctx *cmd.Context, deployInfo DeploymentInfo, prevErr error) error {
	if prevErr == nil || !a.allocated {
		return nil
	}
	var err error
	if a.APIClient == nil {
		a.APIClient, err = getApiClient(client)
		if err != nil {
			return errors.Trace(err)
		}
	}
	resp, err := a.APIClient.DeleteAllocation(deployInfo.EnvUUID, deployInfo.ServiceName)
	if err != nil {
		return errors.Annotate(err, "failed to remove allocation")
	}
	fmt.Fprintf(ctx.Stdout, "%s\n", resp)
	return nil
}

func parseBudgetWithLimit(bl string) (string, string, error) {
	if !budgetWithLimitRe.MatchString(bl) {
		return "", "", errors.New("invalid budget specification, expecting <budget>:<limit>")
	}
	parts := strings.Split(bl, ":")
	return parts[0], parts[1], nil
}

var getApiClient = getApiClientImpl

func getApiClientImpl(client *http.Client) (apiClient, error) {
	bakeryClient := &httpbakery.Client{Client: client, VisitWebPage: httpbakery.OpenWebBrowser}
	c := budget.NewClient(bakeryClient)
	return c, nil
}

type apiClient interface {
	CreateAllocation(string, string, string, []string) (string, error)
	DeleteAllocation(string, string) (string, error)
}
