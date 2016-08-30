// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gui

import (
	"archive/tar"
	"compress/bzip2"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/version"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs/gui"
)

// NewUpgradeGUICommand creates and returns a new upgrade-gui command.
func NewUpgradeGUICommand() cmd.Command {
	return modelcmd.Wrap(&upgradeGUICommand{})
}

// upgradeGUICommand upgrades to a new Juju GUI version in the controller.
type upgradeGUICommand struct {
	modelcmd.ModelCommandBase

	versOrPath string
	list       bool
}

const upgradeGUIDoc = `
Upgrade to the latest Juju GUI released version:

	juju upgrade-gui

Upgrade to a specific Juju GUI released version:

	juju upgrade-gui 2.2.0

Upgrade to a Juju GUI version present in a local tar.bz2 GUI release file:

	juju upgrade-gui /path/to/jujugui-2.2.0.tar.bz2

List available Juju GUI releases without upgrading:

	juju upgrade-gui --list
`

// Info implements the cmd.Command interface.
func (c *upgradeGUICommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "upgrade-gui",
		Purpose: "Upgrade to a new Juju GUI version.",
		Doc:     upgradeGUIDoc,
	}
}

// SetFlags implements the cmd.Command interface.
func (c *upgradeGUICommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.list, "list", false, "List available Juju GUI release versions without upgrading")
}

// Init implements the cmd.Command interface.
func (c *upgradeGUICommand) Init(args []string) error {
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
func (c *upgradeGUICommand) Run(ctx *cmd.Context) error {
	if c.list {
		// List available Juju GUI archive versions.
		allMeta, err := remoteArchiveMetadata()
		if err != nil {
			return errors.Annotate(err, "cannot list Juju GUI release versions")
		}
		for _, metadata := range allMeta {
			ctx.Infof(metadata.Version.String())
		}
		return nil
	}
	// Retrieve the GUI archive and its related info.
	archive, err := openArchive(c.versOrPath)
	if err != nil {
		return errors.Trace(err)
	}
	defer archive.r.Close()

	// Open the Juju API client.
	client, err := c.NewAPIClient()
	if err != nil {
		return errors.Annotate(err, "cannot establish API connection")
	}
	defer client.Close()

	// Check currently uploaded GUI version.
	existingHash, isCurrent, err := existingVersionInfo(client, archive.vers)
	if err != nil {
		return errors.Trace(err)
	}

	// Upload the release file if required.
	if archive.hash != existingHash {
		if archive.local {
			ctx.Infof("using local Juju GUI archive")
		} else {
			ctx.Infof("fetching Juju GUI archive")
		}
		f, err := storeArchive(archive.r)
		if err != nil {
			return errors.Trace(err)
		}
		defer f.Close()
		ctx.Infof("uploading Juju GUI %s", archive.vers)
		isCurrent, err = clientUploadGUIArchive(client, f, archive.hash, archive.size, archive.vers)
		if err != nil {
			return errors.Annotate(err, "cannot upload Juju GUI")
		}
		ctx.Infof("upload completed")
	}
	// Switch to the new version if not already at the desired one.
	if isCurrent {
		ctx.Infof("Juju GUI at version %s", archive.vers)
		return nil
	}
	if err = clientSelectGUIVersion(client, archive.vers); err != nil {
		return errors.Annotate(err, "cannot switch to new Juju GUI version")
	}
	ctx.Infof("Juju GUI switched to version %s", archive.vers)
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

// openArchive opens a Juju GUI archive from the given version or file path.
// The readSeekCloser returned in openedArchive.r must be closed by callers.
func openArchive(versOrPath string) (openedArchive, error) {
	if versOrPath == "" {
		// Return the most recent Juju GUI from simplestreams.
		allMeta, err := remoteArchiveMetadata()
		if err != nil {
			return openedArchive{}, errors.Annotate(err, "cannot upgrade to most recent release")
		}
		// The most recent Juju GUI release is the first on the list.
		metadata := allMeta[0]
		r, _, err := metadata.Source.Fetch(metadata.Path)
		if err != nil {
			return openedArchive{}, errors.Annotatef(err, "cannot open Juju GUI archive at %q", metadata.FullPath)
		}
		return openedArchive{
			r:    r,
			hash: metadata.SHA256,
			size: metadata.Size,
			vers: metadata.Version,
		}, nil
	}
	f, err := os.Open(versOrPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return openedArchive{}, errors.Annotate(err, "cannot open GUI archive")
		}
		vers, err := version.Parse(versOrPath)
		if err != nil {
			return openedArchive{}, errors.Errorf("invalid GUI release version or local path %q", versOrPath)
		}
		// Return a specific release version from simplestreams.
		allMeta, err := remoteArchiveMetadata()
		if err != nil {
			return openedArchive{}, errors.Annotatef(err, "cannot upgrade to release %s", vers)
		}
		metadata, err := findMetadataVersion(allMeta, vers)
		if err != nil {
			return openedArchive{}, errors.Trace(err)
		}
		r, _, err := metadata.Source.Fetch(metadata.Path)
		if err != nil {
			return openedArchive{}, errors.Annotatef(err, "cannot open Juju GUI archive at %q", metadata.FullPath)
		}
		return openedArchive{
			r:    r,
			hash: metadata.SHA256,
			size: metadata.Size,
			vers: metadata.Version,
		}, nil
	}
	// This is a local Juju GUI release.
	defer func() {
		if err != nil {
			f.Close()
		}
	}()
	vers, err := archiveVersion(f)
	if err != nil {
		return openedArchive{}, errors.Annotatef(err, "cannot upgrade Juju GUI using %q", versOrPath)
	}
	if _, err := f.Seek(0, 0); err != nil {
		return openedArchive{}, errors.Annotate(err, "cannot seek archive")
	}
	hash, size, err := hashAndSize(f)
	if err != nil {
		return openedArchive{}, errors.Annotatef(err, "cannot upgrade Juju GUI using %q", versOrPath)
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

// remoteArchiveMetadata returns Juju GUI archive metadata from simplestreams.
func remoteArchiveMetadata() ([]*gui.Metadata, error) {
	source := gui.NewDataSource(common.GUIDataSourceBaseURL())
	allMeta, err := guiFetchMetadata(gui.ReleasedStream, source)
	if err != nil {
		return nil, errors.Annotate(err, "cannot retrieve Juju GUI archive info")
	}
	if len(allMeta) == 0 {
		return nil, errors.New("no available Juju GUI archives found")
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
	return nil, errors.NotFoundf("Juju GUI release version %s", vers)
}

// archiveVersion retrieves the GUI version from the juju-gui-* directory
// included in the given tar.bz2 archive reader.
func archiveVersion(r io.Reader) (version.Number, error) {
	var vers version.Number
	prefix := "jujugui-"
	tr := tar.NewReader(bzip2.NewReader(r))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return vers, errors.New("cannot read Juju GUI archive")
		}
		info := hdr.FileInfo()
		if !info.IsDir() || !strings.HasPrefix(hdr.Name, prefix) {
			continue
		}
		n := filepath.Dir(hdr.Name)[len(prefix):]
		vers, err = version.Parse(n)
		if err != nil {
			return vers, errors.Errorf("invalid version %q in archive", n)
		}
		return vers, nil
	}
	return vers, errors.New("cannot find Juju GUI version in archive")
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

// existingVersionInfo returns the hash of the existing GUI archive at the
// given version and reports whether that's the current version served by the
// controller. If the given version is not present in the server, an empty
// hash is returned.
func existingVersionInfo(client *api.Client, vers version.Number) (hash string, current bool, err error) {
	versions, err := clientGUIArchives(client)
	if err != nil {
		return "", false, errors.Annotate(err, "cannot retrieve GUI versions from the controller")
	}
	for _, v := range versions {
		if v.Version == vers {
			return v.SHA256, v.Current, nil
		}
	}
	return "", false, nil
}

// storeArchive saves the Juju GUI archive in the given reader in a temporary
// file. The resulting returned readSeekCloser is deleted when closed.
func storeArchive(r io.Reader) (readSeekCloser, error) {
	f, err := ioutil.TempFile("", "gui-archive")
	if err != nil {
		return nil, errors.Annotate(err, "cannot create a temporary file to save the Juju GUI archive")
	}
	if _, err = io.Copy(f, r); err != nil {
		return nil, errors.Annotate(err, "cannot retrieve Juju GUI archive")
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

// clientGUIArchives is defined for testing purposes.
var clientGUIArchives = func(client *api.Client) ([]params.GUIArchiveVersion, error) {
	return client.GUIArchives()
}

// clientSelectGUIVersion is defined for testing purposes.
var clientSelectGUIVersion = func(client *api.Client, vers version.Number) error {
	return client.SelectGUIVersion(vers)
}

// clientUploadGUIArchive is defined for testing purposes.
var clientUploadGUIArchive = func(client *api.Client, r io.ReadSeeker, hash string, size int64, vers version.Number) (bool, error) {
	return client.UploadGUIArchive(r, hash, size, vers)
}

// guiFetchMetadata is defined for testing purposes.
var guiFetchMetadata = gui.FetchMetadata
