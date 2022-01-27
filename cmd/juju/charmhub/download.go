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

	"github.com/juju/charm/v8"
	"github.com/juju/cmd/v3"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	"github.com/juju/os/v2/series"

	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/transport"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/output/progress"
	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
	coreseries "github.com/juju/juju/core/series"
	"github.com/juju/juju/version"
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
	return &downloadCommand{
		charmHubCommand: newCharmHubCommand(),
		orderedSeries:   series.SupportedJujuControllerSeries(),
	}
}

// downloadCommand supplies the "download" CLI command used for downloading
// charm snaps.
type downloadCommand struct {
	*charmHubCommand

	out cmd.Output

	channel       string
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

	f.StringVar(&c.arch, "arch", ArchAll, fmt.Sprintf("specify an arch <%s>", c.archArgumentList()))
	f.StringVar(&c.series, "series", SeriesAll, "specify a series")
	f.StringVar(&c.channel, "channel", "", "specify a channel to use instead of the default release")
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
	config, err := charmhub.CharmHubConfigFromURL(c.charmHubURL, downloadLogger{
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

	// Locate a release that we would expect to be default. In this case
	// we want to fall back to latest/stable. We don't want to use the
	// info.DefaultRelease here as that isn't actually the default release,
	// but instead the last release and that's not what we want.
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
	pSeries := c.series
	if pSeries == "all" || pSeries == "" {
		pSeries = version.DefaultSupportedLTS()
	}
	platform := fmt.Sprintf("%s/%s", pArch, pSeries)
	normBase, err := corecharm.ParsePlatformNormalize(platform)
	if err != nil {
		return errors.Trace(err)
	}

	if normBase.Series != "" {
		sys, err := series.GetOSFromSeries(normBase.Series)
		if err != nil {
			return errors.Trace(err)
		}
		normBase.OS = strings.ToLower(sys.String())
	}

	refreshConfig, err := charmhub.InstallOneFromChannel(c.charmOrBundle, normChannel.String(), charmhub.RefreshBase{
		Architecture: normBase.Architecture,
		Name:         normBase.OS,
		Channel:      normBase.Series,
	})
	if err != nil {
		return errors.Trace(err)
	}

	results, err := client.Refresh(ctx, refreshConfig)
	if err != nil {
		return errors.Trace(err)
	}

	if len(results) == 0 {
		return errors.NotFoundf(c.charmOrBundle)
	}
	// Ensure we didn't get any errors whilst querying the charmhub API
	for _, res := range results {
		if res.Error != nil {
			if res.Error.Code == transport.ErrorCodeRevisionNotFound {
				return c.suggested(normBase.Series, normChannel.String(), res.Error.Extra.Releases, cmdContext)
			}
			return errors.Errorf("unable to locate %s: %s", c.charmOrBundle, res.Error.Message)
		}
	}

	// In theory we can get multiple responses from the refresh API, but in
	// reality if we only request one action, we only get one result. If that
	// happens not to be the case, just select the first one.
	result := results[0]
	entity := result.Entity
	entityType := entity.Type
	entitySHA := entity.Download.HashSHA256

	path := c.archivePath
	if c.archivePath == "" {
		// Use the sha256 to create a unique path for every download. The
		// consequence of this is that same sha binary blobs will overwrite
		// each other. That should be ok, as the sha will match.
		var short string
		if len(entitySHA) >= 7 {
			short = fmt.Sprintf("_%s", entitySHA[0:7])
		}
		path = fmt.Sprintf("%s%s.%s", entity.Name, short, entityType)
	}

	base := normBase
	base.Series, err = coreseries.SeriesVersion(normBase.Series)
	if err != nil {
		base.Series = normBase.Series
	}

	cmdContext.Infof("Fetching %s %q using %q channel and base %q", entityType, entity.Name, normChannel, base)

	resourceURL, err := url.Parse(entity.Download.URL)
	if err != nil {
		return errors.Trace(err)
	}

	pb := progress.MakeProgressBar(cmdContext.Stdout)
	ctx = context.WithValue(ctx, charmhub.DownloadNameKey, entity.Name)
	if err := client.Download(ctx, resourceURL, path, charmhub.WithProgressBar(pb)); err != nil {
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

func (c *downloadCommand) suggested(ser string, channel string, releases []transport.Release, cmdContext *cmd.Context) error {
	series := set.NewStrings()
	for _, rel := range releases {
		if rel.Channel == channel {
			s, err := coreseries.VersionSeries(rel.Base.Channel)
			if err == nil {
				series.Add(s)
			} else {
				// Shouldn't happen, log and continue if verbose is set.
				cmdContext.Verbosef("%s of %s", err, rel.Base.Name)
			}
		}
	}
	return errors.Errorf("%s does not support series %s in channel %s.  Supported series are %s.",
		c.charmOrBundle, ser, channel, strings.Join(series.SortedValues(), ", "))
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
