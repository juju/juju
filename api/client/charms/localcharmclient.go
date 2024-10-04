// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/version/v2"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/core/lxdprofile"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/internal/charm"
)

// LocalCharmClient allows access to the API endpoints
// required to add a local charm
type LocalCharmClient struct {
	base.ClientFacade
	facade      base.FacadeCaller
	charmPutter CharmPutter
}

// NewLocalCharmClient creates a client which can be used to
// upload local charms to the server
func NewLocalCharmClient(st base.APICallCloser) (*LocalCharmClient, error) {
	s3Putter, err := newS3Putter(st)
	if err != nil {
		return nil, errors.Trace(err)
	}
	frontend, backend := base.NewClientFacade(st, "Charms")
	return &LocalCharmClient{ClientFacade: frontend, facade: backend, charmPutter: s3Putter}, nil
}

// AddLocalCharm prepares the given charm with a local: schema in its
// URL, and uploads it via the API server, returning the assigned
// charm URL.
func (c *LocalCharmClient) AddLocalCharm(curl *charm.URL, ch charm.Charm, force bool, agentVersion version.Number) (*charm.URL, error) {
	if curl.Schema != "local" {
		return nil, errors.Errorf("expected charm URL with local: schema, got %q", curl.String())
	}

	if err := c.validateCharmVersion(ch, agentVersion); err != nil {
		return nil, errors.Trace(err)
	}
	if err := lxdprofile.ValidateLXDProfile(lxdCharmProfiler{Charm: ch}); err != nil {
		if !force {
			return nil, errors.Trace(err)
		}
	}

	// Package the charm for uploading.
	var archive *os.File
	switch ch := ch.(type) {
	case *charm.CharmDir:
		var err error
		if archive, err = os.CreateTemp("", "charm"); err != nil {
			return nil, errors.Annotate(err, "cannot create temp file")
		}
		defer func() {
			_ = archive.Close()
			_ = os.Remove(archive.Name())
		}()

		if err := ch.ArchiveTo(archive); err != nil {
			return nil, errors.Annotate(err, "cannot repackage charm")
		}
		if _, err := archive.Seek(0, os.SEEK_SET); err != nil {
			return nil, errors.Annotate(err, "cannot rewind packaged charm")
		}
	case *charm.CharmArchive:
		var err error
		if archive, err = os.Open(ch.Path); err != nil {
			return nil, errors.Annotate(err, "cannot read charm archive")
		}
		defer archive.Close()
	default:
		return nil, errors.Errorf("unknown charm type %T", ch)
	}

	anyHooksOrDispatch, err := hasHooksOrDispatch(archive.Name())
	if err != nil {
		return nil, errors.Trace(err)
	}
	if !anyHooksOrDispatch {
		return nil, errors.Errorf("invalid charm %q: has no hooks nor dispatch file", curl.Name)
	}

	hash, err := hashArchive(archive)
	if err != nil {
		return nil, errors.Trace(err)
	}
	charmRef := fmt.Sprintf("%s-%s", curl.Name, hash)

	modelTag, _ := c.facade.RawAPICaller().ModelTag()

	newCurlStr, err := c.charmPutter.PutCharm(context.Background(), modelTag.Id(), charmRef, curl.String(), archive)
	if err != nil {
		return nil, errors.Trace(err)
	}
	newCurl, err := charm.ParseURL(newCurlStr)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return newCurl, nil
}

// lxdCharmProfiler massages a charm.Charm into a LXDProfiler inside of the
// core package.
type lxdCharmProfiler struct {
	Charm charm.Charm
}

// LXDProfile implements core.lxdprofile.LXDProfiler
func (p lxdCharmProfiler) LXDProfile() lxdprofile.LXDProfile {
	if p.Charm == nil {
		return nil
	}
	if profiler, ok := p.Charm.(charm.LXDProfiler); ok {
		profile := profiler.LXDProfile()
		if profile == nil {
			return nil
		}
		return profile
	}
	return nil
}

var hasHooksOrDispatch = hasHooksFolderOrDispatchFile

func hasHooksFolderOrDispatchFile(name string) (bool, error) {
	zipr, err := zip.OpenReader(name)
	if err != nil {
		return false, err
	}
	defer zipr.Close()
	// zip file spec 4.4.17.1 says that separators are always "/" even on Windows.
	dispatchPath := "dispatch"
	for _, f := range zipr.File {
		if isHook(f) {
			return true, nil
		}
		if strings.Contains(f.Name, dispatchPath) {
			return true, nil
		}
	}
	return false, nil
}

// isHook verifies whether the given file inside a zip archive is a valid hook
// file.
func isHook(f *zip.File) bool {
	// Hook files must be in the top level of the 'hooks' directory
	if filepath.Dir(f.Name) != "hooks" {
		return false
	}
	// Valid file modes are regular files or symlinks. All others (directories,
	// sockets, devices, etc.) are considered invalid file modes.
	switch mode := f.Mode(); {
	case mode.IsRegular():
		return true
	case mode&fs.ModeSymlink != 0:
		return true
	default:
		return false
	}
}

func (c *LocalCharmClient) validateCharmVersion(ch charm.Charm, agentVersion version.Number) error {
	minver := ch.Meta().MinJujuVersion
	if minver != version.Zero {
		return jujuversion.CheckJujuMinVersion(minver, agentVersion)
	}
	return nil
}

func hashArchive(archive *os.File) (string, error) {
	hash := sha256.New()
	_, err := io.Copy(hash, archive)
	if err != nil {
		return "", errors.Trace(err)
	}
	_, err = archive.Seek(0, os.SEEK_SET)
	if err != nil {
		return "", errors.Trace(err)
	}
	return hex.EncodeToString(hash.Sum(nil))[0:7], nil
}
