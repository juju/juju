// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/version/v2"

	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/internal/devtools"
	"github.com/juju/juju/juju/names"
)

// Archive writes the executable files found in the given directory in
// gzipped tar format to w.
func Archive(w io.Writer, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	gzw := gzip.NewWriter(w)
	defer closeErrorCheck(&err, gzw)

	tarw := tar.NewWriter(gzw)
	defer closeErrorCheck(&err, tarw)

	for _, ent := range entries {
		fi, err := ent.Info()
		if err != nil {
			logger.Errorf("failed to read file info: %s", ent.Name())
			continue
		}

		h := tarHeader(fi)
		logger.Debugf("adding entry: %#v", h)
		// ignore local umask
		if isExecutable(fi) {
			h.Mode = 0755
		} else {
			h.Mode = 0644
		}
		err = tarw.WriteHeader(h)
		if err != nil {
			return err
		}
		fileName := filepath.Join(dir, ent.Name())
		if err := copyFile(tarw, fileName); err != nil {
			return err
		}
	}
	return nil
}

// archiveAndSHA256 calls Archive with the provided arguments,
// and returns a hex-encoded SHA256 hash of the resulting
// archive.
func archiveAndSHA256(w io.Writer, dir string) (sha256hash string, err error) {
	h := sha256.New()
	if err := Archive(io.MultiWriter(h, w), dir); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), err
}

// copyFile writes the contents of the given file to w.
func copyFile(w io.Writer, file string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return err
}

// tarHeader returns a tar file header given the file's stat
// information.
func tarHeader(i os.FileInfo) *tar.Header {
	return &tar.Header{
		Typeflag:   tar.TypeReg,
		Name:       i.Name(),
		Size:       i.Size(),
		Mode:       int64(i.Mode() & 0777),
		ModTime:    i.ModTime(),
		AccessTime: i.ModTime(),
		ChangeTime: i.ModTime(),
		Uname:      "ubuntu",
		Gname:      "ubuntu",
	}
}

// isExecutable returns whether the given info
// represents a regular file executable by (at least) the user.
func isExecutable(i os.FileInfo) bool {
	return i.Mode()&(0100|os.ModeType) == 0100
}

// closeErrorCheck means that we can ensure that
// Close errors do not get lost even when we defer them,
func closeErrorCheck(errp *error, c io.Closer) {
	err := c.Close()
	if *errp == nil {
		*errp = err
	}
}

func findExecutable(execFile string) (string, error) {
	logger.Debugf("looking for: %s", execFile)
	if filepath.IsAbs(execFile) {
		return execFile, nil
	}

	dir, file := filepath.Split(execFile)

	// Now we have two possibilities:
	//   file == path indicating that the PATH was searched
	//   dir != "" indicating that it is a relative path

	if dir == "" {
		path := os.Getenv("PATH")
		for _, name := range filepath.SplitList(path) {
			result := filepath.Join(name, file)
			// Use exec.LookPath() to check if the file exists and is executable`
			f, err := exec.LookPath(result)
			if err == nil {
				return f, nil
			}
		}

		return "", fmt.Errorf("could not find %q in the path", file)
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Clean(filepath.Join(cwd, execFile)), nil
}

func copyFileWithMode(from, to string, mode os.FileMode) error {
	source, err := os.Open(from)
	if err != nil {
		logger.Infof("open source failed: %v", err)
		return err
	}
	defer source.Close()
	destination, err := os.OpenFile(to, os.O_RDWR|os.O_TRUNC|os.O_CREATE, mode)
	if err != nil {
		logger.Infof("open destination failed: %v", err)
		return err
	}
	defer destination.Close()
	_, err = io.Copy(destination, source)
	if err != nil {
		return err
	}
	return nil
}

// Override for testing.
var ExistingJujuLocation = existingJujuLocation

// ExistingJujuLocation returns the directory where 'juju' is running, and where
// we expect to find 'jujuc' and 'jujud'.
func existingJujuLocation() (string, error) {
	jujuLocation, err := findExecutable(os.Args[0])
	if err != nil {
		logger.Infof("%v", err)
		return "", err
	}
	jujuDir := filepath.Dir(jujuLocation)
	return jujuDir, nil
}

// VersionFileFallbackDir is the other location we'll check for a
// juju-versions file if it's not alongside the binary (for example if
// Juju was installed from a .deb). (Exposed so we can override it in
// tests.)
var VersionFileFallbackDir = "/usr/lib/juju"

func copyBins(srcDir string, targetDir string) error {
	jujudLocation := filepath.Join(srcDir, names.Jujud)
	logger.Debugf("checking: %s", jujudLocation)
	info, err := os.Stat(jujudLocation)
	if err != nil {
		logger.Infof("couldn't find existing jujud: %v", err)
		return errors.Trace(err)
	}
	logger.Infof("Found agent binary to upload (%s)", jujudLocation)
	target := filepath.Join(targetDir, names.Jujud)
	logger.Infof("target: %v", target)
	err = copyFileWithMode(jujudLocation, target, info.Mode())
	if err != nil {
		return errors.Trace(err)
	}
	jujucLocation := filepath.Join(srcDir, names.Jujuc)
	jujucTarget := filepath.Join(targetDir, names.Jujuc)
	if _, err = os.Stat(jujucLocation); os.IsNotExist(err) {
		logger.Infof("jujuc not found at %s, not including", jujucLocation)
	} else if err != nil {
		return errors.Trace(err)
	} else {
		logger.Infof("target jujuc: %v", jujucTarget)
		err = copyFileWithMode(jujucLocation, jujucTarget, info.Mode())
		if err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

type BundleToolsFunc func(devSrcDir string, toolsArch arch.Arch, w io.Writer) (toolsVersion version.Binary, sha256hash string, err error)

// BundleTools bundles all the current juju tools in gzipped tar
// format to the given writer.
var BundleTools BundleToolsFunc = bundleTools

// bundleTools bundles all the current juju tools in gzipped tar
// format to the given writer.
func bundleTools(devSrcDir string, toolsArch arch.Arch, w io.Writer) (_ version.Binary, sha256hash string, _ error) {
	dir, err := os.MkdirTemp("", "juju-tools")
	if err != nil {
		return version.Binary{}, "", err
	}
	defer os.RemoveAll(dir)

	binDir := path.Join(devSrcDir, "_build", "linux_"+arch.ToGoArch(toolsArch), "bin")
	err = copyBins(binDir, dir)
	if err != nil {
		return version.Binary{}, "", errors.New("no prepackaged agent available and no jujud binary can be found")
	}

	jujudBin := filepath.Join(dir, names.Jujud)
	toolsVersion, err := devtools.ELFExtractVersion(jujudBin)
	if err != nil {
		return version.Binary{}, "", err
	}
	if toolsVersion.Arch != toolsArch {
		return version.Binary{}, "", fmt.Errorf("invalid architecture %q for %s: expected %q", toolsVersion.Arch, jujudBin, toolsArch)
	}

	sha256hash, err = archiveAndSHA256(w, dir)
	if err != nil {
		return version.Binary{}, "", err
	}
	return toolsVersion, sha256hash, nil
}
