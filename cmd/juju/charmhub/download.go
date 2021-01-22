// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"syscall"

	"github.com/juju/charm/v8"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/os/v2/series"

	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/selector"
	"github.com/juju/juju/charmhub/transport"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output/progress"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/environs/config"
)

const (
	downloadSummary = "Locates and then downloads a CharmHub charm."
	downloadDoc     = `
Download a charm to the current directory from the CharmHub store
by a specified name.

Adding a hyphen as the second argument allows the download to be piped
to stdout.

Examples:
    juju download postgresql
    juju download postgresql - > postgresql.charm

See also:
    info
    find
`
)

// NewDownloadCommand wraps downloadCommand with sane model settings.
func NewDownloadCommand() cmd.Command {
	return modelcmd.Wrap(&downloadCommand{
		charmHubCommand: newCharmHubCommand(),
		orderedSeries:   series.SupportedJujuControllerSeries(),
		CharmHubClientFunc: func(config charmhub.Config, fs charmhub.FileSystem) (DownloadCommandAPI, error) {
			return charmhub.NewClientWithFileSystem(config, fs)
		},
	}, modelcmd.WrapSkipModelInit)
}

// downloadCommand supplies the "download" CLI command used for downloading
// charm snaps.
type downloadCommand struct {
	*charmHubCommand

	CharmHubClientFunc func(charmhub.Config, charmhub.FileSystem) (DownloadCommandAPI, error)

	out cmd.Output

	channel       string
	charmHubURL   string
	charmOrBundle string
	archivePath   string
	pipeToStdout  bool

	orderedSeries []string
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
	c.charmHubCommand.SetFlags(f)

	f.StringVar(&c.channel, "channel", "", "specify a channel to use instead of the default release")
	f.StringVar(&c.charmHubURL, "charmhub-url", "", "override the model config by specifying the Charmhub URL for querying the store")
	f.StringVar(&c.archivePath, "filepath", "", "filepath location of the charm to download to")
}

// Init initializes the download command, including validating the provided
// flags. It implements part of the cmd.Command interface.
func (c *downloadCommand) Init(args []string) error {
	if err := c.charmHubCommand.Init(args); err != nil {
		return errors.Trace(err)
	}

	if len(args) < 1 || len(args) > 2 {
		return errors.Errorf("expected a charm or bundle name")
	}
	if len(args) == 2 {
		if args[1] != "-" {
			return errors.Errorf("expected a charm or bundle name, followed by hyphen to pipe to stdout")
		}
		c.pipeToStdout = true
	}

	if err := c.validateCharmOrBundle(args[0]); err != nil {
		return errors.Trace(err)
	}
	c.charmOrBundle = args[0]

	if c.charmHubURL != "" {
		_, err := url.ParseRequestURI(c.charmHubURL)
		if err != nil {
			return errors.Annotatef(err, "unexpected charmhub-url")
		}
	}

	return nil
}

func (c *downloadCommand) validateCharmOrBundle(charmOrBundle string) error {
	curl, err := charm.ParseURL(charmOrBundle)
	if err != nil {
		return errors.Annotatef(err, "unexpected charm or bundle name")
	}
	if !charm.CharmHub.Matches(curl.Schema) {
		return errors.Errorf("%q is not a Charm Hub charm", charmOrBundle)
	}
	return nil
}

// Run is the business logic of the download command.  It implements the meaty
// part of the cmd.Command interface.
func (c *downloadCommand) Run(cmdContext *cmd.Context) error {
	var (
		err         error
		charmHubURL string
	)
	if c.charmHubURL != "" {
		charmHubURL = c.charmHubURL
	} else {
		// This is a horrible workaround for the fact that this command can work
		// with and without a bootstrapped controller.
		// To correctly handle the fact that we want to lazily connect to a
		// controller, we have to grab the model identifier once we know what
		// we want to do (based on the flags) and then call the init the model
		// callstack.
		// The reason this exists is because everything is curated for you, but
		// when we do need to customize this workflow, it unfortunately gets in
		// the way.
		modelIdentifier, _ := c.ModelCommandBase.ModelIdentifier()
		if err := c.ModelCommandBase.SetModelIdentifier(modelIdentifier, true); err != nil {
			return errors.Trace(err)
		}

		if err := c.charmHubCommand.Run(cmdContext); err != nil {
			return errors.Trace(err)
		}

		charmHubURL, err = c.getCharmHubURL()
		if err != nil {
			if errors.IsNotImplemented(err) {
				cmdContext.Warningf("juju download not supported with controllers < 2.9")
				return nil
			}
			return errors.Trace(err)
		}
	}

	config, err := charmhub.CharmHubConfigFromURL(charmHubURL, downloadLogger{
		Context: cmdContext,
	})
	if err != nil {
		return errors.Trace(err)
	}

	var fileSystem charmhub.FileSystem
	if c.pipeToStdout {
		fileSystem = stdoutFileSystem{}
	} else {
		fileSystem = charmhub.DefaultFileSystem()
	}

	client, err := c.CharmHubClientFunc(config, fileSystem)
	if err != nil {
		return errors.Trace(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	info, err := client.Info(ctx, c.charmOrBundle)
	if err != nil {
		return errors.Trace(err)
	}

	channel := c.channel
	// Locate a release that we would expect to be default. In this case
	// we want to fall back to latest/stable. We don't want to use the
	// info.DefaultRelease here as that isn't actually the default release,
	// but instead the last release and that's not what we want.
	if channel == "" {
		channel = corecharm.DefaultChannelString
	}
	charmChannel, err := corecharm.ParseChannelNormalize(channel)
	if err != nil {
		return errors.Trace(err)
	}

	selector := selector.NewSelectorForDownload(c.orderedSeries)
	resourceURL, origin, err := selector.Locate(info, corecharm.Origin{
		Channel: &charmChannel,
		Platform: corecharm.Platform{
			Architecture: c.arch,
			Series:       c.series,
		},
	})
	if err != nil {
		return errors.Trace(err)
	}

	path := c.archivePath
	if c.archivePath == "" {
		path = fmt.Sprintf("%s.%s", info.Name, info.Type)
	}

	cmdContext.Infof("Fetching %s %q using %q channel at revision %d", info.Type, info.Name, charmChannel, origin.Revision)

	pb := progress.MakeProgressBar(cmdContext.Stdout)
	ctx = context.WithValue(ctx, charmhub.DownloadNameKey, info.Name)
	if err := client.Download(ctx, resourceURL, path, charmhub.WithProgressBar(pb)); err != nil {
		return errors.Trace(err)
	}

	// If we're piping to stdout, then we don't need to mention how to install
	// and deploy the charm.
	if c.pipeToStdout {
		cmdContext.Infof("Downloading of %s complete", info.Type)
		return nil
	}

	if !strings.HasPrefix(path, "/") {
		path = fmt.Sprintf("./%s", path)
	}

	cmdContext.Infof(`
Install the %q %s with:
    juju deploy %s`[1:], info.Name, info.Type, path)

	return nil
}

func (c *downloadCommand) getCharmHubURL() (string, error) {
	apiRoot, err := c.APIRootFunc()
	if err != nil {
		return "", errors.Trace(err)
	}
	defer func() { _ = apiRoot.Close() }()

	if apiRoot.BestFacadeVersion("CharmHub") < 1 {
		return "", errors.NotImplementedf("charmhub")
	}

	modelConfigClient := c.ModelConfigClientFunc(apiRoot)
	defer func() { _ = modelConfigClient.Close() }()

	attrs, err := modelConfigClient.ModelGet()
	if err != nil {
		return "", errors.Trace(err)
	}

	config, err := config.New(config.NoDefaults, attrs)
	if err != nil {
		return "", errors.Trace(err)
	}

	charmHubURL, _ := config.CharmHubURL()
	return charmHubURL, nil
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

func (d downloadLogger) Errorf(msg string, args ...interface{}) {
	d.Context.Verbosef(msg, args...)
}

func (d downloadLogger) Debugf(msg string, args ...interface{}) {
	d.Context.Verbosef(msg, args...)
}

func (d downloadLogger) Tracef(msg string, args ...interface{}) {}

type stdoutFileSystem struct{}

// Create creates or truncates the named file. If the file already exists,
// it is truncated.
func (stdoutFileSystem) Create(string) (*os.File, error) {
	return os.NewFile(uintptr(syscall.Stdout), "/dev/stdout"), nil
}
