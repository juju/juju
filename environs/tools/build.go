// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/version/v2"

	"github.com/juju/juju/core/arch"
	corelogger "github.com/juju/juju/core/logger"
	coreos "github.com/juju/juju/core/os"
	jujuversion "github.com/juju/juju/core/version"
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
			logger.Errorf(context.TODO(), "failed to read file info: %s", ent.Name())
			continue
		}

		h := tarHeader(fi)
		logger.Debugf(context.TODO(), "adding entry: %#v", h)
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

func findExecutable(ctx context.Context, execFile string) (string, error) {
	logger.Debugf(ctx, "looking for: %s", execFile)
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

func copyFileWithMode(ctx context.Context, from, to string, mode os.FileMode) error {
	source, err := os.Open(from)
	if err != nil {
		logger.Infof(ctx, "open source failed: %v", err)
		return err
	}
	defer source.Close()
	destination, err := os.OpenFile(to, os.O_RDWR|os.O_TRUNC|os.O_CREATE, mode)
	if err != nil {
		logger.Infof(ctx, "open destination failed: %v", err)
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
	jujuLocation, err := findExecutable(context.TODO(), os.Args[0])
	if err != nil {
		logger.Infof(context.TODO(), "%v", err)
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

func copyExistingJujus(ctx context.Context, dir string, skipCopyVersionFile bool) error {
	// Assume that the user is running juju.
	jujuDir, err := ExistingJujuLocation()
	if err != nil {
		logger.Infof(ctx, "couldn't find existing jujud: %v", err)
		return errors.Trace(err)
	}
	jujudLocation := filepath.Join(jujuDir, names.Jujud)
	logger.Debugf(ctx, "checking: %s", jujudLocation)
	info, err := os.Stat(jujudLocation)
	if err != nil {
		logger.Infof(ctx, "couldn't find existing jujud: %v", err)
		return errors.Trace(err)
	}
	logger.Infof(ctx, "Found agent binary to upload (%s)", jujudLocation)
	target := filepath.Join(dir, names.Jujud)
	logger.Infof(ctx, "target: %v", target)
	err = copyFileWithMode(ctx, jujudLocation, target, info.Mode())
	if err != nil {
		return errors.Trace(err)
	}
	jujucLocation := filepath.Join(jujuDir, names.Jujuc)
	jujucTarget := filepath.Join(dir, names.Jujuc)
	if _, err = os.Stat(jujucLocation); os.IsNotExist(err) {
		logger.Infof(ctx, "jujuc not found at %s, not including", jujucLocation)
	} else if err != nil {
		return errors.Trace(err)
	} else {
		logger.Infof(ctx, "target jujuc: %v", jujucTarget)
		err = copyFileWithMode(ctx, jujucLocation, jujucTarget, info.Mode())
		if err != nil {
			return errors.Trace(err)
		}
	}
	if skipCopyVersionFile {
		return nil
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
		logger.Infof(ctx, "including versions file %q", versionPath)
		return errors.Trace(copyFileWithMode(ctx, versionPath, versionTarget, info.Mode()))
	}
	return nil
}

func buildJujus(ctx context.Context, dir string) error {
	logger.Infof(ctx, "building jujud")

	// Determine if we are in tree of juju and if to prefer
	// vendor or readonly mod deps.
	var lastErr error
	var cmdDir string
	for _, m := range []string{"-mod=vendor", "-mod=readonly"} {
		var stdout, stderr bytes.Buffer
		cmd := exec.Command("go", "list", "-json", m, "github.com/juju/juju")
		cmd.Env = append(os.Environ(), "GO111MODULE=on")
		cmd.Stderr = &stderr
		cmd.Stdout = &stdout
		err := cmd.Run()
		if err != nil {
			lastErr = fmt.Errorf(`cannot build juju agent outside of github.com/juju/juju tree
			cd into the directory containing juju version=%s commit=%s: %w:
			%s`, jujuversion.Current.String(), jujuversion.GitCommit, err, stderr.String())
			continue
		}
		pkg := struct {
			Root string `json:"Root"`
		}{}
		err = json.Unmarshal(stdout.Bytes(), &pkg)
		if err != nil {
			lastErr = fmt.Errorf("cannot parse go list output for github.com/juju/juju version=%s commit=%s: %w",
				jujuversion.Current.String(), jujuversion.GitCommit, err)
			continue
		}
		lastErr = nil
		cmdDir = pkg.Root
		break
	}
	if lastErr != nil {
		return lastErr
	}

	// Build binaries.
	cmds := [][]string{
		{"make", "jujud-controller"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Env = append(os.Environ(), "GOBIN="+dir)
		cmd.Dir = cmdDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("build command %q failed: %v; %s", args[0], err, out)
		}
		if logger.IsLevelEnabled(corelogger.TRACE) {
			logger.Tracef(ctx, "Built jujud:\n%s", out)
		}
	}
	return nil
}

func packageLocalTools(ctx context.Context, toolsDir string, buildAgent bool) error {
	if !buildAgent {
		if err := copyExistingJujus(ctx, toolsDir, true); err != nil {
			return errors.New("no prepackaged agent available and no jujud binary can be found")
		}
		return nil
	}
	logger.Infof(ctx, "Building agent binary to upload (%s)", jujuversion.Current.String())
	if err := buildJujus(ctx, toolsDir); err != nil {
		return errors.Annotate(err, "cannot build jujud agent binary from source")
	}
	return nil
}

// BundleToolsFunc is a function which can bundle all the current juju tools
// in gzipped tar format to the given writer.
type BundleToolsFunc func(
	build bool, w io.Writer,
	getForceVersion func(version.Number) version.Number,
) (builtVersion version.Binary, forceVersion version.Number, _ bool, _ string, _ error)

// Override for testing.
var BundleTools BundleToolsFunc = func(
	build bool, w io.Writer,
	getForceVersion func(version.Number) version.Number,
) (version.Binary, version.Number, bool, string, error) {
	return bundleTools(context.TODO(), build, w, getForceVersion, JujudVersion)
}

// bundleTools bundles all the current juju tools in gzipped tar
// format to the given writer. A FORCE-VERSION file is included in
// the tools bundle so it will lie about its current version number.
func bundleTools(
	ctx context.Context,
	build bool, w io.Writer,
	getForceVersion func(version.Number) version.Number,
	jujudVersion func(dir string) (version.Binary, bool, error),
) (_ version.Binary, _ version.Number, official bool, sha256hash string, _ error) {
	dir, err := os.MkdirTemp("", "juju-tools")
	if err != nil {
		return version.Binary{}, version.Number{}, false, "", err
	}
	defer os.RemoveAll(dir)

	existingJujuLocation, err := ExistingJujuLocation()
	if err != nil {
		return version.Binary{}, version.Number{}, false, "", errors.Annotate(err, "couldn't find existing jujud")
	}
	_, official, err = jujudVersion(existingJujuLocation)
	if err != nil && !errors.Is(err, errors.NotFound) {
		return version.Binary{}, version.Number{}, official, "", errors.Trace(err)
	}
	if official && build {
		return version.Binary{}, version.Number{}, official, "", errors.Errorf("cannot build agent for official build")
	}

	if err := packageLocalTools(ctx, dir, build); err != nil {
		return version.Binary{}, version.Number{}, false, "", err
	}

	// We need to get the version again because the juju binaries at dir might be built from source code.
	tvers, official, err := jujudVersion(dir)
	if err != nil {
		return version.Binary{}, version.Number{}, false, "", errors.Trace(err)
	}
	if official {
		logger.Debugf(ctx, "using official version %s", tvers)
	}
	forceVersion := getForceVersion(tvers.Number)
	logger.Debugf(ctx, "forcing version to %s", forceVersion)
	if err := os.WriteFile(filepath.Join(dir, "FORCE-VERSION"), []byte(forceVersion.String()), 0666); err != nil {
		return version.Binary{}, version.Number{}, false, "", err
	}

	sha256hash, err = archiveAndSHA256(w, dir)
	if err != nil {
		return version.Binary{}, version.Number{}, false, "", err
	}
	return tvers, forceVersion, official, sha256hash, err
}

// Override for testing.
var ExecCommand = exec.Command

func getVersionFromJujud(dir string) (version.Binary, error) {
	// If there's no jujud, return a NotFound error.
	path := filepath.Join(dir, names.Jujud)
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return version.Binary{}, errors.NotFoundf(path)
		}
		return version.Binary{}, errors.Trace(err)
	}
	cmd := ExecCommand(path, "version")
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
	if err != nil && !errors.Is(err, errors.NotFound) && !isNoMatchingToolsChecksum(err) {
		return version.Binary{}, false, errors.Trace(err)
	}
	if errors.Is(err, errors.NotFound) || isNoMatchingToolsChecksum(err) {
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
	thisHost := coreos.HostOSTypeName()
	var current version.Binary
	for _, ver := range versions {
		var err error
		current, err = version.ParseBinary(ver)
		if err != nil {
			return version.Binary{}, errors.Trace(err)
		}
		if current.Release == thisHost && current.Arch == thisArch {
			return current, nil
		}
	}
	// There's no version matching our osType/arch, but the signature
	// still matches the binary for all versions passed in, so just
	// punt.
	return current, nil
}
