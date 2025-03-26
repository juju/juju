// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/v4/symlink"

	coretools "github.com/juju/juju/internal/tools"
	"github.com/juju/juju/internal/version"
)

const (
	dirPerm   = 0755
	filePerm  = 0644
	toolsFile = "downloaded-tools.txt"
)

// SharedToolsDir returns the directory that is used to
// store binaries for the given version of the juju tools
// within the dataDir directory.
func SharedToolsDir(dataDir string, vers version.Binary) string {
	return path.Join(dataDir, "tools", vers.String())
}

// ToolsDir returns the directory that is used/ to store binaries for
// the tools used by the given agent within the given dataDir directory.
// Conventionally it is a symbolic link to the actual tools directory.
func ToolsDir(dataDir, agentName string) string {
	//TODO(perrito666) ToolsDir and any other *Dir needs to take the
	// agent series to use the right path, in this case, if filepath
	// is used it ends up creating a bogus toolsdir when the client
	// is in windows.
	return path.Join(dataDir, "tools", agentName)
}

// UnpackTools reads a set of juju tools in gzipped tar-archive
// format and unpacks them into the appropriate tools directory
// within dataDir. If a valid tools directory already exists,
// UnpackTools returns without error.
func UnpackTools(dataDir string, tools *coretools.Tools, r io.Reader) (err error) {
	// Unpack the gzip file and compute the checksum.
	sha256hash := sha256.New()
	zr, err := gzip.NewReader(io.TeeReader(r, sha256hash))
	if err != nil {
		return err
	}
	defer zr.Close()
	f, err := os.CreateTemp(os.TempDir(), "tools-tar")
	if err != nil {
		return err
	}
	defer func() {
		_ = f.Close()
		_ = os.Remove(f.Name())
	}()

	_, err = io.Copy(f, zr)
	if err != nil {
		return err
	}

	gzipSHA256 := fmt.Sprintf("%x", sha256hash.Sum(nil))
	if tools.SHA256 != gzipSHA256 {
		return fmt.Errorf("tarball sha256 mismatch, expected %s, got %s", tools.SHA256, gzipSHA256)
	}

	// Make a temporary directory in the tools directory,
	// first ensuring that the tools directory exists.
	toolsDir := path.Join(dataDir, "tools")
	err = os.MkdirAll(toolsDir, dirPerm)
	if err != nil {
		return err
	}
	dir, err := os.MkdirTemp(toolsDir, "unpacking-")
	if err != nil {
		return err
	}
	defer removeAll(dir)

	// Checksum matches, now reset the file and untar it.
	_, err = f.Seek(0, 0)
	if err != nil {
		return err
	}
	tr := tar.NewReader(f)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if strings.ContainsAny(hdr.Name, "/\\") {
			return fmt.Errorf("bad name %q in agent binary archive", hdr.Name)
		}
		if hdr.Typeflag != tar.TypeReg {
			return fmt.Errorf("bad file type %c in file %q in agent binary archive", hdr.Typeflag, hdr.Name)
		}
		name := path.Join(dir, hdr.Name)
		if err := writeFile(name, os.FileMode(hdr.Mode&0777), tr); err != nil {
			return errors.Annotatef(err, "tar extract %q failed", name)
		}
	}
	if err = WriteToolsMetadataData(dir, tools); err != nil {
		return err
	}

	// The tempdir is created with 0700, so we need to make it more
	// accessible for juju-exec.
	err = os.Chmod(dir, dirPerm)
	if err != nil {
		return err
	}

	err = os.Rename(dir, SharedToolsDir(dataDir, tools.Version))
	// If we've failed to rename the directory, it may be because
	// the directory already exists - if ReadTools succeeds, we
	// assume all's ok.
	if err != nil {
		if _, err := ReadTools(dataDir, tools.Version); err == nil {
			return nil
		}
	}
	return err
}

func removeAll(dir string) {
	err := os.RemoveAll(dir)
	if err == nil || os.IsNotExist(err) {
		return
	}
	logger.Errorf(context.TODO(), "cannot remove %q: %v", dir, err)
}

func writeFile(name string, mode os.FileMode, r io.Reader) error {
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}

// ReadTools checks that the tools information for the given version exists
// in the dataDir directory, and returns a Tools instance.
// The tools information is json encoded in a text file, "downloaded-tools.txt".
func ReadTools(dataDir string, vers version.Binary) (*coretools.Tools, error) {
	dir := SharedToolsDir(dataDir, vers)
	toolsData, err := os.ReadFile(path.Join(dir, toolsFile))
	if err != nil {
		return nil, fmt.Errorf("cannot read agent metadata in directory %v: %w", dir, err)
	}
	var tools coretools.Tools
	if err := json.Unmarshal(toolsData, &tools); err != nil {
		return nil, fmt.Errorf("invalid agent metadata in directory %q: %w", dir, err)
	}
	return &tools, nil
}

// ChangeAgentTools atomically replaces the agent-specific symlink
// under dataDir so it points to the previously unpacked
// version vers. It returns the new tools read.
func ChangeAgentTools(dataDir string, agentName string, vers version.Binary) (*coretools.Tools, error) {
	tools, err := ReadTools(dataDir, vers)
	if err != nil {
		return nil, err
	}
	// build absolute path to toolsDir. Windows implementation of symlink
	// will check for the existence of the source file and error if it does
	// not exists. This is a limitation of junction points (symlinks) on NTFS
	toolPath := SharedToolsDir(dataDir, tools.Version)
	toolsDir := ToolsDir(dataDir, agentName)

	err = symlink.Replace(toolsDir, toolPath)
	if err != nil {
		return nil, fmt.Errorf("cannot replace tools directory: %s", err)
	}
	return tools, nil
}

// WriteToolsMetadataData writes the tools metadata file to the given directory.
func WriteToolsMetadataData(dir string, tools *coretools.Tools) error {
	toolsMetadataData, err := json.Marshal(tools)
	if err != nil {
		return err
	}
	return os.WriteFile(path.Join(dir, toolsFile), toolsMetadataData, filePerm)
}
