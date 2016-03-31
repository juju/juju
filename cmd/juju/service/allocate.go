// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/romulus/api/budget"
	wireformat "github.com/juju/romulus/wireformat/budget"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/charms"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/jujuclient"
)

var budgetWithLimitRe = regexp.MustCompile(`^[a-zA-Z0-9\-]+:[0-9]+$`)

// AllocateBudget implements the DeployStep interface.
type AllocateBudget struct {
	AllocationSpec string
	APIClient      apiClient
	allocated      bool
	Budget         string
	Limit          string
}

// SetFlags is part of the DeployStep interface.
func (a *AllocateBudget) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&a.AllocationSpec, "budget", "personal:0", "budget and allocation limit")
}

// RunPre is part of the DeployStep interface.
func (a *AllocateBudget) RunPre(state api.Connection, bakeryClient *httpbakery.Client, ctx *cmd.Context, deployInfo DeploymentInfo) error {
	if deployInfo.CharmURL.Schema == "local" {
		return nil
	}
	charmsClient := charms.NewClient(state)
	metered, err := charmsClient.IsMetered(deployInfo.CharmURL.String())
	if params.IsCodeNotImplemented(err) {
		// The state server is too old to support metering.  Warn
		// the user, but don't return an error.
		logger.Tracef("current state server version does not support charm metering")
		return nil
	} else if err != nil {
		return errors.Annotate(err, "could not determine charm type")
	}
	if !metered {
		return nil
	}

	allocBudget, allocLimit, err := parseBudgetWithLimit(a.AllocationSpec)
	if err != nil {
		return errors.Trace(err)
	}
	a.Budget, a.Limit = allocBudget, allocLimit
	a.APIClient, err = getApiClient(bakeryClient)
	if err != nil {
		return errors.Annotate(err, "could not create API client")
	}
	resp, err := a.APIClient.CreateAllocation(allocBudget, allocLimit, deployInfo.ModelUUID, []string{deployInfo.ServiceName})
	if err != nil {
		if wireformat.IsNotAvail(err) {
			fmt.Fprintf(ctx.Stdout, "WARNING: Budget allocation not created - %s.\n", err.Error())
			return nil
		}
		return errors.Annotate(err, "could not create budget allocation")
	}
	a.allocated = true
	fmt.Fprintf(ctx.Stdout, "%s\n", resp)
	return nil
}

func (a *AllocateBudget) RunPost(_ api.Connection, bclient *httpbakery.Client, ctx *cmd.Context, deployInfo DeploymentInfo, prevErr error) error {
	if prevErr == nil || !a.allocated {
		return nil
	}
	var err error
	if a.APIClient == nil {
		a.APIClient, err = getApiClient(bclient)
		if err != nil {
			return errors.Trace(err)
		}
	}
	resp, err := a.APIClient.DeleteAllocation(deployInfo.ModelUUID, deployInfo.ServiceName)
	if err != nil {
		return errors.Annotate(err, "failed to remove allocation")
	}
	fmt.Fprintf(ctx.Stdout, "%s\n", resp)
	return nil
}

func parseBudgetWithLimit(bl string) (string, string, error) {
	if !budgetWithLimitRe.MatchString(bl) {
		return "", "", errors.New("invalid allocation, expecting <budget>:<limit>")
	}
	parts := strings.Split(bl, ":")
	return parts[0], parts[1], nil
}

var getApiClient = getApiClientImpl

var tokenStore = jujuclient.NewTokenStore

func getApiClientImpl(bclient *httpbakery.Client) (apiClient, error) {
	return budget.NewClient(bclient), nil
}

type apiClient interface {
	CreateAllocation(string, string, string, []string) (string, error)
	DeleteAllocation(string, string) (string, error)
}
