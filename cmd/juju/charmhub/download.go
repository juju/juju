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
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/os/v2/series"

	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/transport"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/environs/config"
)

const (
	// DefaultReleaseChannel is the channel we should look at if a user didn't
	// specify a channel.
	DefaultReleaseChannel = "latest/stable"
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
	f.StringVar(&c.charmHubURL, "charm-hub-url", "", "override the model config by specifying the charmhub url for querying the store")
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

	// Locate a release that we would expect to be default. In this case
	// we want to fall back to latest/stable. We don't want to use the
	// info.DefaultRelease here as that isn't actually the default release,
	// but instead the last release and that's not what we want.
	if c.channel == "" {
		c.channel = DefaultReleaseChannel
	}
	charmChannel, err := corecharm.ParseChannelNormalize(c.channel)
	if err != nil {
		return errors.Trace(err)
	}

	var (
		filterFn  FilterInfoChannelMapFunc
		selectAll = c.arch == ArchAll && c.series == SeriesAll
	)
	switch {
	case selectAll:
		// Select every channel for all architectures and all series.
		filterFn = func(transport.InfoChannelMap) bool {
			return true
		}
	case c.arch != ArchAll && c.series == SeriesAll:
		filterFn = filterByArchitecture(c.arch)
	case c.arch == ArchAll && c.series != SeriesAll:
		filterFn = filterBySeries(c.series)
	default:
		filterFn = filterByArchitectureAndSeries(c.arch, c.series)
	}

	// To allow users to download from closed channels (same UX for snaps), we
	// link all closed channels to their parent channel, if their parent channel
	// is also closed, we move to the next.
	// We do the linking here because we want to ensure that we only ever filter
	// once ensuring we're always correct.
	channelMap, err := linkClosedChannels(info.ChannelMap)
	if err != nil {
		return errors.Trace(err)
	}
	channelMap = filterInfoChannelMap(channelMap, filterFn)
	revisions, found := locateRevisionByChannel(c.sortInfoChannelMap(channelMap), charmChannel)
	if !found {
		if c.series != "" {
			return errors.Errorf("%s %q not found for %q channel matching %q series", info.Type, c.charmOrBundle, c.channel, c.series)
		}
		return errors.Errorf("%s %q not found with in the channel %q", info.Type, c.charmOrBundle, c.channel)
	}

	if selectAll {
		// Ensure we have just one architecture.
		if arches := listArchitectures(revisions); len(arches) > 1 {
			list := strings.Join(arches, ",")
			return errors.Errorf("multiple architectures (%s) found, use --arch flag to specify the one to download", list)
		}
	}
	if len(revisions) == 0 {
		return errors.Errorf("%s %q not found with in the channel %q", info.Type, c.charmOrBundle, c.channel)
	}

	revision := revisions[0]
	resourceURL, err := url.Parse(revision.Download.URL)
	if err != nil {
		return errors.Trace(err)
	}

	path := c.archivePath
	if c.archivePath == "" {
		path = fmt.Sprintf("%s.%s", info.Name, info.Type)
	}

	cmdContext.Infof("Fetching %s %q using %q channel at revision %d", info.Type, info.Name, charmChannel, revision.Revision)

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

func (c *downloadCommand) getCharmHubURL() (string, error) {
	apiRoot, err := c.APIRootFunc()
	if err != nil {
		return "", errors.Trace(err)
	}
	defer func() { _ = apiRoot.Close() }()

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

func (c *downloadCommand) sortInfoChannelMap(in []transport.InfoChannelMap) []transport.InfoChannelMap {
	// Order the channelMap by the ordered supported controller series. That
	// way we'll always find the newest one first (hopefully the most
	// supported).
	// Then attempt to find the revision by a channel.
	channelMap := channelMapBySeries{
		channelMap: in,
		series:     c.orderedSeries,
	}
	sort.Sort(channelMap)

	return channelMap.channelMap
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

func (d downloadLogger) Tracef(msg string, args ...interface{}) {}

type stdoutFileSystem struct {
}

// Create creates or truncates the named file. If the file already exists,
// it is truncated.
func (stdoutFileSystem) Create(string) (*os.File, error) {
	return os.NewFile(uintptr(syscall.Stdout), "/dev/stdout"), nil
}

func filterByArchitecture(arch string) FilterInfoChannelMapFunc {
	return func(channelMap transport.InfoChannelMap) bool {
		if arch == ArchAll {
			return true
		}

		platformArch := channelMap.Channel.Platform.Architecture
		return (platformArch == arch || platformArch == ArchAll) ||
			isArchInPlatforms(channelMap.Revision.Platforms, arch) ||
			isArchInPlatforms(channelMap.Revision.Platforms, ArchAll)
	}
}

func filterBySeries(series string) FilterInfoChannelMapFunc {
	return func(channelMap transport.InfoChannelMap) bool {
		if series == SeriesAll {
			return true
		}

		platformSeries := channelMap.Channel.Platform.Series
		return (platformSeries == series || platformSeries == SeriesAll) ||
			isSeriesInPlatforms(channelMap.Revision.Platforms, series) ||
			isSeriesInPlatforms(channelMap.Revision.Platforms, SeriesAll)
	}
}

func filterByArchitectureAndSeries(arch, series string) FilterInfoChannelMapFunc {
	return func(channelMap transport.InfoChannelMap) bool {
		return filterByArchitecture(arch)(channelMap) &&
			filterBySeries(series)(channelMap)
	}
}

// FilterInfoChannelMapFunc is a type alias for representing a filter function.
type FilterInfoChannelMapFunc func(channelMap transport.InfoChannelMap) bool

func filterInfoChannelMap(in []transport.InfoChannelMap, fn FilterInfoChannelMapFunc) []transport.InfoChannelMap {
	var filtered []transport.InfoChannelMap
	for _, channelMap := range in {
		if !fn(channelMap) {
			continue
		}
		filtered = append(filtered, channelMap)
	}
	return filtered
}

func locateRevisionByChannel(channelMaps []transport.InfoChannelMap, channel corecharm.Channel) ([]transport.InfoRevision, bool) {
	var (
		revisions []transport.InfoRevision
		found     bool
	)
	for _, channelMap := range channelMaps {
		if rev, ok := locateRevisionByChannelMap(channelMap, channel); ok {
			revisions = append(revisions, rev)
			found = true
		}
	}
	return revisions, found
}

func listArchitectures(revisions []transport.InfoRevision) []string {
	arches := set.NewStrings()
	for _, rev := range revisions {
		for _, platform := range rev.Platforms {
			arches.Add(platform.Architecture)
		}
	}
	return arches.SortedValues()
}

type channelMapIndex struct {
	witnessed  bool
	channelMap transport.InfoChannelMap
}

func linkClosedChannels(channelMaps []transport.InfoChannelMap) ([]transport.InfoChannelMap, error) {
	witness := make(map[string][]channelMapIndex)
	for _, channelMap := range channelMaps {
		track := channelMap.Channel.Track
		if witness[track] == nil {
			witness[track] = make([]channelMapIndex, len(channelRisks))
		}
		index := riskIndex(channelMap)
		if index < 0 {
			return nil, errors.Errorf("invalid risk %q", channelMap.Channel.Risk)
		}
		witness[track][index] = channelMapIndex{
			witnessed:  true,
			channelMap: channelMap,
		}
	}

	secondary := make(map[string][]transport.InfoChannelMap)
	for track, channels := range witness {
		secondary[track] = make([]transport.InfoChannelMap, len(channelRisks))

		for i, channel := range channels {
			if channel.witnessed {
				secondary[track][i] = channel.channelMap
				continue
			}

			k := i
			for ; k >= 0; k-- {
				if channels[k].witnessed {
					break
				}
			}

			if k == -1 {
				continue
			}

			link := channels[k].channelMap
			link.Channel.Risk = channelRisks[i]
			secondary[track][i] = link
		}
	}

	var result []transport.InfoChannelMap
	for _, risks := range secondary {
		for _, channel := range risks {
			result = append(result, channel)
		}
	}
	return result, nil
}

func riskIndex(ch transport.InfoChannelMap) int {
	for i, risk := range channelRisks {
		if risk == ch.Channel.Risk {
			return i
		}
	}
	return -1
}

func isSeriesInPlatforms(platforms []transport.Platform, series string) bool {
	for _, platform := range platforms {
		if platform.Series == series {
			return true
		}
	}
	return false
}

func isArchInPlatforms(platforms []transport.Platform, arch string) bool {
	for _, platform := range platforms {
		if platform.Architecture == arch {
			return true
		}
	}
	return false
}

func constructChannelFromTrackAndRisk(track, risk string) (corecharm.Channel, error) {
	rawChannel := fmt.Sprintf("%s/%s", track, risk)
	if strings.HasPrefix(rawChannel, "/") {
		rawChannel = rawChannel[1:]
	} else if strings.HasSuffix(rawChannel, "/") {
		rawChannel = rawChannel[:len(rawChannel)-1]
	}
	return corecharm.ParseChannelNormalize(rawChannel)
}

func locateRevisionByChannelMap(channelMap transport.InfoChannelMap, channel corecharm.Channel) (transport.InfoRevision, bool) {
	charmChannel, err := constructChannelFromTrackAndRisk(channelMap.Channel.Track, channelMap.Channel.Risk)
	if err != nil {
		return transport.InfoRevision{}, false
	}

	// Check that we're an exact match.
	if channel.Track == charmChannel.Track && channel.Risk == charmChannel.Risk {
		return channelMap.Revision, true
	}

	return transport.InfoRevision{}, false
}

type channelMapBySeries struct {
	channelMap []transport.InfoChannelMap
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
