// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/os/series"
	"github.com/juju/utils/arch"
	"github.com/juju/version"

	"github.com/juju/juju/juju/names"
	jujuversion "github.com/juju/juju/version"
)

// Archive writes the executable files found in the given directory in
// gzipped tar format to w.
func Archive(w io.Writer, dir string) error {
	entries, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}

	gzw := gzip.NewWriter(w)
	defer closeErrorCheck(&err, gzw)

	tarw := tar.NewWriter(gzw)
	defer closeErrorCheck(&err, tarw)

	for _, ent := range entries {
		h := tarHeader(ent)
		logger.Debugf("adding entry: %#v", h)
		// ignore local umask
		if isExecutable(ent) {
			h.Mode = 0755
		} else {
			h.Mode = 0644
		}
		err := tarw.WriteHeader(h)
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

// ExistingJujuLocation returns the directory where 'juju' is running, and where
// we expect to find 'jujuc' and 'jujud'.
func ExistingJujuLocation() (string, error) {
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

func copyExistingJujus(dir string) error {
	// Assume that the user is running juju.
	jujuDir, err := ExistingJujuLocation()
	if err != nil {
		logger.Infof("couldn't find existing jujud: %v", err)
		return errors.Trace(err)
	}
	jujudLocation := filepath.Join(jujuDir, names.Jujud)
	logger.Debugf("checking: %s", jujudLocation)
	info, err := os.Stat(jujudLocation)
	if err != nil {
		logger.Infof("couldn't find existing jujud: %v", err)
		return errors.Trace(err)
	}
	logger.Infof("Found agent binary to upload (%s)", jujudLocation)
	target := filepath.Join(dir, names.Jujud)
	logger.Infof("target: %v", target)
	err = copyFileWithMode(jujudLocation, target, info.Mode())
	if err != nil {
		return errors.Trace(err)
	}
	jujucLocation := filepath.Join(jujuDir, names.Jujuc)
	jujucTarget := filepath.Join(dir, names.Jujuc)
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
	// If there's a version file beside the jujud binary or in the
	// fallback location, include that.
	versionTarget := filepath.Join(dir, names.JujudVersions)

	versionPaths := []string{
		filepath.Join(jujuDir, names.JujudVersions),
		filepath.Join(VersionFileFallbackDir, names.JujudVersions),
	}
	for _, versionPath := range versionPaths {
		info, err = os.Stat(versionPath)
		if os.IsNotExist(err) {
			continue
		} else if err != nil {
			return errors.Trace(err)
		}
		logger.Infof("including versions file %q", versionPath)
		return errors.Trace(copyFileWithMode(versionPath, versionTarget, info.Mode()))
	}
	return nil
}

func buildJujus(dir string) error {
	logger.Infof("building jujud")

	// Determine if we are in tree of juju and if to prefer
	// vendor or readonly mod deps.
	var modArg string
	var lastErr error
	for _, m := range []string{"-mod=vendor", "-mod=readonly"} {
		cmd := exec.Command("go", "list", m, "github.com/juju/juju")
		cmd.Env = append(os.Environ(), "GO111MODULE=on")
		out, err := cmd.CombinedOutput()
		if err != nil {
			info := `cannot build juju agent outside of github.com/juju/juju tree
	cd into the directory containing juju %s %s
	%s`
			lastErr = errors.Annotatef(err, info, jujuversion.Current.String(), jujuversion.GitCommit, out)
			continue
		}
		modArg = m
		lastErr = nil
		break
	}
	if lastErr != nil {
		return lastErr
	}

	// Build binaries.
	cmds := [][]string{
		// TODO: jam 2020-03-12 do we want to also default to stripping the binary?
		//       -ldflags "-s -w"
		{"go", "build", modArg, "-ldflags", "-extldflags \"-static\"", "-o", filepath.Join(dir, names.Jujud), "github.com/juju/juju/cmd/jujud"},
		{"go", "build", modArg, "-ldflags", "-extldflags \"-static\"", "-o", filepath.Join(dir, names.Jujuc), "github.com/juju/juju/cmd/jujuc"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("build command %q failed: %v; %s", args[0], err, out)
		}
	}
	return nil
}

func packageLocalTools(toolsDir string, buildAgent bool) error {
	if !buildAgent {
		if err := copyExistingJujus(toolsDir); err != nil {
			return errors.New("no prepackaged agent available and no jujud binary can be found")
		}
		return nil
	}
	logger.Infof("Building agent binary to upload (%s)", jujuversion.Current.String())
	if err := buildJujus(toolsDir); err != nil {
		return errors.Annotate(err, "cannot build jujud agent binary from source")
	}
	return nil
}

// BundleToolsFunc is a function which can bundle all the current juju tools
// in gzipped tar format to the given writer.
type BundleToolsFunc func(build bool, w io.Writer, forceVersion *version.Number) (version.Binary, bool, string, error)

// Override for testing.
var BundleTools BundleToolsFunc = bundleTools

// bundleTools bundles all the current juju tools in gzipped tar
// format to the given writer.  If forceVersion is not nil and the
// file isn't an official build, a FORCE-VERSION file is included in
// the tools bundle so it will lie about its current version number.
func bundleTools(build bool, w io.Writer, forceVersion *version.Number) (_ version.Binary, official bool, sha256hash string, _ error) {
	dir, err := ioutil.TempDir("", "juju-tools")
	if err != nil {
		return version.Binary{}, false, "", err
	}
	defer os.RemoveAll(dir)
	if err := packageLocalTools(dir, build); err != nil {
		return version.Binary{}, false, "", err
	}

	tvers, official, err := JujudVersion(dir)
	if err != nil {
		return version.Binary{}, false, "", errors.Trace(err)
	}
	if official {
		logger.Debugf("using official version %s", tvers)
	} else if forceVersion != nil {
		logger.Debugf("forcing version to %s", forceVersion)
		if err := ioutil.WriteFile(filepath.Join(dir, "FORCE-VERSION"), []byte(forceVersion.String()), 0666); err != nil {
			return version.Binary{}, false, "", err
		}
	}

	sha256hash, err = archiveAndSHA256(w, dir)
	if err != nil {
		return version.Binary{}, false, "", err
	}
	return tvers, official, sha256hash, err
}

var execCommand = exec.Command

func getVersionFromJujud(dir string) (version.Binary, error) {
	path := filepath.Join(dir, names.Jujud)
	cmd := execCommand(path, "version")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return version.Binary{}, errors.Errorf("cannot get version from %q: %v; %s", path, err, stderr.String()+stdout.String())
	}
	tvs := strings.TrimSpace(stdout.String())
	tvers, err := version.ParseBinary(tvs)
	if err != nil {
		return version.Binary{}, errors.Errorf("invalid version %q printed by jujud", tvs)
	}
	return tvers, nil
}

// JujudVersion returns the Jujud version at the specified location,
// and whether it is an official binary.
func JujudVersion(dir string) (version.Binary, bool, error) {
	tvers, err := getVersionFromFile(dir)
	official := err == nil
	if err != nil && !errors.IsNotFound(err) && !isNoMatchingToolsChecksum(err) {
		return version.Binary{}, false, errors.Trace(err)
	}
	if errors.IsNotFound(err) || isNoMatchingToolsChecksum(err) {
		// No signature file found.
		// Extract the version number that the jujud binary was built with.
		// This is used to check compatibility with the version of the client
		// being used to bootstrap.
		tvers, err = getVersionFromJujud(dir)
		if err != nil {
			return version.Binary{}, false, errors.Trace(err)
		}
	}
	return tvers, official, nil
}

type noMatchingToolsChecksum struct {
	versionPath string
	jujudPath   string
}

func (e *noMatchingToolsChecksum) Error() string {
	return fmt.Sprintf("no SHA256 in version file %q matches binary %q", e.versionPath, e.jujudPath)
}

func isNoMatchingToolsChecksum(err error) bool {
	_, ok := err.(*noMatchingToolsChecksum)
	return ok
}

func getVersionFromFile(dir string) (version.Binary, error) {
	versionPath := filepath.Join(dir, names.JujudVersions)
	sigFile, err := os.Open(versionPath)
	if os.IsNotExist(err) {
		return version.Binary{}, errors.NotFoundf("version file %q", versionPath)
	} else if err != nil {
		return version.Binary{}, errors.Trace(err)
	}
	defer sigFile.Close()

	versions, err := ParseVersions(sigFile)
	if err != nil {
		return version.Binary{}, errors.Trace(err)
	}

	// Find the binary by hash.
	jujudPath := filepath.Join(dir, names.Jujud)
	jujudFile, err := os.Open(jujudPath)
	if err != nil {
		return version.Binary{}, errors.Trace(err)
	}
	defer jujudFile.Close()
	matching, err := versions.VersionsMatching(jujudFile)
	if err != nil {
		return version.Binary{}, errors.Trace(err)
	}
	if len(matching) == 0 {
		return version.Binary{}, &noMatchingToolsChecksum{versionPath, jujudPath}
	}
	return selectBinary(matching)
}

func selectBinary(versions []string) (version.Binary, error) {
	thisArch := arch.HostArch()
	thisSeries, err := series.HostSeries()
	if err != nil {
		return version.Binary{}, errors.Trace(err)
	}
	var current version.Binary
	for _, ver := range versions {
		current, err = version.ParseBinary(ver)
		if err != nil {
			return version.Binary{}, errors.Trace(err)
		}
		if current.Series == thisSeries && current.Arch == thisArch {
			return current, nil
		}
	}
	// There's no version matching our series/arch, but the signature
	// still matches the binary for all versions passed in, so just
	// punt.
	return current, nil
}
