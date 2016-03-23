// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gui

import (
	"archive/tar"
	"compress/bzip2"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/version"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
)

// NewUpgradeGUICommand creates and returns a new upgrade-gui command.
func NewUpgradeGUICommand() cmd.Command {
	return modelcmd.Wrap(&upgradeGUICommand{})
}

// upgradeGUICommand upgrades to a new Juju GUI version in the controller.
type upgradeGUICommand struct {
	modelcmd.ModelCommandBase

	versOrPath string
}

const upgradeGUIDoc = `
Upgrade to the latest Juju GUI released version:

	juju upgrade-gui

Upgrade to a specific Juju GUI released version:

	juju upgrade-gui 2.2.0

Upgrade to a Juju GUI version present in a local tar.bz2 GUI release file.

	juju upgrade-gui /path/to/jujugui-2.2.0.tar.bz2
`

func (c *upgradeGUICommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "upgrade-gui",
		Purpose: "upgrade to a new Juju GUI version",
		Doc:     upgradeGUIDoc,
	}
}

func (c *upgradeGUICommand) Init(args []string) error {
	if len(args) == 1 {
		c.versOrPath = args[0]
		return nil
	}
	return cmd.CheckEmpty(args)
}

func (c *upgradeGUICommand) Run(ctx *cmd.Context) error {
	// Retrieve the GUI archive and its related info.
	r, hash, size, vers, err := openArchive(c.versOrPath)
	if err != nil {
		return errors.Trace(err)
	}
	defer r.Close()

	// Open the Juju API client.
	client, err := c.NewAPIClient()
	if err != nil {
		return errors.Annotate(err, "cannot establish API connection")
	}
	defer client.Close()

	// Check currently uploaded GUI version.
	existingHash, isCurrent, err := existingVersionInfo(client, vers)
	if err != nil {
		return errors.Trace(err)
	}

	// Upload the release file if required.
	if hash != existingHash {
		ctx.Infof("uploading Juju GUI %s", vers)
		isCurrent, err = clientUploadGUIArchive(client, r, hash, size, vers)
		if err != nil {
			return errors.Annotate(err, "cannot upload Juju GUI")
		}
		ctx.Infof("upload completed")
	}
	// Switch to the new version if not already at the desired one.
	if isCurrent {
		ctx.Infof("Juju GUI at version %s", vers)
		return nil
	}
	if err = clientSelectGUIVersion(client, vers); err != nil {
		return errors.Annotate(err, "cannot switch to new Juju GUI version")
	}
	ctx.Infof("Juju GUI switched to version %s", vers)
	return nil
}

// openArchive opens a Juju GUI archive from the given version or file path.
// The returned readSeekCloser must be closed by callers.
func openArchive(versOrPath string) (r readSeekCloser, hash string, size int64, vers version.Number, err error) {
	if versOrPath == "" {
		// TODO frankban: implement retrieving the latest GUI archive from
		// simplestreams.
		return nil, "", 0, vers, errors.New("upgrading to latest released version not implemented")
	}
	f, err := os.Open(versOrPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, "", 0, vers, errors.Annotate(err, "cannot open GUI archive")
		}
		vers, err = version.Parse(versOrPath)
		if err != nil {
			return nil, "", 0, vers, errors.Errorf("invalid GUI release version or local path %q", versOrPath)
		}
		// TODO frankban: implement upgrading to a released version.
		return nil, "", 0, vers, errors.Errorf("upgrading to a released version (%s) not implemented", vers)
	}
	defer func() {
		if err != nil {
			f.Close()
		}
	}()
	vers, err = archiveVersion(f)
	if err != nil {
		return nil, "", 0, vers, errors.Annotatef(err, "cannot upgrade Juju GUI using %q", versOrPath)
	}
	if _, err := f.Seek(0, 0); err != nil {
		return nil, "", 0, version.Number{}, errors.Annotate(err, "cannot seek archive")
	}
	hash, size, err = hashAndSize(f)
	if err != nil {
		return nil, "", 0, version.Number{}, errors.Annotatef(err, "cannot upgrade Juju GUI using %q", versOrPath)
	}
	if _, err := f.Seek(0, 0); err != nil {
		return nil, "", 0, version.Number{}, errors.Annotate(err, "cannot seek archive")
	}
	return f, hash, size, vers, nil
}

// readSeekCloser combines the io read, seek and close methods.
type readSeekCloser interface {
	io.ReadCloser
	io.Seeker
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
