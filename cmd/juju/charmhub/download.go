// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"syscall"

	"github.com/juju/charm/v11"
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"

	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/transport"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/output/progress"
	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/series"
	coreseries "github.com/juju/juju/core/series"
	"github.com/juju/juju/version"
)

const (
	downloadSummary = "Locates and then downloads a CharmHub charm."
	downloadDoc     = `
Download a charm to the current directory from the CharmHub store
by a specified name. Downloading for a specific base can be done via
--base. --base can be specified using the OS name and the version of
the OS, separated by @. For example, --base ubuntu@22.04.

Adding a hyphen as the second argument allows the download to be piped
to stdout.
`

	downloadExamples = `
    juju download postgresql
    juju download postgresql --no-progress - > postgresql.charm
`
)

// NewDownloadCommand wraps downloadCommand with sane model settings.
func NewDownloadCommand() cmd.Command {
	return &downloadCommand{
		charmHubCommand: newCharmHubCommand(),
	}
}

// downloadCommand supplies the "download" CLI command used for downloading
// charm snaps.
type downloadCommand struct {
	*charmHubCommand

	channel       string
	charmOrBundle string
	archivePath   string
	pipeToStdout  bool
	noProgress    bool
}

// Info returns help related download about the command, it implements
// part of the cmd.Command interface.
func (c *downloadCommand) Info() *cmd.Info {
	download := &cmd.Info{
		Name:     "download",
		Args:     "[options] <charm>",
		Purpose:  downloadSummary,
		Doc:      downloadDoc,
		Examples: downloadExamples,
		SeeAlso: []string{
			"info",
			"find",
		},
	}
	return jujucmd.Info(download)
}

// SetFlags defines flags which can be used with the download command.
// It implements part of the cmd.Command interface.
func (c *downloadCommand) SetFlags(f *gnuflag.FlagSet) {
	c.charmHubCommand.SetFlags(f)

	f.StringVar(&c.arch, "arch", ArchAll, fmt.Sprintf("specify an arch <%s>", c.archArgumentList()))
	f.StringVar(&c.series, "series", SeriesAll, "specify a series. DEPRECATED use --base")
	f.StringVar(&c.base, "base", "", "specify a base")
	f.StringVar(&c.channel, "channel", "", "specify a channel to use instead of the default release")
	f.StringVar(&c.archivePath, "filepath", "", "filepath location of the charm to download to")
	f.BoolVar(&c.noProgress, "no-progress", false, "disable the progress bar")
}

// Init initializes the download command, including validating the provided
// flags. It implements part of the cmd.Command interface.
func (c *downloadCommand) Init(args []string) error {
	if c.base != "" && (c.series != "" && c.series != SeriesAll) {
		return errors.New("--series and --base cannot be specified together")
	}

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

	curl, err := c.validateCharmOrBundle(args[0])
	if err != nil {
		return errors.Trace(err)
	}
	// Allow for both <charm> and ch:<charm> to download.
	c.charmOrBundle = curl.Name

	return nil
}

func (c *downloadCommand) validateCharmOrBundle(charmOrBundle string) (*charm.URL, error) {
	curl, err := charm.ParseURL(charmOrBundle)
	if err != nil {
		logger.Debugf("%s", err)
		return nil, errors.NotValidf("charm or bundle name, %q, is", charmOrBundle)
	}
	if !charm.CharmHub.Matches(curl.Schema) {
		return nil, errors.Errorf("%q is not a Charmhub charm", charmOrBundle)
	}
	return curl, nil
}

// Run is the business logic of the download command.  It implements the meaty
// part of the cmd.Command interface.
func (c *downloadCommand) Run(cmdContext *cmd.Context) error {
	var (
		base series.Base
		err  error
	)
	// Note: we validated that both series and base cannot be specified in
	// Init(), so it's safe to assume that only one of them is set here.
	if c.series == SeriesAll {
		c.series = ""
	} else if c.series != "" {
		cmdContext.Warningf("series flag is deprecated, use --base instead")
		if base, err = series.GetBaseFromSeries(c.series); err != nil {
			return errors.Annotatef(err, "attempting to convert %q to a base", c.series)
		}
		c.base = base.String()
		c.series = ""
	}
	if c.base != "" {
		if base, err = series.ParseBaseFromString(c.base); err != nil {
			return errors.Trace(err)
		}
	}

	cfg := charmhub.Config{
		URL:    c.charmHubURL,
		Logger: downloadLogger{Context: cmdContext},
	}

	if c.pipeToStdout {
		cfg.FileSystem = stdoutFileSystem{}
	}

	client, err := c.CharmHubClientFunc(cfg)
	if err != nil {
		return errors.Trace(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Locate a release that we would expect to be default. In this case
	// we want to fall back to latest/stable.
	channel := c.channel
	if channel == "" {
		channel = corecharm.DefaultChannelString
	}
	normChannel, err := charm.ParseChannelNormalize(channel)
	if err != nil {
		return errors.Trace(err)
	}

	pArch := c.arch
	if pArch == "all" || pArch == "" {
		pArch = arch.DefaultArchitecture
	}
	if base.Empty() {
		base = version.DefaultSupportedLTSBase()
	}

	results, normBase, err := c.refresh(ctx, cmdContext, client, normChannel, pArch, base, true)
	if err != nil {
		return errors.Trace(err)
	}

	// In theory we can get multiple responses from the refresh API, but in
	// reality if we only request one action, we only get one result. If that
	// happens not to be the case, just select the first one.
	result := results[0]
	entity := result.Entity
	entityType := entity.Type
	entitySHA := entity.Download.HashSHA256

	path := c.archivePath
	if path == "" {
		// Use the revision number to create a unique path for every download.
		path = fmt.Sprintf("%s_r%d.%s", entity.Name, entity.Revision, entityType)
	}

	cmdContext.Infof("Fetching %s %q revision %d using %q channel and base %q",
		entityType, entity.Name, entity.Revision, normChannel, normBase)

	resourceURL, err := url.Parse(entity.Download.URL)
	if err != nil {
		return errors.Trace(err)
	}

	ctx = context.WithValue(ctx, charmhub.DownloadNameKey, entity.Name)

	if c.noProgress {
		err = client.Download(ctx, resourceURL, path)
	} else {
		pb := progress.MakeProgressBar(cmdContext.Stdout)
		err = client.Download(ctx, resourceURL, path, charmhub.WithProgressBar(pb))
	}
	if err != nil {
		return errors.Trace(err)
	}

	// If we're piping to stdout, then we don't need to mention how to install
	// and deploy the charm.
	if c.pipeToStdout {
		cmdContext.Infof("Downloading of %s complete", entityType)
		return nil
	}

	// Ensure we calculate the hash of the file.
	calculatedHash, err := c.calculateHash(path)
	if err != nil {
		return errors.Trace(err)
	}
	if calculatedHash != entitySHA {
		return errors.Errorf(`Checksum of download failed for %q:
Expected:   %s
Calculated: %s`, c.charmOrBundle, entitySHA, calculatedHash)
	}

	if !strings.HasPrefix(path, "/") {
		path = fmt.Sprintf("./%s", path)
	}

	cmdContext.Infof(`
Install the %q %s with:
    juju deploy %s`[1:], entity.Name, entityType, path)

	return nil
}

func (c *downloadCommand) refresh(
	ctx context.Context, cmdContext *cmd.Context,
	client CharmHubClient,
	normChannel charm.Channel,
	arch string,
	base series.Base,
	retrySuggested bool,
) ([]transport.RefreshResponse, *corecharm.Platform, error) {
	platform := fmt.Sprintf("%s/%s/%s", arch, base.OS, base.Channel.Track)
	normBase, err := corecharm.ParsePlatformNormalize(platform)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	refreshConfig, err := charmhub.InstallOneFromChannel(c.charmOrBundle, normChannel.String(), charmhub.RefreshBase{
		Architecture: normBase.Architecture,
		Name:         normBase.OS,
		Channel:      normBase.Channel,
	})
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	results, err := client.Refresh(ctx, refreshConfig)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	if len(results) == 0 {
		return nil, nil, errors.NotFoundf(c.charmOrBundle)
	}

	// Ensure we didn't get any errors whilst querying the charmhub API
	for _, res := range results {
		if res.Error != nil {
			if res.Error.Code == transport.ErrorCodeRevisionNotFound {
				possibleBases, err := c.suggested(cmdContext, base, normChannel.String(), res.Error.Extra.Releases)
				// The following will attempt to refresh the charm with the
				// suggested series. If it can't do that, it will give up after
				// the second attempt.
				if retrySuggested && errors.Is(err, errors.NotSupported) && len(possibleBases) > 0 {
					cmdContext.Infof("Base %q is not supported for charm %q, trying base %q", base.DisplayString(), c.charmOrBundle, possibleBases[0].DisplayString())
					return c.refresh(ctx, cmdContext, client, normChannel, arch, possibleBases[0], false)
				}
				return nil, nil, errors.Trace(err)
			}
			return nil, nil, errors.Errorf("unable to locate %s: %s", c.charmOrBundle, res.Error.Message)
		}
	}

	return results, &normBase, nil
}

func (c *downloadCommand) suggested(cmdContext *cmd.Context, requestedBase coreseries.Base, channel string, releases []transport.Release) ([]coreseries.Base, error) {
	var (
		ordered []coreseries.Base
		bases   = make(map[coreseries.Base]struct{})
	)
	for _, rel := range releases {
		if rel.Channel == channel {
			parsedBase, err := coreseries.ParseBase(rel.Base.Name, rel.Base.Channel)
			if err != nil {
				// Shouldn't happen, log and continue if verbose is set.
				cmdContext.Verbosef("%s of %s", err, rel.Base.Name)
				continue
			}
			if _, ok := bases[parsedBase]; !ok {
				ordered = append(ordered, parsedBase)
			}
			bases[parsedBase] = struct{}{}
		}
	}
	if len(bases) == 0 {
		// No releases in this channel
		return nil, errors.Errorf(`%q has no releases in channel %q. Type
    juju info %s
for a list of supported channels.`,
			c.charmOrBundle, channel, c.charmOrBundle)
	}

	orderedBaseStrings := make([]string, len(ordered))
	for i, base := range ordered {
		orderedBaseStrings[i] = base.DisplayString()
	}

	return ordered, errors.NewNotSupported(nil, fmt.Sprintf("%q does not support base %q in channel %q. Supported bases are: %s.",
		c.charmOrBundle, requestedBase.DisplayString(), channel, strings.Join(orderedBaseStrings, ", ")))
}

func (c *downloadCommand) calculateHash(path string) (string, error) {
	file, err := c.Filesystem().Open(path)
	if err != nil {
		return "", errors.Trace(err)
	}
	defer func() { _ = file.Close() }()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", errors.Annotatef(err, "unable to hash file for checksum")
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
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

func (d downloadLogger) ChildWithLabels(name string, labels ...string) loggo.Logger {
	return logger.ChildWithLabels(name, labels...)
}

type stdoutFileSystem struct{}

// Create creates or truncates the named file. If the file already exists,
// it is truncated.
func (stdoutFileSystem) Create(string) (*os.File, error) {
	return os.NewFile(uintptr(syscall.Stdout), "/dev/stdout"), nil
}
