// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils"

	"github.com/juju/juju/api/block"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/network"
)

var (
	bootstrapReadyPollDelay = 1 * time.Second
	bootstrapReadyPollCount = 60
	blockAPI                = getBlockAPI
)

type listBlocksAPI interface {
	List() ([]params.Block, error)
	Close() error
}

// getBlockAPI returns a block api for listing blocks.
func getBlockAPI(c *modelcmd.ModelCommandBase) (listBlocksAPI, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return block.NewClient(root), nil
}

// tryAPI attempts to open the API and makes a trivial call
// to check if the API is available yet.
func tryAPI(c *modelcmd.ModelCommandBase) error {
	client, err := blockAPI(c)
	if err == nil {
		_, err = client.List()
		closeErr := client.Close()
		if closeErr != nil {
			logger.Debugf("Error closing client: %v", closeErr)
		}
	}
	return err
}

// WaitForAgentInitialisation polls the bootstrapped controller with a read-only
// command which will fail until the controller is fully initialised.
// TODO(wallyworld) - add a bespoke command to maybe the admin facade for this purpose.
func WaitForAgentInitialisation(ctx *cmd.Context, c *modelcmd.ModelCommandBase, controllerName, hostedModelName string) error {
	// TODO(katco): 2016-08-09: lp:1611427
	attempts := utils.AttemptStrategy{
		Min:   bootstrapReadyPollCount,
		Delay: bootstrapReadyPollDelay,
	}
	var (
		apiAttempts int
		err         error
	)

	// Make a best effort to find the new controller address so we can print it.
	addressInfo := ""
	controller, err := c.ClientStore().ControllerByName(controllerName)
	if err == nil && len(controller.APIEndpoints) > 0 {
		addr, err := network.ParseHostPort(controller.APIEndpoints[0])
		if err == nil {
			addressInfo = fmt.Sprintf(" at %s", addr.Address.Value)
		}
	}

	ctx.Infof("Contacting Juju controller%s to verify accessibility...", addressInfo)
	apiAttempts = 1
	for attempt := attempts.Start(); attempt.Next(); apiAttempts++ {
		err = tryAPI(c)
		if err == nil {
			ctx.Infof("Bootstrap complete, %q controller now available", controllerName)
			ctx.Infof("Controller machines are in the %q model", bootstrap.ControllerModelName)
			ctx.Infof("Initial model %q added", hostedModelName)
			break
		}
		// As the API server is coming up, it goes through a number of steps.
		// Initially the upgrade steps run, but the api server allows some
		// calls to be processed during the upgrade, but not the list blocks.
		// Logins are also blocked during space discovery.
		// It is also possible that the underlying database causes connections
		// to be dropped as it is initialising, or reconfiguring. These can
		// lead to EOF or "connection is shut down" error messages. We skip
		// these too, hoping that things come back up before the end of the
		// retry poll count.
		errorMessage := errors.Cause(err).Error()
		switch {
		case errors.Cause(err) == io.EOF,
			strings.HasSuffix(errorMessage, "connection is shut down"),
			strings.HasSuffix(errorMessage, "no api connection available"),
			strings.Contains(errorMessage, "spaces are still being discovered"):
			ctx.Verbosef("Still waiting for API to become available")
			continue
		case params.ErrCode(err) == params.CodeUpgradeInProgress:
			ctx.Verbosef("Still waiting for API to become available: %v", err)
			continue
		}
		break
	}
	return errors.Annotatef(err, "unable to contact api server after %d attempts", apiAttempts)
}

// BootstrapEndpointAddresses returns the addresses of the bootstrapped instance.
func BootstrapEndpointAddresses(environ environs.InstanceBroker, callContext context.ProviderCallContext) ([]network.Address, error) {
	instances, err := environ.AllInstances(callContext)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if n := len(instances); n != 1 {
		return nil, errors.Errorf("expected one instance, got %d", n)
	}
	netAddrs, err := instances[0].Addresses(callContext)
	if err != nil {
		return nil, errors.Annotate(err, "failed to get bootstrap instance addresses")
	}
	return netAddrs, nil
}
