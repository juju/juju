// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"launchpad.net/juju-core/version"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// archive writes the executable files found in the given directory in
// gzipped tar format to w.  An error is returned if an entry inside dir
// is not a regular executable file.
func archive(w io.Writer, dir string) (err error) {
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
		if err := copyFile(tarw, filepath.Join(dir, ent.Name())); err != nil {
			return err
		}
	}
	return nil
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

func setenv(env []string, val string) []string {
	prefix := val[0 : strings.Index(val, "=")+1]
	for i, eval := range env {
		if strings.HasPrefix(eval, prefix) {
			env[i] = val
			return env
		}
	}
	return append(env, val)
}

// bundleTools bundles all the current juju tools in gzipped tar
// format to the given writer.
// If forceVersion is not nil, a FORCE-VERSION file is included in
// the tools bundle so it will lie about its current version number.
func bundleTools(w io.Writer, forceVersion *version.Number) (version.Binary, error) {
	dir, err := ioutil.TempDir("", "juju-tools")
	if err != nil {
		return version.Binary{}, err
	}
	defer os.RemoveAll(dir)

	cmds := [][]string{
		{"go", "install", "launchpad.net/juju-core/cmd/jujud"},
		{"strip", dir + "/jujud"},
	}
	env := setenv(os.Environ(), "GOBIN="+dir)
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			return version.Binary{}, fmt.Errorf("build command %q failed: %v; %s", args[0], err, out)
		}
	}
	if forceVersion != nil {
		if err := ioutil.WriteFile(filepath.Join(dir, "FORCE-VERSION"), []byte(forceVersion.String()), 0666); err != nil {
			return version.Binary{}, err
		}
	}
	cmd := exec.Command(filepath.Join(dir, "jujud"), "version")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return version.Binary{}, fmt.Errorf("cannot get version from %q: %v; %s", cmd.Args[0], err, out)
	}
	tvs := strings.TrimSpace(string(out))
	tvers, err := version.ParseBinary(tvs)
	if err != nil {
		return version.Binary{}, fmt.Errorf("invalid version %q printed by jujud", tvs)
	}
	err = archive(w, dir)
	if err != nil {
		return version.Binary{}, err
	}
	return tvers, err
}
