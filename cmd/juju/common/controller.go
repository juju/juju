// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"
	"fmt"
	"io"
	"net/rpc"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/retry"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/jujuclient"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/internal/errors"
	caasprovider "github.com/juju/juju/internal/provider/kubernetes"
	"github.com/juju/juju/rpc/params"
)

var (
	bootstrapReadyPollDelay = 3 * time.Second
	bootstrapReadyPollCount = 60
)

// TryAPI attempts to open the API.
func TryAPI(ctx context.Context, c *modelcmd.ModelCommandBase) error {
	dialOpts := api.DefaultDialOpts()
	dialOpts.Timeout = 10 * time.Second
	root, err := c.NewAPIRootWithDialOpts(ctx, &dialOpts)
	if err != nil {
		return errors.Capture(err)
	}
	_ = root.Close()
	return err
}

const unknownError = errors.ConstError("unknown error in bootstrap api connect")

// WaitForAgentInitialisation polls the bootstrapped controller with a read-only
// command which will fail until the controller is fully initialised.
// TODO(wallyworld) - add a bespoke command to maybe the admin facade for this purpose.
func WaitForAgentInitialisation(
	ctx environs.BootstrapContext,
	c *modelcmd.ModelCommandBase,
	isCAASController bool,
	controllerName string,
	tryAPI func(ctx context.Context, c *modelcmd.ModelCommandBase) error,
) (err error) {
	if ctx.Err() != nil {
		return errors.Errorf("unable to contact api server: (%v)", ctx.Err())
	}

	// Make a best effort to find the new controller address so we can print it.
	var addressInfo string
	controller, err := c.ClientStore().ControllerByName(controllerName)
	if err == nil && len(controller.APIEndpoints) > 0 {
		addr, err := network.ParseMachineHostPort(controller.APIEndpoints[0])
		if err == nil {
			addressInfo = fmt.Sprintf(" at %s", addr.Host())
		}
	}

	ctx.Infof("Contacting Juju controller%s to verify accessibility...", addressInfo)

	var apiAttempts int
	err = retry.Call(retry.CallArgs{
		Clock:    clock.WallClock,
		Attempts: bootstrapReadyPollCount,
		Delay:    bootstrapReadyPollDelay,
		Stop:     ctx.Done(),
		NotifyFunc: func(lastErr error, attempts int) {
			apiAttempts = attempts
		},
		IsFatalError: func(err error) bool {
			return errors.Is(err, unknownError) ||
				retry.IsRetryStopped(err) ||
				errors.Is(err, context.Canceled)
		},
		Func: func() error {
			retryErr := tryAPI(ctx, c)
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

			logger.Debugf(ctx, "failed api connection attempt: %v", retryErr)

			// As the API server is coming up, it goes through a number of steps.
			// Initially the upgrade steps run, but the api server allows some
			// calls to be processed during the upgrade, but not the list blocks.
			// Logins are also blocked during space discovery.
			// It is also possible that the underlying database causes connections
			// to be dropped as it is initialising, or reconfiguring. These can
			// lead to EOF or "connection is shut down" error messages. We skip
			// these too, hoping that things come back up before the end of the
			// retry poll count.
			switch {
			case errors.Is(retryErr, io.EOF):
				ctx.Verbosef("Still waiting for API to become available: %v", retryErr)
				return retryErr
			case errors.Is(retryErr, api.ConnectionOpenTimedOut):
				ctx.Verbosef("Still waiting for API to become available: %v", retryErr)
				return retryErr
			case errors.Is(retryErr, api.ConnectionDialTimedOut):
				ctx.Verbosef("Still waiting for API to become available: %v", retryErr)
				return retryErr
			case errors.Is(retryErr, rpc.ErrShutdown):
				ctx.Verbosef("Still waiting for API to become available: %v", retryErr)
				return retryErr
			case params.ErrCode(retryErr) == params.CodeUpgradeInProgress:
				ctx.Verbosef("Still waiting for API to become available: %v", retryErr)
				return retryErr
			case isRetryableErrorMessage(retryErr.Error()):
				ctx.Verbosef("Still waiting for API to become available: %v", retryErr)
				return retryErr
			default:
				return errors.Errorf(
					"unknown error while connecting to controller api: %v", retryErr,
				).Add(unknownError)
			}
		},
	})

	switch {
	case err == nil:
		return nil
	case !errors.Is(err, unknownError):
		err = retry.LastError(err)
	}
	return errors.Errorf(
		"unable to contact api server after %d attempts: %w", apiAttempts, err,
	)
}

func isRetryableErrorMessage(errorMessage string) bool {
	switch {
	case strings.HasSuffix(errorMessage, "no such host"), // wait for dns getting resolvable, aws elb for example.
		strings.HasSuffix(errorMessage, "operation not permitted"),
		strings.HasSuffix(errorMessage, "connection refused"),
		strings.HasSuffix(errorMessage, "target machine actively refused it."), // Winsock message for connection refused
		strings.HasSuffix(errorMessage, "connection is shut down"),
		strings.HasSuffix(errorMessage, "i/o timeout"),
		strings.HasSuffix(errorMessage, "network is unreachable"),
		strings.HasSuffix(errorMessage, "deadline exceeded"),
		strings.HasSuffix(errorMessage, "no api connection available"):
		return true
	default:
		return false
	}
}

// BootstrapEndpointAddresses returns the addresses of the bootstrapped instance.
func BootstrapEndpointAddresses(
	environ environs.InstanceBroker, ctx context.Context,
) ([]network.ProviderAddress, error) {
	instances, err := environ.AllRunningInstances(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}
	if n := len(instances); n != 1 {
		return nil, errors.Errorf("expected one instance, got %d", n)
	}
	netAddrs, err := instances[0].Addresses(ctx)
	if err != nil {
		return nil, errors.Errorf(
			"failed to get bootstrap instance addresses: %w", err,
		)
	}
	return netAddrs, nil
}

// ValidateIaasController returns an error if the controller
// is not an IAAS controller.
func ValidateIaasController(ctx context.Context, c modelcmd.CommandBase, cmdName, controllerName string, store jujuclient.ClientStore) error {
	// Ensure controller model is cached.
	controllerModel := jujuclient.QualifyModelName(
		environs.AdminUser, bootstrap.ControllerModelName)
	_, err := c.ModelUUIDs(ctx, store, controllerName, []string{controllerModel})
	if err != nil {
		return errors.Errorf("cannot get controller model uuid: %w", err)
	}

	details, err := store.ModelByName(controllerName, controllerModel)
	if err != nil {
		return errors.Capture(err)
	}
	if details.ModelType == model.IAAS {
		return nil
	}
	return errors.Errorf(
		"Juju command %q not supported on container controllers", cmdName,
	)
}
