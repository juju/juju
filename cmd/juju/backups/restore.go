// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"crypto/rand"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/utils"

	"github.com/juju/juju/api/backups"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/sync"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/version"
)

// NewRestoreCommand returns a command used to restore a backup.
func NewRestoreCommand() cmd.Command {
	restoreCmd := &restoreCommand{}
	restoreCmd.newEnvironFunc = environs.New
	restoreCmd.getRebootstrapParamsFunc = restoreCmd.getRebootstrapParams
	restoreCmd.newAPIClientFunc = func() (RestoreAPI, error) {
		return restoreCmd.newClient()
	}
	restoreCmd.getArchiveFunc = getArchive
	restoreCmd.waitForAgentFunc = common.WaitForAgentInitialisation
	return modelcmd.Wrap(restoreCmd)
}

// restoreCommand is a subcommand of backups that implement the restore behavior
// it is invoked with "juju restore-backup".
type restoreCommand struct {
	CommandBase
	constraints    constraints.Value
	constraintsStr string
	filename       string
	backupId       string
	bootstrap      bool
	buildAgent     bool

	newAPIClientFunc         func() (RestoreAPI, error)
	newEnvironFunc           func(environs.OpenParams) (environs.Environ, error)
	getRebootstrapParamsFunc func(*cmd.Context, string, *params.BackupsMetadataResult) (*restoreBootstrapParams, error)
	getArchiveFunc           func(string) (ArchiveReader, *params.BackupsMetadataResult, error)
	waitForAgentFunc         func(ctx *cmd.Context, c *modelcmd.ModelCommandBase, controllerName, hostedModelName string) error
}

// RestoreAPI is used to invoke various API calls.
type RestoreAPI interface {
	// Close is taken from io.Closer.
	Close() error

	// Restore is taken from backups.Client.
	Restore(backupId string, newClient backups.ClientConnection) error

	// RestoreReader is taken from backups.Client.
	RestoreReader(r io.ReadSeeker, meta *params.BackupsMetadataResult, newClient backups.ClientConnection) error
}

var restoreDoc = `
Restores a backup that was previously created with "juju create-backup".

This command creates a new controller and arranges for it to replace
the previous controller for a model.  It does *not* restore
an existing server to a previous state, but instead creates a new server
with equivalent state.  As part of restore, all known instances are
configured to treat the new controller as their master.

The given constraints will be used to choose the new instance.

If the provided state cannot be restored, this command will fail with
an appropriate message.  For instance, if the existing bootstrap
instance is already running then the command will fail with a message
to that effect.
`

var BootstrapFunc = bootstrap.Bootstrap

// Info returns the content for --help.
func (c *restoreCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "restore-backup",
		Purpose: "Restore from a backup archive to a new controller.",
		Args:    "",
		Doc:     strings.TrimSpace(restoreDoc),
	}
}

// SetFlags handles known option flags.
func (c *restoreCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	f.StringVar(&c.constraintsStr, "constraints", "", "set model constraints")
	f.BoolVar(&c.bootstrap, "b", false, "Bootstrap a new state machine")
	f.StringVar(&c.filename, "file", "", "Provide a file to be used as the backup.")
	f.StringVar(&c.backupId, "id", "", "Provide the name of the backup to be restored")
	f.BoolVar(&c.buildAgent, "build-agent", false, "Build binary agent if bootstraping a new machine")
}

// Init is where the preconditions for this commands can be checked.
func (c *restoreCommand) Init(args []string) error {
	if c.filename == "" && c.backupId == "" {
		return errors.Errorf("you must specify either a file or a backup id.")
	}
	if c.filename != "" && c.backupId != "" {
		return errors.Errorf("you must specify either a file or a backup id but not both.")
	}
	if c.backupId != "" && c.bootstrap {
		return errors.Errorf("it is not possible to rebootstrap and restore from an id.")
	}

	var err error
	if c.filename != "" {
		c.filename, err = filepath.Abs(c.filename)
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

type restoreBootstrapParams struct {
	ControllerConfig controller.Config
	Cloud            environs.CloudSpec
	CredentialName   string
	AdminSecret      string
	ModelConfig      *config.Config
}

// getRebootstrapParams returns the params for rebootstrapping the
// specified controller.
func (c *restoreCommand) getRebootstrapParams(
	ctx *cmd.Context, controllerName string, meta *params.BackupsMetadataResult,
) (*restoreBootstrapParams, error) {
	// TODO(axw) delete this and -b. We will update bootstrap with a flag
	// to specify a restore file. When we do that, we'll need to extract
	// the CA cert from the backup, and we'll need to reset the password
	// after restore so the admin user can login. We also need to store
	// things like the admin-secret, controller certificate etc with the
	// backup.
	store := c.ClientStore()
	controllerDetails, err := store.ControllerByName(controllerName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	config, params, err := modelcmd.NewGetBootstrapConfigParamsFunc(ctx, store)(controllerName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	provider, err := environs.Provider(config.CloudType)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cfg, err := provider.PrepareConfig(*params)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Get the local admin user so we can use the password as the admin secret.
	// TODO(axw) check that account.User is environs.AdminUser.
	var adminSecret string
	account, err := store.AccountDetails(controllerName)
	if err == nil {
		adminSecret = account.Password
	} else if errors.IsNotFound(err) {
		// No relevant local admin user so generate a new secret.
		buf := make([]byte, 16)
		if _, err := io.ReadFull(rand.Reader, buf); err != nil {
			return nil, errors.Annotate(err, "generating new admin secret")
		}
		adminSecret = fmt.Sprintf("%x", buf)
	} else {
		return nil, errors.Trace(err)
	}

	// Turn on safe mode so that the newly bootstrapped instance
	// will not destroy all the instances it does not know about.
	// Also set the admin secret and ca cert info.
	cfg, err = cfg.Apply(map[string]interface{}{
		"provisioner-safe-mode": true,
	})
	if err != nil {
		return nil, errors.Annotatef(err, "cannot enable provisioner-safe-mode")
	}

	controllerCfg := make(controller.Config)
	for k, v := range config.ControllerConfig {
		controllerCfg[k] = v
	}
	controllerCfg[controller.ControllerUUIDKey] = controllerDetails.ControllerUUID
	controllerCfg[controller.CACertKey] = meta.CACert

	return &restoreBootstrapParams{
		controllerCfg,
		params.Cloud,
		config.Credential,
		adminSecret,
		cfg,
	}, nil
}

// rebootstrap will bootstrap a new server in safe-mode (not killing any other agent)
// if there is no current server available to restore to.
func (c *restoreCommand) rebootstrap(ctx *cmd.Context, meta *params.BackupsMetadataResult) error {
	params, err := c.getRebootstrapParamsFunc(ctx, c.ControllerName(), meta)
	if err != nil {
		return errors.Trace(err)
	}

	cloudParam, err := cloud.CloudByName(params.Cloud.Name)
	if errors.IsNotFound(err) {
		provider, err := environs.Provider(params.Cloud.Type)
		if errors.IsNotFound(err) {
			return errors.NewNotFound(nil, fmt.Sprintf("unknown cloud %q, please try %q", params.Cloud.Name, "juju update-clouds"))
		} else if err != nil {
			return errors.Trace(err)
		}
		detector, ok := provider.(environs.CloudRegionDetector)
		if !ok {
			return errors.Errorf("provider %q does not support detecting regions", params.Cloud.Type)
		}
		var cloudEndpoint string
		regions, err := detector.DetectRegions()
		if errors.IsNotFound(err) {
			// It's not an error to have no regions. If the
			// provider does not support regions, then we
			// reinterpret the supplied region name as the
			// cloud's endpoint. This enables the user to
			// supply, for example, maas/<IP> or manual/<IP>.
			if params.Cloud.Region != "" {
				cloudEndpoint = params.Cloud.Region
			}
		} else if err != nil {
			return errors.Annotatef(err, "detecting regions for %q cloud provider", params.Cloud.Type)
		}
		schemas := provider.CredentialSchemas()
		authTypes := make([]cloud.AuthType, 0, len(schemas))
		for authType := range schemas {
			authTypes = append(authTypes, authType)
		}
		cloudParam = &cloud.Cloud{
			Type:      params.Cloud.Type,
			AuthTypes: authTypes,
			Endpoint:  cloudEndpoint,
			Regions:   regions,
		}
	} else if err != nil {
		return errors.Trace(err)
	}

	env, err := c.newEnvironFunc(environs.OpenParams{
		Cloud:  params.Cloud,
		Config: params.ModelConfig,
	})
	if err != nil {
		return errors.Annotate(err, "opening environ for rebootstrapping")
	}

	instanceIds, err := env.ControllerInstances(params.ControllerConfig.ControllerUUID())
	if err != nil && errors.Cause(err) != environs.ErrNotBootstrapped {
		return errors.Annotatef(err, "cannot determine controller instances")
	}
	if len(instanceIds) > 0 {
		inst, err := env.Instances(instanceIds)
		if err == nil {
			return errors.Errorf("old bootstrap instance %q still seems to exist; will not replace", inst)
		}
		if err != environs.ErrNoInstances {
			return errors.Annotatef(err, "cannot detect whether old instance is still running")
		}
	}

	// We require a hosted model config to bootstrap. We'll fill in some defaults
	// just to get going. The restore will clear the initial state.
	hostedModelUUID, err := utils.NewUUID()
	if err != nil {
		return errors.Trace(err)
	}
	hostedModelConfig := map[string]interface{}{
		"name":         "default",
		config.UUIDKey: hostedModelUUID.String(),
	}

	// We may have previous controller metadata. We need to replace that so it
	// will contain the new CA Cert and UUID required to connect to the newly
	// bootstrapped controller API.
	store := c.ClientStore()
	details := jujuclient.ControllerDetails{
		ControllerUUID: params.ControllerConfig.ControllerUUID(),
		CACert:         meta.CACert,
		Cloud:          params.Cloud.Name,
		CloudRegion:    params.Cloud.Region,
	}
	err = store.UpdateController(c.ControllerName(), details)
	if err != nil {
		return errors.Trace(err)
	}

	bootVers := version.Current
	args := bootstrap.BootstrapParams{
		Cloud:               *cloudParam,
		CloudName:           params.Cloud.Name,
		CloudRegion:         params.Cloud.Region,
		CloudCredentialName: params.CredentialName,
		CloudCredential:     params.Cloud.Credential,
		ModelConstraints:    c.constraints,
		BuildAgent:          c.buildAgent,
		BuildAgentTarball:   sync.BuildAgentTarball,
		ControllerConfig:    params.ControllerConfig,
		HostedModelConfig:   hostedModelConfig,
		BootstrapSeries:     meta.Series,
		AgentVersion:        &bootVers,
		AdminSecret:         params.AdminSecret,
		CAPrivateKey:        meta.CAPrivateKey,
		DialOpts: environs.BootstrapDialOpts{
			Timeout:        time.Second * bootstrap.DefaultBootstrapSSHTimeout,
			RetryDelay:     time.Second * bootstrap.DefaultBootstrapSSHRetryDelay,
			AddressesDelay: time.Second * bootstrap.DefaultBootstrapSSHAddressesDelay,
		},
	}
	if err := BootstrapFunc(modelcmd.BootstrapContext(ctx), env, args); err != nil {
		return errors.Annotatef(err, "cannot bootstrap new instance")
	}

	// New controller is bootstrapped, so now record the API address so
	// we can connect.
	apiPort := params.ControllerConfig.APIPort()
	err = common.SetBootstrapEndpointAddress(store, c.ControllerName(), bootVers, apiPort, env)
	if err != nil {
		return errors.Trace(err)
	}

	// To avoid race conditions when running scripted bootstraps, wait
	// for the controller's machine agent to be ready to accept commands
	// before exiting this bootstrap command.
	return c.waitForAgentFunc(ctx, &c.ModelCommandBase, c.ControllerName(), "default")
}

func (c *restoreCommand) newClient() (*backups.Client, error) {
	client, err := c.NewAPIClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	backupsClient, ok := client.(*backups.Client)
	if !ok {
		return nil, errors.Errorf("invalid client for backups")
	}
	return backupsClient, nil
}

// Run is the entry point for this command.
func (c *restoreCommand) Run(ctx *cmd.Context) error {
	var err error
	c.constraints, err = common.ParseConstraints(ctx, c.constraintsStr)
	if err != nil {
		return err
	}

	if c.Log != nil {
		if err := c.Log.Start(ctx); err != nil {
			return err
		}
	}

	var archive ArchiveReader
	var meta *params.BackupsMetadataResult
	target := c.backupId
	if c.filename != "" {
		// Read archive specified by the filename;
		// we'll need the info later regardless if
		// we need it now to rebootstrap.
		target = c.filename
		var err error
		archive, meta, err = c.getArchiveFunc(c.filename)
		if err != nil {
			return errors.Trace(err)
		}
		defer archive.Close()

		if c.bootstrap {
			if err := c.rebootstrap(ctx, meta); err != nil {
				return errors.Trace(err)
			}
		}
	}

	client, err := c.newAPIClientFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer client.Close()

	// We have a backup client, now use the relevant method
	// to restore the backup.
	if c.filename != "" {
		err = client.RestoreReader(archive, meta, c.newClient)
	} else {
		err = client.Restore(c.backupId, c.newClient)
	}
	if err != nil {
		return errors.Trace(err)
	}
	fmt.Fprintf(ctx.Stdout, "restore from %q completed\n", target)
	return nil
}
