// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/modelconfig"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/environs/config"
)

// ModelConfigClient represents a model config client for requesting model
// configurations.
type ModelConfigClient interface {
	ModelConfigGetter
	Close() error
}

func newCharmHubCommand() *charmHubCommand {
	cmd := &charmHubCommand{
		arches: arch.AllArches(),
	}
	cmd.APIRootFunc = func() (base.APICallCloser, error) {
		return cmd.NewAPIRoot()
	}
	cmd.ModelConfigClientFunc = func(api base.APICallCloser) ModelConfigClient {
		return modelconfig.NewClient(api)
	}
	return cmd
}

type charmHubCommand struct {
	modelcmd.ModelCommandBase

	APIRootFunc           func() (base.APICallCloser, error)
	ModelConfigClientFunc func(base.APICallCloser) ModelConfigClient

	arch   string
	arches arch.Arches
	series string
}

func (c *charmHubCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)

	f.StringVar(&c.arch, "arch", ArchAll, fmt.Sprintf("specify an arch <%s>", c.archArgumentList()))
	f.StringVar(&c.series, "series", SeriesAll, "specify a series")
}

// Init initializes the info command, including validating the provided
// flags. It implements part of the cmd.Command interface.
func (c *charmHubCommand) Init(args []string) error {
	// If the architecture is empty, ensure we normalize it to all to prevent
	// complicated comparison checking.
	if c.arch == "" {
		c.arch = ArchAll
	}

	if c.arch != ArchAll && !c.arches.Contains(c.arch) {
		return errors.Errorf("unexpected architecture flag value %q, expected <%s>", c.arch, c.archArgumentList())
	}

	return nil
}

func (c *charmHubCommand) Run(ctx *cmd.Context) error {
	apiRoot, err := c.APIRootFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer func() { _ = apiRoot.Close() }()

	modelConfigClient := c.ModelConfigClientFunc(apiRoot)
	defer func() { _ = modelConfigClient.Close() }()

	if err := c.verifySeries(modelConfigClient); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (c *charmHubCommand) verifySeries(modelConfigClient ModelConfigGetter) error {
	if c.series != "" {
		return nil
	}

	attrs, err := modelConfigClient.ModelGet()
	if err != nil {
		return errors.Trace(err)
	}
	cfg, err := config.New(config.NoDefaults, attrs)
	if err != nil {
		return errors.Trace(err)
	}
	if defaultSeries, explicit := cfg.DefaultSeries(); explicit {
		c.series = defaultSeries
	}
	return nil
}

func (c *charmHubCommand) archArgumentList() string {
	archList := strings.Join(c.arches.StringList(), "|")
	return fmt.Sprintf("%s|%s", ArchAll, archList)
}
