// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package system

import (
	"bytes"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/environmentmanager"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/juju"
)

// ForceDestroyCommand destroys the specified system.
type ForceDestroyCommand struct {
	DestroyCommand
	killAll      bool
	ignoreBlocks bool
	broken       bool
}

// TODO (cherylj) Add more detailed documentation
var (
	forceDestroyDoc = `Forcefully destroys the specified system`
	dialAPI         = juju.NewAPIFromName
	ErrConnTimedOut = errors.New("connection to state server timed out")
)

// Info implements Command.Info.
func (c *ForceDestroyCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "force-destroy",
		Args:    "<system name>",
		Purpose: "forcefully terminate all machines and other associated resources for a system environment",
		Doc:     forceDestroyDoc,
	}
}

// SetFlags implements Command.SetFlags.
func (c *ForceDestroyCommand) SetFlags(f *gnuflag.FlagSet) {
	f.BoolVar(&c.killAll, "kill-all-envs", false, "Destroy all hosted environments on the system")
	f.BoolVar(&c.ignoreBlocks, "ignore-blocks", false, "Ignore any blocks on hosted environments")
	f.BoolVar(&c.broken, "broken", false, "Destroy the system even if state servers are unreachable")
	c.DestroyCommand.SetFlags(f)
}

func (c *ForceDestroyCommand) getAPI(info configstore.EnvironInfo) (_ destroyEnvironmentAPI, err error) {
	if c.api != nil {
		return c.api, c.apierr
	}

	// Attempt to connect to the API with a short timeout
	apic := make(chan *api.State)
	errc := make(chan error)
	go func() {
		api, err := dialAPI(c.systemName)
		if err != nil {
			errc <- err
			return
		}
		apic <- api
	}()

	select {
	case err = <-errc:
		c.apiRoot = nil
	case c.apiRoot = <-apic:
		err = nil
	case <-time.After(10 * time.Second):
		err = ErrConnTimedOut
	}

	if err != nil {
		return nil, err
	}

	return environmentmanager.NewClient(c.apiRoot), nil
}

// Run implements Command.Run
func (c *ForceDestroyCommand) Run(ctx *cmd.Context) error {
	store, err := configstore.Default()
	if err != nil {
		return errors.Annotate(err, "cannot open system info storage")
	}

	cfgInfo, err := store.ReadInfo(c.systemName)
	if err != nil {
		return errors.Annotate(err, "cannot read system info")
	}

	// Verify that we're destroying a system
	apiEndpoint := cfgInfo.APIEndpoint()
	if apiEndpoint.ServerUUID != "" && apiEndpoint.EnvironUUID != apiEndpoint.ServerUUID {
		return errors.Errorf("%q is not a system; use juju environment destroy to destroy it", c.systemName)
	}

	// Attempt to connect to the API.
	api, err := c.getAPI(cfgInfo)
	switch {
	case errors.IsNotFound(err):
		logger.Warningf("system not found, removing config file")
		ctx.Infof("system not found, removing config file")
		return environs.DestroyInfo(c.systemName, store)
	case err == common.ErrPerm:
		return errors.Annotate(err, "cannot destroy system")
	case err == ErrConnTimedOut:
		logger.Warningf("Unable to open API: %s\n", err)
		if !c.broken {
			return c.ensureUserFriendlyErrorLog(errors.Annotate(err, "cannot destroy system"))
		}
		api = nil
	case err == nil:
		defer api.Close()
	default:
		return c.ensureUserFriendlyErrorLog(errors.Annotate(err, "cannot destroy system"))
	}

	if !c.assumeYes {
		if err = confirmDestruction(ctx, c.systemName); err != nil {
			return err
		}
	}

	// If the user specified both --kill-all-envs and --ignore-blocks, then we should
	// force the system destruction, even if the underlying API call returns an error
	// (except for unauthorized errors)
	force := c.killAll && c.ignoreBlocks

	// Obtain bootstrap / system environ information
	systemEnviron, err := getSystemEnviron(cfgInfo, api)
	if params.IsCodeNotImplemented(err) {
		// Fall back to using the client endpoint to obtain bootstrap
		// information and to destroy the system, sending the info
		// we were already able to collect.
		logger.Warningf("Falling back to client")
		return c.destroyEnvironmentViaClient(ctx, cfgInfo, nil, store, force)
	}
	if err != nil {
		return errors.Annotate(err, "cannot obtain bootstrap information")
	}

	// Attempt to destroy the system.
	if api != nil {
		err = api.DestroySystem(apiEndpoint.EnvironUUID, c.killAll, c.ignoreBlocks)
		if params.IsCodeNotImplemented(err) {
			// Fall back to using the client endpoint to destroy the system,
			// sending the info we were already able to collect.
			return c.destroyEnvironmentViaClient(ctx, cfgInfo, systemEnviron, store, force)
		}
		if err != nil {
			if params.IsCodeOperationBlocked(err) {
				printBlockedEnvs(ctx, api)
				return err
			}
			if !force || err == common.ErrPerm {
				return c.ensureUserFriendlyErrorLog(errors.Annotate(err, "cannot destroy system"))
			}
			logger.Warningf("failed to destroy system %q through API: %s.  Attempting forced destruction.", c.systemName, err)
		}
	}

	return environs.Destroy(systemEnviron, store)
}

func printBlockedEnvs(ctx *cmd.Context, api destroyEnvironmentAPI) {
	envs, err := api.ListBlockedEnvironments()
	if err != nil {
		fmt.Fprintf(ctx.Stdout, "Unable to list blocked environments: %s", err)
		return
	}

	var out bytes.Buffer
	const (
		// To format things into columns.
		minwidth = 0
		tabwidth = 1
		padding  = 2
		padchar  = ' '
		flags    = 0
	)
	tw := tabwriter.NewWriter(&out, minwidth, tabwidth, padding, padchar, flags)
	fmt.Fprintf(tw, "NAME\tENVIRONMENT UUID\tOWNER\tBLOCKS\n")
	for _, env := range envs {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", env.Name, env.UUID, env.OwnerTag, blocksToStr(env.Blocks))
	}
	tw.Flush()

	fmt.Fprintf(ctx.Stdout, blockedMsg, out.String())
}

var blockedMsg = `
Unable to destroy system.  Found blocks on the following environment(s):
%s
To ignore blocks when destroying the system, run

    juju system force-destroy --ignore-blocks

to remove all blocks in the system before destruction.
`

func blocksToStr(blocks []string) string {
	result := ""

	blockTypes := map[string]string{
		"BlockDestroy": "destroy-environment",
		"BlockRemove":  "remove-object",
		"BlockChange":  "all-changes",
	}

	for _, blk := range blocks {
		sep := ","
		if result == "" {
			sep = ""
		}

		result = result + sep + blockTypes[blk]
	}

	return result
}
