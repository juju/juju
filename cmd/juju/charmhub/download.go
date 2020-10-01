// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"net/url"
	"os"
	"syscall"

	"github.com/juju/charm/v8"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/modelconfig"
	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/transport"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/config"
)

const (
	downloadSummary = "Locates and then downloads a CharmHub charm."
	downloadDoc     = `
Download a charm from the CharmHub store by a specified name.

Examples:
    juju download postgresql

See also:
	info
	find
`
)

// NewDownloadCommand wraps downloadCommand with sane model settings.
func NewDownloadCommand() cmd.Command {
	return modelcmd.Wrap(&downloadCommand{})
}

// downloadCommand supplies the "download" CLI command used for downloading
// charm snaps.
type downloadCommand struct {
	modelcmd.ModelCommandBase
	out cmd.Output

	modelConfigAPI ModelConfigGetter
	charmHubClient CharmHubClient

	charmHubURL   string
	charmOrBundle string
	archivePath   string
}

// Info returns help related download about the command, it implements
// part of the cmd.Command interface.
func (c *downloadCommand) Info() *cmd.Info {
	download := &cmd.Info{
		Name:    "download",
		Args:    "[options] <charm>",
		Purpose: downloadSummary,
		Doc:     downloadDoc,
	}
	return jujucmd.Info(download)
}

// SetFlags defines flags which can be used with the download command.
// It implements part of the cmd.Command interface.
func (c *downloadCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.StringVar(&c.charmHubURL, "charm-hub-url", "", "specify url for querying the charmhub store")
	f.StringVar(&c.archivePath, "path", "", "path to download the charm to")
}

// Init initializes the download command, including validating the provided
// flags. It implements part of the cmd.Command interface.
func (c *downloadCommand) Init(args []string) error {
	if len(args) != 1 {
		return errors.Errorf("expected a charm or bundle name")
	}
	if err := c.validateCharmOrBundle(args[0]); err != nil {
		return errors.Trace(err)
	}
	c.charmOrBundle = args[0]
	return nil
}

func (c *downloadCommand) validateCharmOrBundle(charmOrBundle string) error {
	curl, err := charm.ParseURL(charmOrBundle)
	if err != nil {
		return errors.Trace(err)
	}
	if !charm.CharmHub.Matches(curl.Schema) {
		return errors.Errorf("%q is not a Charm Hub charm", charmOrBundle)
	}
	return nil
}

// Run is the business logic of the download command.  It implements the meaty
// part of the cmd.Command interface.
func (c *downloadCommand) Run(cmdContext *cmd.Context) error {
	var charmHubURL string
	if c.charmHubURL != "" {
		charmHubURL = c.charmHubURL
	} else {
		config, err := c.getModelConfig()
		if err != nil {
			return errors.Trace(err)
		}

		charmHubURL, _ = config.CharmHubURL()
	}

	config, err := charmhub.CharmHubConfigFromURL(charmHubURL, downloadLogger{
		Context: cmdContext,
	})
	if err != nil {
		return errors.Trace(err)
	}

	var fileSystem charmhub.FileSystem
	if c.archivePath != "" {
		fileSystem = charmhub.DefaultFileSystem()
	} else {
		fileSystem = stdoutFileSystem{}
	}

	client, err := c.getCharmHubClient(config, fileSystem)
	if err != nil {
		return errors.Trace(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	info, err := client.Info(ctx, c.charmOrBundle)
	if err != nil {
		return errors.Trace(err)
	}

	// TODO (stickupkid): All the user to specify the location of the download
	// path.
	resourceURL, err := url.Parse(info.DefaultRelease.Revision.Download.URL)
	if err != nil {
		return errors.Trace(err)
	}

	return client.Download(ctx, resourceURL, c.archivePath)
}

// getAPI returns the API that supplies methods
// required to execute this command.
func (c *downloadCommand) getAPI() (ModelConfigGetter, error) {
	if c.modelConfigAPI != nil {
		// This is for testing purposes, for testing, both values
		// should be set.
		return c.modelConfigAPI, nil
	}
	api, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Annotate(err, "opening API connection")
	}
	return modelconfig.NewClient(api), nil
}

func (c *downloadCommand) getModelConfig() (*config.Config, error) {
	api, err := c.getAPI()
	if err != nil {
		return nil, errors.Trace(err)
	}
	attrs, err := api.ModelGet()
	if err != nil {
		return nil, errors.Wrap(err, errors.New("cannot fetch model settings"))
	}

	return config.New(config.NoDefaults, attrs)
}

func (c *downloadCommand) getCharmHubClient(config charmhub.Config, fileSystem charmhub.FileSystem) (CharmHubClient, error) {
	if c.charmHubClient != nil {
		return c.charmHubClient, nil
	}
	return charmhub.NewClientWithFileSystem(config, fileSystem)
}

// CharmHubClient defines a charmhub client, used for querying the charmhub
// store.
type CharmHubClient interface {
	// Info returns charm info on the provided charm name from CharmHub API.
	Info(context.Context, string) (transport.InfoResponse, error)

	// Download defines a client for downloading charms directly.
	Download(context.Context, *url.URL, string) error
}

type downloadLogger struct {
	Context *cmd.Context
}

func (d downloadLogger) IsTraceEnabled() bool {
	return !d.Context.Quiet()
}

func (d downloadLogger) Debugf(msg string, args ...interface{}) {
	d.Context.Verbosef(msg, args...)
}

func (d downloadLogger) Tracef(msg string, args ...interface{}) {
}

type stdoutFileSystem struct {
}

// Create creates or truncates the named file. If the file already exists,
// it is truncated.
func (stdoutFileSystem) Create(string) (*os.File, error) {
	return os.NewFile(uintptr(syscall.Stdout), "/dev/stdout"), nil
}
