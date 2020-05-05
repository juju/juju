// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package model

import (
	"fmt"
	"os"

	"github.com/juju/charm/v7"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/api/application"
	"github.com/juju/juju/api/bundle"
	appFacade "github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
)

// NewExportBundleCommand returns a fully constructed export bundle command.
func NewExportBundleCommand() cmd.Command {
	command := &exportBundleCommand{}
	command.newAPIFunc = func() (ExportBundleAPI, ConfigAPI, error) {
		return command.getAPIs()
	}
	return modelcmd.Wrap(command)
}

type exportBundleCommand struct {
	modelcmd.ModelCommandBase
	out        cmd.Output
	newAPIFunc func() (ExportBundleAPI, ConfigAPI, error)
	Filename   string
}

const exportBundleHelpDoc = `
Exports the current model configuration as a reusable bundle.

If --filename is not used, the configuration is printed to stdout.
 --filename specifies an output file.

Examples:

    juju export-bundle
    juju export-bundle --filename mymodel.yaml

`

// Info implements Command.
func (c *exportBundleCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "export-bundle",
		Purpose: "Exports the current model configuration as a reusable bundle.",
		Doc:     exportBundleHelpDoc,
	})
}

// SetFlags implements Command.
func (c *exportBundleCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.StringVar(&c.Filename, "filename", "", "Bundle file")
}

// Init implements Command.
func (c *exportBundleCommand) Init(args []string) error {
	return cmd.CheckEmpty(args)
}

// ExportBundleAPI specifies the used function calls of the BundleFacade.
type ExportBundleAPI interface {
	BestAPIVersion() int
	Close() error
	ExportBundle() (string, error)
}

// ConfigAPI specifies the used function calls of the ApplicationFacade.
type ConfigAPI interface {
	Close() error
	Get(branchName string, application string) (*params.ApplicationGetResults, error)
}

func (c *exportBundleCommand) getAPIs() (ExportBundleAPI, ConfigAPI, error) {
	api, err := c.NewAPIRoot()
	if err != nil {
		return nil, nil, err
	}

	return bundle.NewClient(api), application.NewClient(api), nil
}

// Run implements Command.
func (c *exportBundleCommand) Run(ctx *cmd.Context) error {
	bundleClient, cfgClient, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer func() {
		_ = bundleClient.Close()
		_ = cfgClient.Close()
	}()

	result, err := bundleClient.ExportBundle()
	if err != nil {
		return err
	}

	// The V3 API exports the trust flag for bundle contents; for
	// older server API versions we need to query the config for each
	// app and patch the bundle client-side.
	if bundleClient.BestAPIVersion() < 3 {
		if result, err = c.injectTrustFlag(cfgClient, result); err != nil {
			return errors.Trace(err)
		}
	}

	if c.Filename == "" {
		_, err := fmt.Fprintf(ctx.Stdout, "%v", result)
		return err
	}
	filename := c.Filename
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0600)
	if err != nil {
		return errors.Annotate(err, "while creating local file")
	}
	defer file.Close()

	_, err = file.WriteString(result)
	if err != nil {
		return errors.Annotate(err, "while copying in local file")
	}

	fmt.Fprintln(ctx.Stdout, "Bundle successfully exported to", filename)

	return nil
}

func (c *exportBundleCommand) injectTrustFlag(cfgClient ConfigAPI, bundleYaml string) (string, error) {
	var (
		bundleSpec   *charm.BundleData
		appliedPatch bool
	)
	if err := yaml.Unmarshal([]byte(bundleYaml), &bundleSpec); err != nil {
		return "", err
	}

	for appName, appSpec := range bundleSpec.Applications {
		res, err := cfgClient.Get(model.GenerationMaster, appName)
		if err != nil {
			return "", errors.Annotatef(err, "could not retrieve configuration for %q", appName)
		}

		if res.ApplicationConfig == nil {
			continue
		}

		cfgMap, ok := res.ApplicationConfig[appFacade.TrustConfigOptionName].(map[string]interface{})
		if ok && cfgMap["value"] == true {
			appSpec.RequiresTrust = true
			appliedPatch = true
		}
	}

	if !appliedPatch {
		return bundleYaml, nil
	}

	patchedYaml, err := yaml.Marshal(bundleSpec)
	if err != nil {
		return "", err
	}
	return string(patchedYaml), nil
}
