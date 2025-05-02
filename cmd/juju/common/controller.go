// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/retry"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/client/block"
	caasprovider "github.com/juju/juju/caas/kubernetes/provider"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
)

var (
	bootstrapReadyPollDelay = 3 * time.Second
	bootstrapReadyPollCount = 60
	blockAPI                = getBlockAPI
)

type listBlocksAPI interface {
	List(context.Context) ([]params.Block, error)
	Close() error
}

// getBlockAPI returns a block api for listing blocks.
func getBlockAPI(ctx context.Context, c *modelcmd.ModelCommandBase) (listBlocksAPI, error) {
	// Set a short dial timeout so WaitForAgentInitialisation can check
	// ctx.Done() in its retry loop.
	dialOpts := api.DefaultDialOpts()
	dialOpts.Timeout = 6 * time.Second

	root, err := c.NewAPIRootWithDialOpts(ctx, &dialOpts)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return block.NewClient(root), nil
}

// tryAPI attempts to open the API and makes a trivial call
// to check if the API is available yet.
func tryAPI(ctx context.Context, c *modelcmd.ModelCommandBase) error {
	client, err := blockAPI(ctx, c)
	if err == nil {
		_, err = client.List(ctx)
		closeErr := client.Close()
		if closeErr != nil {
			logger.Debugf(context.TODO(), "Error closing client: %v", closeErr)
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
			return errors.Is(err, &unknownError{}) ||
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

			// As the API server is coming up, it goes through a number of steps.
			// Initially the upgrade steps run, but the api server allows some
			// calls to be processed during the upgrade, but not the list blocks.
			// Logins are also blocked during space discovery.
			// It is also possible that the underlying database causes connections
			// to be dropped as it is initialising, or reconfiguring. These can
			// lead to EOF or "connection is shut down" error messages. We skip
			// these too, hoping that things come back up before the end of the
			// retry poll count.
			cause := errors.Cause(retryErr)
			errorMessage := cause.Error()
			switch {
			case cause == io.EOF,
				strings.HasSuffix(errorMessage, "no such host"), // wait for dns getting resolvable, aws elb for example.
				strings.HasSuffix(errorMessage, "operation not permitted"),
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
			default:
				return &unknownError{
					err: retryErr,
				}
			}
		},
	})

	switch {
	case err == nil:
		return nil
	case errors.Is(err, &unknownError{}):
		err = errors.Cause(err)
	default:
		err = retry.LastError(err)
	}
	return errors.Annotatef(err, "unable to contact api server after %d attempts", apiAttempts)
}

// unknownError is used to wrap errors that we don't know how to handle.
type unknownError struct {
	err error
}

// Is implements errors.Is, so that we can identify this error type.
func (e *unknownError) Is(other error) bool {
	_, ok := other.(*unknownError)
	return ok
}

// Cause implements errors.Cause, so that we can unwrap this error type.
func (e *unknownError) Cause() error {
	return e.err
}

// Error implements error.Error.
func (e *unknownError) Error() string {
	return e.err.Error()
}

// BootstrapEndpointAddresses returns the addresses of the bootstrapped instance.
func BootstrapEndpointAddresses(
	environ environs.InstanceBroker, ctx context.Context,
) ([]network.ProviderAddress, error) {
	instances, err := environ.AllRunningInstances(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if n := len(instances); n != 1 {
		return nil, errors.Errorf("expected one instance, got %d", n)
	}
	netAddrs, err := instances[0].Addresses(ctx)
	if err != nil {
		return nil, errors.Annotate(err, "failed to get bootstrap instance addresses")
	}
	return netAddrs, nil
}

// ValidateIaasController returns an error if the controller
// is not an IAAS controller.
func ValidateIaasController(ctx context.Context, c modelcmd.CommandBase, cmdName, controllerName string, store jujuclient.ClientStore) error {
	// Ensure controller model is cached.
	controllerModel := jujuclient.JoinOwnerModelName(
		names.NewUserTag(environs.AdminUser), bootstrap.ControllerModelName)
	_, err := c.ModelUUIDs(ctx, store, controllerName, []string{controllerModel})
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
