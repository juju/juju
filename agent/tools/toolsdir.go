// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strings"

	"launchpad.net/juju-core/version"
)

const urlFile = "downloaded-url.txt"

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

// Dir returns the agent-specific data directory.
func Dir(dataDir, agentName string) string {
	return path.Join(dataDir, "agents", agentName)
}

// UnpackTools reads a set of juju tools in gzipped tar-archive
// format and unpacks them into the appropriate tools directory
// within dataDir. If a valid tools directory already exists,
// UnpackTools returns without error.
func UnpackTools(dataDir string, tools *Tools, r io.Reader) (err error) {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer zr.Close()

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

	tr := tar.NewReader(zr)
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
	err = ioutil.WriteFile(path.Join(dir, urlFile), []byte(tools.URL), 0644)
	if err != nil {
		return err
	}

	err = os.Rename(dir, SharedToolsDir(dataDir, tools.Binary))
	// If we've failed to rename the directory, it may be because
	// the directory already exists - if ReadTools succeeds, we
	// assume all's ok.
	if err != nil {
		if _, err := ReadTools(dataDir, tools.Binary); err == nil {
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

// ReadTools checks that the tools for the given version exist
// in the dataDir directory, and returns a Tools instance describing them.
func ReadTools(dataDir string, vers version.Binary) (*Tools, error) {
	dir := SharedToolsDir(dataDir, vers)
	urlData, err := ioutil.ReadFile(path.Join(dir, urlFile))
	if err != nil {
		return nil, fmt.Errorf("cannot read URL in tools directory: %v", err)
	}
	url := strings.TrimSpace(string(urlData))
	if len(url) == 0 {
		return nil, fmt.Errorf("empty URL in tools directory %q", dir)
	}
	// TODO(rog): do more verification here too, such as checking
	// for the existence of certain files.
	return &Tools{
		URL:    url,
		Binary: vers,
	}, nil
}

// ChangeAgentTools atomically replaces the agent-specific symlink
// under dataDir so it points to the previously unpacked
// version vers. It returns the new tools read.
func ChangeAgentTools(dataDir string, agentName string, vers version.Binary) (*Tools, error) {
	tools, err := ReadTools(dataDir, vers)
	if err != nil {
		return nil, err
	}
	tmpName := ToolsDir(dataDir, "tmplink-"+agentName)
	err = os.Symlink(tools.Binary.String(), tmpName)
	if err != nil {
		return nil, fmt.Errorf("cannot create tools symlink: %v", err)
	}
	err = os.Rename(tmpName, ToolsDir(dataDir, agentName))
	if err != nil {
		return nil, fmt.Errorf("cannot update tools symlink: %v", err)
	}
	return tools, nil
}
