// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/retry"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/block"
	"github.com/juju/juju/apiserver/params"
	caasprovider "github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/jujuclient"
)

var (
	bootstrapReadyPollDelay = 3 * time.Second
	bootstrapReadyPollCount = 60
	blockAPI                = getBlockAPI
)

type listBlocksAPI interface {
	List() ([]params.Block, error)
	Close() error
}

// getBlockAPI returns a block api for listing blocks.
func getBlockAPI(c *modelcmd.ModelCommandBase) (listBlocksAPI, error) {
	// Set a short dial timeout so WaitForAgentInitialisation can check
	// ctx.Done() in its retry loop.
	dialOpts := api.DefaultDialOpts()
	dialOpts.Timeout = 6 * time.Second

	root, err := c.NewAPIRootWithDialOpts(&dialOpts)
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
func WaitForAgentInitialisation(
	ctx environs.BootstrapContext,
	c *modelcmd.ModelCommandBase,
	isCAASController bool,
	controllerName string,
) (err error) {
	// Make a best effort to find the new controller address so we can print it.
	addressInfo := ""
	controller, err := c.ClientStore().ControllerByName(controllerName)
	if err == nil && len(controller.APIEndpoints) > 0 {
		addr, err := network.ParseMachineHostPort(controller.APIEndpoints[0])
		if err == nil {
			addressInfo = fmt.Sprintf(" at %s", addr.Host())
		}
	}

	ctx.Infof("Contacting Juju controller%s to verify accessibility...", addressInfo)
	apiAttempts := 0
	stop := make(chan struct{}, 1)
	defer close(stop)
	err = retry.Call(retry.CallArgs{
		Clock:    clock.WallClock,
		Attempts: bootstrapReadyPollCount,
		Delay:    bootstrapReadyPollDelay,
		Stop:     stop,
		Func: func() error {
			apiAttempts++

			retryErr := tryAPI(c)
			if retryErr == nil {
				msg := fmt.Sprintf("\nBootstrap complete, controller %q is now available", controllerName)
				if isCAASController {
					msg += fmt.Sprintf(" in namespace %q", caasprovider.DecideControllerNamespace(controllerName))
				} else {
					msg += fmt.Sprintf("\nController machines are in the %q model", bootstrap.ControllerModelName)
				}
				ctx.Infof(msg)
				return nil
			}

			// Check whether context is cancelled after each attempt (as context
			// isn't fully threaded through yet).
			select {
			case <-ctx.Context().Done():
				stop <- struct{}{}
				return errors.Annotatef(err, "contacting controller (cancelled)")
			default:
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
			errorMessage := errors.Cause(retryErr).Error()
			switch {
			case errors.Cause(retryErr) == io.EOF,
				strings.HasSuffix(errorMessage, "no such host"), // wait for dns getting resolvable, aws elb for example.
				strings.HasSuffix(errorMessage, "connection refused"),
				strings.HasSuffix(errorMessage, "target machine actively refused it."), // Winsock message for connection refused
				strings.HasSuffix(errorMessage, "connection is shut down"),
				strings.HasSuffix(errorMessage, "i/o timeout"),
				strings.HasSuffix(errorMessage, "network is unreachable"),
				strings.HasSuffix(errorMessage, "deadline exceeded"),
				strings.HasSuffix(errorMessage, "no api connection available"):
				ctx.Verbosef("Still waiting for API to become available: %v", retryErr)
				return retryErr
			case params.ErrCode(retryErr) == params.CodeUpgradeInProgress:
				ctx.Verbosef("Still waiting for API to become available: %v", retryErr)
				return retryErr
			}
			stop <- struct{}{}
			return retryErr
		},
	})
	if err != nil {
		err = retry.LastError(err)
		return errors.Annotatef(err, "unable to contact api server after %d attempts", apiAttempts)
	}
	return nil
}

// BootstrapEndpointAddresses returns the addresses of the bootstrapped instance.
func BootstrapEndpointAddresses(
	environ environs.InstanceBroker, callContext context.ProviderCallContext,
) ([]network.ProviderAddress, error) {
	instances, err := environ.AllRunningInstances(callContext)
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

// ValidateIaasController returns an error if the controller
// is not an IAAS controller.
func ValidateIaasController(c modelcmd.CommandBase, cmdName, controllerName string, store jujuclient.ClientStore) error {
	// Ensure controller model is cached.
	controllerModel := jujuclient.JoinOwnerModelName(
		names.NewUserTag(environs.AdminUser), bootstrap.ControllerModelName)
	_, err := c.ModelUUIDs(store, controllerName, []string{controllerModel})
	if err != nil {
		return errors.Annotatef(err, "cannot get controller model uuid")
	}

	details, err := store.ModelByName(controllerName, controllerModel)
	if err != nil {
		return errors.Trace(err)
	}
	if details.ModelType == model.IAAS {
		return nil
	}
	return errors.Errorf("Juju command %q not supported on container controllers", cmdName)
}
