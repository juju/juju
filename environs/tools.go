package environs

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"launchpad.net/juju/go/version"
	"os"
	"os/exec"
	"path/filepath"
)

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

// archive writes the executable files found in the given
// directory in gzipped tar format to w.
// An error is returned if an entry inside dir is not
// a regular executable file.
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
		if !isExecutable(ent) {
			return fmt.Errorf("archive: found non-executable file %q", filepath.Join(dir, ent.Name()))
		}
		h := tarHeader(ent)
		// ignore local umask
		h.Mode = 0755
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

// closeErrorCheck means that we can ensure that
// Close errors do not get lost even when we defer them,
func closeErrorCheck(errp *error, c io.Closer) {
	err := c.Close()
	if *errp == nil {
		*errp = err
	}
}

func copyFile(w io.Writer, file string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(w, f)
	return err
}

// bundleTools bundles all the current juju tools in gzipped tar
// format to the given writer.
func bundleTools(w io.Writer) error {
	dir, err := ioutil.TempDir("", "juju-tools")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	cmd := exec.Command("go", "install", "launchpad.net/juju/go/cmd/...")
	cmd.Env = []string{
		"GOPATH=" + os.Getenv("GOPATH"),
		"GOBIN=" + dir,
		"PATH=" + os.Getenv("PATH"),
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build failed: %v; %s", err, out)
	}
	return archive(w, dir)
}

// UploadTools uploads the current version of the juju tools
// executables to the given environment.
// TODO find binaries from $PATH when go dev environment not available.
func UploadTools(env Environ) error {
	// We create the entire archive before asking the environment to
	// start uploading so that we can be sure we have archived
	// correctly.
	f, err := ioutil.TempFile("", "juju-tgz")
	if err != nil {
		return err
	}
	defer f.Close()
	defer os.Remove(f.Name())
	err = bundleTools(f)
	if err != nil {
		return err
	}
	_, err = f.Seek(0, 0)
	if err != nil {
		return err
	}
	return env.PutFile(version.ToolsPath, f)
}
