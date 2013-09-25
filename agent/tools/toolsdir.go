// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"

	coretools "launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
)

const toolsFile = "downloaded-tools.txt"

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
	f, err := ioutil.TempFile(os.TempDir(), "tools-tar")
	if err != nil {
		return err
	}
	_, err = io.Copy(f, zr)
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	// TODO(wallyworld) - 2013-09-24 bug=1229512
	// When we can ensure all tools records have valid checksums recorded,
	// we can remove this test short circuit.
	gzipSHA256 := fmt.Sprintf("%x", sha256hash.Sum(nil))
	if tools.SHA256 != "" && tools.SHA256 != gzipSHA256 {
		return fmt.Errorf("tarball sha256 mismatch, expected %s, got %s", tools.SHA256, gzipSHA256)
	}

	// Make a temporary directory in the tools directory,
	// first ensuring that the tools directory exists.
	toolsDir := path.Join(dataDir, "tools")
	err = os.MkdirAll(toolsDir, 0755)
	if err != nil {
		return err
	}
	dir, err := ioutil.TempDir(toolsDir, "unpacking-")
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
			return fmt.Errorf("bad name %q in tools archive", hdr.Name)
		}
		if hdr.Typeflag != tar.TypeReg {
			return fmt.Errorf("bad file type %c in file %q in tools archive", hdr.Typeflag, hdr.Name)
		}
		name := path.Join(dir, hdr.Name)
		if err := writeFile(name, os.FileMode(hdr.Mode&0777), tr); err != nil {
			return fmt.Errorf("tar extract %q failed: %v", name, err)
		}
	}
	toolsMetadataData, err := json.Marshal(tools)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(path.Join(dir, toolsFile), []byte(toolsMetadataData), 0644)
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
	logger.Warningf("cannot remove %q: %v", dir, err)
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
	toolsData, err := ioutil.ReadFile(path.Join(dir, toolsFile))
	if err != nil {
		return nil, fmt.Errorf("cannot read tools metadata in tools directory: %v", err)
	}
	var tools coretools.Tools
	if err := json.Unmarshal(toolsData, &tools); err != nil {
		return nil, fmt.Errorf("invalid tools metadata in tools directory %q: %v", dir, err)
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
	tmpName := ToolsDir(dataDir, "tmplink-"+agentName)
	err = os.Symlink(tools.Version.String(), tmpName)
	if err != nil {
		return nil, fmt.Errorf("cannot create tools symlink: %v", err)
	}
	err = os.Rename(tmpName, ToolsDir(dataDir, agentName))
	if err != nil {
		return nil, fmt.Errorf("cannot update tools symlink: %v", err)
	}
	return tools, nil
}
