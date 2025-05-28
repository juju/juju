// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/controller/controller"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/cmd"
)

const killDoc = `
Forcibly destroy the specified controller.  If the API server is accessible,
this command will attempt to destroy the controller model and all models
and their resources.

If the API server is unreachable, the machines of the controller model will be
destroyed through the cloud provisioner.  If there are additional machines,
including machines within models, these machines will not be destroyed
and will never be reconnected to the Juju controller being destroyed.

The normal process of killing the controller will involve watching the
models as they are brought down in a controlled manner. If for some reason the
models do not stop cleanly, there is a default five minute timeout. If no change
in the model state occurs for the duration of this timeout, the command will
stop watching and destroy the models directly through the cloud provider.
`

// NewKillCommand returns a command to kill a controller. Killing is a
// forceful destroy.
func NewKillCommand() modelcmd.Command {
	cmd := killCommand{clock: clock.WallClock}
	cmd.environsDestroy = environs.Destroy
	return wrapKillCommand(&cmd)
}

// wrapKillCommand provides the common wrapping used by tests and
// the default NewKillCommand above.
func wrapKillCommand(kill *killCommand) modelcmd.Command {
	return modelcmd.WrapController(
		kill,
		modelcmd.WrapControllerSkipControllerFlags,
		modelcmd.WrapControllerSkipDefaultController,
	)
}

// killCommand kills the specified controller.
type killCommand struct {
	destroyCommandBase

	clock   clock.Clock
	timeout time.Duration
}

// SetFlags implements Command.SetFlags.
func (c *killCommand) SetFlags(f *gnuflag.FlagSet) {
	c.destroyCommandBase.SetFlags(f)
	f.Var(newDurationValue(time.Minute*5, &c.timeout), "t", "Timeout before direct destruction")
	f.Var(newDurationValue(time.Minute*5, &c.timeout), "timeout", "")
}

// Info implements Command.Info.
func (c *killCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "kill-controller",
		Args:    "<controller name>",
		Purpose: "Forcibly terminate all machines and other associated resources for a Juju controller.",
		Doc:     killDoc,
		SeeAlso: []string{
			"destroy-controller",
			"unregister",
		},
	})
}

// Init implements Command.Init.
func (c *killCommand) Init(args []string) error {
	return c.destroyCommandBase.Init(args)
}

var errConnTimedOut = errors.New("open connection timed out")

// Run implements Command.Run
func (c *killCommand) Run(ctx *cmd.Context) error {
	controllerName, err := c.ControllerName()
	if err != nil {
		return errors.Trace(err)
	}
	store := c.ClientStore()

	// Attempt to connect to the API.
	api, err := c.getControllerAPIWithTimeout(ctx, 10*time.Second)
	switch errors.Cause(err) {
	case nil:
		defer api.Close()
	case apiservererrors.ErrPerm:
		return errors.Annotate(err, "cannot destroy controller")
	default:
		ctx.Infof("Unable to open API: %s\n", err)
	}

	controllerModelConfigAPI, err := c.getControllerModelConfigAPI(ctx)
	if err != nil {
		return fmt.Errorf("cannot connect to model config API: %w", err)
	}
	defer func() { _ = controllerModelConfigAPI.Close() }()

	// Obtain controller environ so we can clean up afterwards.
	controllerEnviron, err := c.getControllerEnviron(ctx, store, controllerName, api, controllerModelConfigAPI)
	if err != nil {
		return errors.Annotate(err, "getting controller environ")
	}
	// If we were unable to connect to the API, just destroy the controller through
	// the environs interface.
	if api == nil {
		ctx.Infof("Unable to connect to the API server, destroying through provider")
		return c.environsDestroy(controllerName, controllerEnviron, ctx, store)
	}

	if c.DestroyConfirmationCommandBase.NeedsConfirmation() {
		updateStatus := newTimedStatusUpdater(ctx, api, controllerEnviron.Config().UUID(), clock.WallClock)
		modelStatus := updateStatus(0)
		ctx.Warningf(destroySysMsg, controllerName)
		if err := printDestroyWarningDetails(ctx, modelStatus, false); err != nil {
			return errors.Trace(err)
		}
		if err := jujucmd.UserConfirmName(controllerName, "controller", ctx); err != nil {
			return errors.Annotate(err, "controller destruction")
		}
	}

	// Attempt to destroy the controller and all models and storage.
	destroyStorage := true
	err = api.DestroyController(ctx, controller.DestroyControllerParams{
		DestroyModels:  true,
		DestroyStorage: &destroyStorage,
	})
	if err != nil {
		ctx.Infof("Unable to destroy controller through the API: %s\nDestroying through provider", err)
		return c.environsDestroy(controllerName, controllerEnviron, ctx, store)
	}

	ctx.Infof("Destroying controller %q\nWaiting for resources to be reclaimed", controllerName)

	controllerCloudSpec, err := c.getControllerCloudSpecFromStore(ctx, store, controllerName)
	if err != nil {
		logger.Debugf(ctx, "unable to get controller %q cloud spec from local store", controllerName)
		controllerCloudSpec = cloudspec.CloudSpec{}
	}

	uuid := controllerEnviron.Config().UUID()
	if err := c.WaitForModels(ctx, api, uuid); err != nil {
		c.DirectDestroyRemaining(ctx, api, controllerCloudSpec)
	}
	return c.environsDestroy(controllerName, controllerEnviron, ctx, store)
}

func (c *killCommand) getControllerAPIWithTimeout(ctx context.Context, timeout time.Duration) (destroyControllerAPI, error) {
	type result struct {
		c   destroyControllerAPI
		err error
	}
	resultC := make(chan result)
	done := make(chan struct{})
	go func() {
		api, err := c.getControllerAPI(ctx)
		select {
		case resultC <- result{api, err}:
		case <-done:
			if api != nil {
				api.Close()
			}
		}
	}()
	select {
	case r := <-resultC:
		return r.c, r.err
	case <-ctx.Done():
		close(done)
		return nil, ctx.Err()
	case <-c.clock.After(timeout):
		close(done)
		return nil, errConnTimedOut
	}
}

// DirectDestroyRemaining will attempt to directly destroy any remaining
// models that have machines left.
func (c *killCommand) DirectDestroyRemaining(
	ctx *cmd.Context,
	api destroyControllerAPI,
	controllerCloudSpec cloudspec.CloudSpec) {

	hasErrors := false
	hostedConfig, err := api.HostedModelConfigs(ctx)
	if err != nil {
		hasErrors = true
		logger.Warningf(ctx, "unable to retrieve hosted model config: %v", err)
	}
	ctrlUUID := ""
	// try to get controller UUID or just ignore.
	if ctrlCfg, err := api.ControllerConfig(ctx.Context); err == nil {
		ctrlUUID = ctrlCfg.ControllerUUID()
	} else {
		logger.Warningf(ctx, "getting controller config from API: %v", err)
	}
	for _, model := range hostedConfig {
		if model.Error != nil {
			// We can only display model name here since
			// the error coming from api can be anything
			// including the parsing of the model owner tag.
			// Only model name is guaranteed to be set in the result
			// when an error is returned.
			hasErrors = true
			logger.Warningf(ctx, "could not kill %s directly: %v", model.Name, model.Error)
			continue
		}
		ctx.Infof("Killing %s/%s directly", model.Qualifier, model.Name)
		cfg, err := config.New(config.NoDefaults, model.Config)
		if err != nil {
			logger.Warningf(ctx, err.Error())
			hasErrors = true
			continue
		}
		p, err := environs.Provider(model.CloudSpec.Type)
		if err != nil {
			logger.Warningf(ctx, err.Error())
			hasErrors = true
			continue
		}

		modelCloudSpec, err := transformModelCloudSpecForInstanceRoles(model.Name, model.CloudSpec, controllerCloudSpec)
		if err != nil {
			logger.Warningf(ctx, "could not kill %s directly: %v", model.Name, err)
			continue
		}

		if cloudProvider, ok := p.(environs.EnvironProvider); ok {
			openParams := environs.OpenParams{
				ControllerUUID: ctrlUUID,
				Cloud:          modelCloudSpec,
				Config:         cfg,
			}
			var env environs.CloudDestroyer
			if cloud.CloudTypeIsCAAS(model.CloudSpec.Type) {
				env, err = caas.Open(ctx, cloudProvider, openParams, environs.NoopCredentialInvalidator())
			} else {
				env, err = environs.Open(ctx, cloudProvider, openParams, environs.NoopCredentialInvalidator())
			}
			if err != nil {
				logger.Warningf(ctx, err.Error())
				hasErrors = true
				continue
			}
			if err := env.Destroy(ctx); err != nil {
				logger.Warningf(ctx, err.Error())
				hasErrors = true
				continue
			}
		}
		ctx.Infof("  done")
	}
	if hasErrors {
		logger.Warningf(ctx, "there were problems destroying some models, manual intervention may be necessary to ensure resources are released")
	} else {
		ctx.Infof("All models destroyed, cleaning up controller machines")
	}
}

// transformModelCloudSpecForInstanceRoles is a temporary hack for dealing ec2
// instance role credentials for cleaning up AWS resources client side as we
// can't use the instance role credentials from the client. A better solution
// exists but requires a significant refactor.
// tlm 9/12/2021
func transformModelCloudSpecForInstanceRoles(
	modelName string,
	modelCloudSpec cloudspec.CloudSpec,
	controllerCloudSpec cloudspec.CloudSpec,
) (cloudspec.CloudSpec, error) {
	authType := modelCloudSpec.Credential.AuthType()
	var notSupportedAuthType bool
	switch modelCloudSpec.Type {
	case "ec2":
		notSupportedAuthType = authType == cloud.InstanceRoleAuthType
	case "azure":
		notSupportedAuthType = authType == cloud.ManagedIdentityAuthType
	}
	if notSupportedAuthType {
		if modelCloudSpec.Type != controllerCloudSpec.Type ||
			modelCloudSpec.Name != controllerCloudSpec.Name {
			return modelCloudSpec, errors.NotSupportedf(
				"model %q uses %s credentials, can't destroy model. It will have to be cleaned up manually", modelName, authType)
		}
		return controllerCloudSpec, nil
	}
	return modelCloudSpec, nil
}

// WaitForModels will wait for the models to bring themselves down nicely.
// It will return the UUIDs of any models that need to be removed forceably.
func (c *killCommand) WaitForModels(ctx *cmd.Context, api destroyControllerAPI, uuid string) error {
	thirtySeconds := (time.Second * 30)
	updateStatus := newTimedStatusUpdater(ctx, api, uuid, c.clock)

	envStatus := updateStatus(0)
	lastStatus := envStatus.Controller
	lastChange := c.clock.Now().Truncate(time.Second)
	deadline := lastChange.Add(c.timeout)
	// Check for both undead models and live machines, as machines may be
	// in the controller model.
	for ; hasUnreclaimedResources(envStatus) && (deadline.After(c.clock.Now())); envStatus = updateStatus(5 * time.Second) {
		now := c.clock.Now().Truncate(time.Second)
		if envStatus.Controller != lastStatus {
			lastStatus = envStatus.Controller
			lastChange = now
			deadline = lastChange.Add(c.timeout)
		}
		timeSinceLastChange := now.Sub(lastChange)
		timeUntilDestruction := deadline.Sub(now)
		warning := ""
		// We want to show the warning if it has been more than 30 seconds since
		// the last change, or we are within 30 seconds of our timeout.
		if timeSinceLastChange > thirtySeconds || timeUntilDestruction < thirtySeconds {
			warning = fmt.Sprintf(", will kill machines directly in %s", timeUntilDestruction)
		}
		ctx.Infof("%s%s", fmtCtrStatus(envStatus.Controller), warning)
		for _, modelStatus := range envStatus.Models {
			ctx.Verbosef("%s", fmtModelStatus(modelStatus))
		}
	}
	if hasUnreclaimedResources(envStatus) {
		return errors.New("timed out")
	} else {
		ctx.Infof("All models reclaimed, cleaning up controller machines")
	}
	return nil
}

type durationValue time.Duration

func newDurationValue(value time.Duration, p *time.Duration) *durationValue {
	*p = value
	return (*durationValue)(p)
}

func (d *durationValue) Set(s string) error {
	v, err := time.ParseDuration(s)
	if err != nil {
		return err
	}
	*d = durationValue(v)
	return err
}

func (d *durationValue) String() string { return (*time.Duration)(d).String() }
