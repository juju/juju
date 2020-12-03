// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gui

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/version"

	"github.com/juju/juju/api/controller"
	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/gui"
)

// NewUpgradeDashboardCommand creates and returns a new upgrade-dashboard command.
func NewUpgradeDashboardCommand() cmd.Command {
	return modelcmd.WrapController(&upgradeDashboardCommand{})
}

// upgradeDashboardCommand upgrades to a new Juju Dashboard version in the controller.
type upgradeDashboardCommand struct {
	modelcmd.ControllerCommandBase

	dashboardStream string
	versOrPath      string
	list            bool
}

const upgradeGUIDoc = `
Upgrade to the latest Juju Dashboard released version:

	juju upgrade-dashboard

Upgrade to the latest Juju Dashboard development version:

	juju upgrade-dashboard --dashboard-stream=devel

Upgrade to a specific Juju Dashboard released version:

	juju upgrade-dashboard 2.2.0

Upgrade to a Juju Dashboard version present in a local tar.bz2 Dashboard release file:

	juju upgrade-dashboard /path/to/jujudashboard-2.2.0.tar.bz2

List available Juju Dashboard releases without upgrading:

	juju upgrade-dashboard --list
`

// Info implements the cmd.Command interface.
func (c *upgradeDashboardCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "upgrade-dashboard",
		Purpose: "Upgrade to a new Juju Dashboard version.",
		Doc:     upgradeGUIDoc,
	})
}

// SetFlags implements the cmd.Command interface.
func (c *upgradeDashboardCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ControllerCommandBase.SetFlags(f)
	f.BoolVar(&c.list, "list", false, "List available Juju Dashboard release versions without upgrading")
	f.StringVar(&c.dashboardStream, "dashboard-stream", gui.ReleasedStream, "Specify the stream used to fetch the dashboard")
}

// Init implements the cmd.Command interface.
func (c *upgradeDashboardCommand) Init(args []string) error {
	if len(args) == 1 {
		if c.list {
			return errors.New("cannot provide arguments if --list is provided")
		}
		c.versOrPath = args[0]
		return nil
	}
	return cmd.CheckEmpty(args)
}

// Run implements the cmd.Command interface.
func (c *upgradeDashboardCommand) Run(ctx *cmd.Context) error {
	// Open the Juju API client.
	client, err := c.NewControllerAPIClient()
	if err != nil {
		return errors.Annotate(err, "cannot establish API connection")
	}
	defer client.Close()

	// Get the controller version so we can select a compatible dashboard.
	versInfo, err := client.ControllerVersion()
	if err != nil {
		return errors.Trace(err)
	}
	ctrlVersion, err := version.Parse(versInfo.Version)
	if err != nil {
		return errors.Trace(err)
	}

	if c.list {
		// List available Juju Dashboard archive versions.
		allMeta, err := remoteArchiveMetadata(c.dashboardStream, ctrlVersion.Major, ctrlVersion.Minor)
		if err != nil {
			return errors.Annotate(err, "cannot list Juju Dashboard release versions")
		}
		for _, metadata := range allMeta {
			ctx.Infof(metadata.Version.String())
		}
		return nil
	}
	// Retrieve the dashboard archive and its related info.
	archive, err := c.openArchive(c.dashboardStream, c.versOrPath, ctrlVersion.Major, ctrlVersion.Minor)
	if err != nil {
		return errors.Trace(err)
	}
	defer archive.r.Close()

	// Check currently uploaded dashboard version.
	existingHash, isCurrent, err := existingVersionInfo(client, archive.vers)
	if err != nil {
		return errors.Trace(err)
	}

	// Upload the release file if required.
	if archive.hash != existingHash {
		if archive.local {
			ctx.Infof("using local Juju Dashboard archive")
		} else {
			ctx.Infof("fetching Juju Dashboard archive")
		}
		f, err := storeArchive(archive.r)
		if err != nil {
			return errors.Trace(err)
		}
		defer f.Close()
		ctx.Infof("uploading Juju Dashboard %s", archive.vers)
		isCurrent, err = clientUploadDashboardArchive(client, f, archive.hash, archive.size, archive.vers)
		if err != nil {
			return errors.Annotate(err, "cannot upload Juju Dashboard")
		}
		ctx.Infof("upload completed")
	}
	// Switch to the new version if not already at the desired one.
	if isCurrent {
		ctx.Infof("Juju Dashboard at version %s", archive.vers)
		return nil
	}
	if err = clientSelectDashboardVersion(client, archive.vers); err != nil {
		return errors.Annotate(err, "cannot switch to new Juju Dashboard version")
	}
	ctx.Infof("Juju Dashboard switched to version %s", archive.vers)
	return nil
}

// openedArchive holds the results of openArchive calls.
type openedArchive struct {
	r     io.ReadCloser
	hash  string
	size  int64
	vers  version.Number
	local bool
}

// openArchive opens a Juju Dashboard archive from the given version or file path.
// The readSeekCloser returned in openedArchive.r must be closed by callers.
func (c *upgradeDashboardCommand) openArchive(stream, versOrPath string, major, minor int) (openedArchive, error) {
	if versOrPath == "" {
		// Return the most recent Juju Dashboard from simplestreams.
		allMeta, err := remoteArchiveMetadata(stream, major, minor)
		if err != nil {
			return openedArchive{}, errors.Annotate(err, "cannot upgrade to most recent release")
		}
		// The most recent Juju Dashboard release is the first on the list.
		metadata := allMeta[0]
		r, _, err := metadata.Source.Fetch(metadata.Path)
		if err != nil {
			return openedArchive{}, errors.Annotatef(err, "cannot open Juju Dashboard archive at %q", metadata.FullPath)
		}
		return openedArchive{
			r:    r,
			hash: metadata.SHA256,
			size: metadata.Size,
			vers: metadata.Version,
		}, nil
	}
	f, err := c.Filesystem().Open(versOrPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return openedArchive{}, errors.Annotate(err, "cannot open Dashboard archive")
		}
		vers, err := version.Parse(versOrPath)
		if err != nil {
			return openedArchive{}, errors.Errorf("invalid Dashboard release version or local path %q", versOrPath)
		}
		// Return a specific release version from simplestreams.
		allMeta, err := remoteArchiveMetadata(stream, major, minor)
		if err != nil {
			return openedArchive{}, errors.Annotatef(err, "cannot upgrade to release %s", vers)
		}
		metadata, err := findMetadataVersion(allMeta, vers)
		if err != nil {
			return openedArchive{}, errors.Trace(err)
		}
		r, _, err := metadata.Source.Fetch(metadata.Path)
		if err != nil {
			return openedArchive{}, errors.Annotatef(err, "cannot open Juju Dashboard archive at %q", metadata.FullPath)
		}
		return openedArchive{
			r:    r,
			hash: metadata.SHA256,
			size: metadata.Size,
			vers: metadata.Version,
		}, nil
	}
	// This is a local Juju Dashboard release.
	defer func() {
		if err != nil {
			f.Close()
		}
	}()
	vers, err := gui.DashboardArchiveVersion(f)
	if err != nil {
		return openedArchive{}, errors.Annotatef(err, "cannot upgrade Juju Dashboard using %q", versOrPath)
	}
	if _, err := f.Seek(0, 0); err != nil {
		return openedArchive{}, errors.Annotate(err, "cannot seek archive")
	}
	hash, size, err := hashAndSize(f)
	if err != nil {
		return openedArchive{}, errors.Annotatef(err, "cannot upgrade Juju Dashboard using %q", versOrPath)
	}
	if _, err := f.Seek(0, 0); err != nil {
		return openedArchive{}, errors.Annotate(err, "cannot seek archive")
	}
	return openedArchive{
		r:     f,
		hash:  hash,
		size:  size,
		vers:  vers,
		local: true,
	}, nil
}

// remoteArchiveMetadata returns Juju Dashboard archive metadata from simplestreams.
// The dashboard metadata will be compatible with the juju major.minor version.
func remoteArchiveMetadata(stream string, major, minor int) ([]*gui.Metadata, error) {
	source := gui.NewDataSource(common.DashboardDataSourceBaseURL())
	allMeta, err := dashboardFetchMetadata(stream, major, minor, source)
	if err != nil {
		return nil, errors.Annotate(err, "cannot retrieve Juju Dashboard archive info")
	}
	if len(allMeta) == 0 {
		return nil, errors.New("no available Juju Dashboard archives found")
	}
	return allMeta, nil
}

// findMetadataVersion returns the metadata in allMeta with the given version.
func findMetadataVersion(allMeta []*gui.Metadata, vers version.Number) (*gui.Metadata, error) {
	for _, metadata := range allMeta {
		if metadata.Version == vers {
			return metadata, nil
		}
	}
	return nil, errors.NotFoundf("Juju Dashboard release version %s", vers)
}

// hashAndSize returns the SHA256 hash and size of the data included in r.
func hashAndSize(r io.Reader) (hash string, size int64, err error) {
	h := sha256.New()
	size, err = io.Copy(h, r)
	if err != nil {
		return "", 0, errors.Annotate(err, "cannot calculate archive hash")
	}
	return fmt.Sprintf("%x", h.Sum(nil)), size, nil
}

// existingVersionInfo returns the hash of the existing Dashboard archive at the
// given version and reports whether that's the current version served by the
// controller. If the given version is not present in the server, an empty
// hash is returned.
func existingVersionInfo(client *controller.Client, vers version.Number) (hash string, current bool, err error) {
	versions, err := clientDashboardArchives(client)
	if err != nil {
		return "", false, errors.Annotate(err, "cannot retrieve Dashboard versions from the controller")
	}
	for _, v := range versions {
		if v.Version == vers {
			return v.SHA256, v.Current, nil
		}
	}
	return "", false, nil
}

// storeArchive saves the Juju Dashboard archive in the given reader in a temporary
// file. The resulting returned readSeekCloser is deleted when closed.
func storeArchive(r io.Reader) (readSeekCloser, error) {
	f, err := ioutil.TempFile("", "dashboard-archive")
	if err != nil {
		return nil, errors.Annotate(err, "cannot create a temporary file to save the Juju Dashboard archive")
	}
	if _, err = io.Copy(f, r); err != nil {
		return nil, errors.Annotate(err, "cannot retrieve Juju Dashboard archive")
	}
	if _, err = f.Seek(0, 0); err != nil {
		return nil, errors.Annotate(err, "cannot seek temporary archive file")
	}
	return deleteOnCloseFile{f}, nil
}

// readSeekCloser combines the io read, seek and close methods.
type readSeekCloser interface {
	io.ReadCloser
	io.Seeker
}

// deleteOnCloseFile is a file that gets deleted when closed.
type deleteOnCloseFile struct {
	*os.File
}

// Close closes the file.
func (f deleteOnCloseFile) Close() error {
	f.File.Close()
	os.Remove(f.Name())
	return nil
}

// clientDashboardArchives is defined for testing purposes.
var clientDashboardArchives = func(client *controller.Client) ([]params.DashboardArchiveVersion, error) {
	return client.DashboardArchives()
}

// clientSelectDashboardVersion is defined for testing purposes.
var clientSelectDashboardVersion = func(client *controller.Client, vers version.Number) error {
	return client.SelectDashboardVersion(vers)
}

// clientUploadDashboardArchive is defined for testing purposes.
var clientUploadDashboardArchive = func(client *controller.Client, r io.ReadSeeker, hash string, size int64, vers version.Number) (bool, error) {
	return client.UploadDashboardArchive(r, hash, size, vers)
}

// dashboardFetchMetadata is defined for testing purposes.
var dashboardFetchMetadata = gui.FetchMetadata
