// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"os"
	"sort"
	"strings"
	"syscall"

	"github.com/juju/charm/v8"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/os/series"

	"github.com/juju/juju/api/modelconfig"
	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/transport"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
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
	return modelcmd.Wrap(&downloadCommand{},
		modelcmd.WrapSkipModelInit,
	)
}

// downloadCommand supplies the "download" CLI command used for downloading
// charm snaps.
type downloadCommand struct {
	modelcmd.ModelCommandBase
	out cmd.Output

	modelConfigAPI ModelConfigGetter
	charmHubClient CharmHubClient

	channel       string
	series        string
	charmHubURL   string
	charmOrBundle string
	archivePath   string
	pipeToStdout  bool
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
	f.StringVar(&c.channel, "channel", "", "specify a channel to use instead of the default release")
	f.StringVar(&c.series, "series", "", "specify a series to use")
	f.StringVar(&c.charmHubURL, "charm-hub-url", "", "override the model config by specifying the charmhub url for querying the store")
	f.StringVar(&c.archivePath, "filepath", "", "filepath location of the charm to download to")
}

// Init initializes the download command, including validating the provided
// flags. It implements part of the cmd.Command interface.
func (c *downloadCommand) Init(args []string) error {
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
			return errors.Annotatef(err, "unexpected charm-hub-url")
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
	var charmHubURL string
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
	if c.pipeToStdout {
		fileSystem = stdoutFileSystem{}
	} else {
		fileSystem = charmhub.DefaultFileSystem()
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

	var (
		found    bool
		revision transport.Revision
	)
	if c.channel != "" {
		charmChannel, err := corecharm.ParseChannel(c.channel)
		if err != nil {
			return errors.Trace(err)
		}

		// If there is no series, then attempt to select the best one first.
		if c.series == "" {
			revision, found = c.locateRevisionByChannel(info.ChannelMap, charmChannel)
		} else {
			// We have a series go attempt to find the channel.
			revision, found = c.locateRevisionByChannelAndSeries(info.ChannelMap, charmChannel, c.series)
		}
	} else if c.series == "" || info.DefaultRelease.Channel.Platform.Series == c.series {
		// If there is no channel, fallback to the default release.
		revision, found = info.DefaultRelease.Revision, true
	}

	if !found {
		if c.series != "" {
			return errors.Errorf("%s %q not found for %s", info.Type, c.charmOrBundle, c.series)
		}
		return errors.Errorf("%s %q not found", info.Type, c.charmOrBundle)
	}

	resourceURL, err := url.Parse(revision.Download.URL)
	if err != nil {
		return errors.Trace(err)
	}

	path := c.archivePath
	if c.archivePath == "" {
		path = fmt.Sprintf("%s.%s", info.Name, info.Type)
	}

	cmdContext.Infof("Fetching %s %q", info.Type, info.Name)

	if err := client.Download(ctx, resourceURL, path); err != nil {
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

func (c *downloadCommand) locateRevisionByChannel(channelMaps []transport.ChannelMap, channel corecharm.Channel) (transport.Revision, bool) {
	// Order the channelMap by the ordered supported controller series. That
	// way we'll always find the newest one first (hopefully the most
	// supported).
	// Then attempt to find the revision by a channel.
	orderedSeries := series.SupportedJujuControllerSeries()
	channelMap := channelMapBySeries{
		channelMap: channelMaps,
		series:     orderedSeries,
	}
	sort.Sort(channelMap)

	for _, channelMap := range channelMap.channelMap {
		if rev, ok := locateRevisionByChannelMap(channelMap, channel); ok {
			return rev, true
		}
	}
	return transport.Revision{}, false
}

func (c *downloadCommand) locateRevisionByChannelAndSeries(channelMaps []transport.ChannelMap, channel corecharm.Channel, series string) (transport.Revision, bool) {
	// Filter out any channels that aren't of a given series.
	var filtered []transport.ChannelMap
	for _, channelMap := range channelMaps {
		if channelMap.Channel.Platform.Series == series {
			filtered = append(filtered, channelMap)
		}
	}

	// If we don't have any filtered series then we don't know what to do here.
	if len(filtered) == 0 {
		return transport.Revision{}, false
	}

	for _, channelMap := range filtered {
		if rev, ok := locateRevisionByChannelMap(channelMap, channel); ok {
			return rev, true
		}
	}
	return transport.Revision{}, false
}

func locateRevisionByChannelMap(channelMap transport.ChannelMap, channel corecharm.Channel) (transport.Revision, bool) {
	rawChannel := fmt.Sprintf("%s/%s", channelMap.Channel.Track, channelMap.Channel.Risk)
	if strings.HasPrefix(rawChannel, "/") {
		rawChannel = rawChannel[1:]
	}
	charmChannel, err := corecharm.ParseChannel(rawChannel)
	if err != nil {
		return transport.Revision{}, false
	}

	fmt.Println(charmChannel, channelMap.Channel.Platform.Series)
	// Check that we're an exact match.
	if channel.Track == charmChannel.Track && channel.Risk == charmChannel.Risk {
		return channelMap.Revision, true
	}

	return transport.Revision{}, false
}

type channelMapBySeries struct {
	channelMap []transport.ChannelMap
	series     []string
}

func (s channelMapBySeries) Len() int {
	return len(s.channelMap)
}

func (s channelMapBySeries) Swap(i, j int) {
	s.channelMap[i], s.channelMap[j] = s.channelMap[j], s.channelMap[i]
}

func (s channelMapBySeries) Less(i, j int) bool {
	idx1 := s.invertedIndexOf(s.channelMap[i].Channel.Platform.Series)
	idx2 := s.invertedIndexOf(s.channelMap[j].Channel.Platform.Series)
	return idx1 > idx2
}

func (s channelMapBySeries) invertedIndexOf(value string) int {
	for k, i := range s.series {
		if i == value {
			return len(s.series) - k
		}
	}
	return math.MinInt64
}
