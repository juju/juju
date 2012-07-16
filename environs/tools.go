package environs

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/version"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

var toolPrefix = "tools/juju-"

var toolFilePat = regexp.MustCompile(`^` + toolPrefix + `(\d+\.\d+\.\d+)-([^-]+)-([^-]+)\.tgz$`)

// ToolsPath returns the path that is used to store and
// retrieve the juju tools in a Storage.
func ToolsPath(v version.Version, series, arch string) string {
	return fmt.Sprintf(toolPrefix+"%v-%s-%s.tgz", v, series, arch)
}

// PutTools uploads the current version of the juju tools
// executables to the given storage.
// TODO find binaries from $PATH when not using a development
// version of juju within a $GOPATH.
func PutTools(storage StorageWriter) error {
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
	fi, err := f.Stat()
	if err != nil {
		return err
	}
	p := ToolsPath(version.Current, config.CurrentSeries, config.CurrentArch)
	log.Printf("environs: putting tools %v", p)
	return storage.Put(p, f, fi.Size())
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

type toolsSpec struct {
	vers   version.Version
	series string
	arch   string
}

// FindTools tries to find a set of tools appropriate for the given
// version, Ubuntu series and architecture, and returns a URL that can
// be used to access them in gzipped tar archive format.
func FindTools(env Environ, vers version.Version, series, arch string) (url string, err error) {
	storage, path, err := findTools(env, toolsSpec{vers, series, arch})
	if err != nil {
		return "", err
	}
	return storage.URL(path)
}

// GetTools finds the latest compatible version of the juju tools
// and downloads them into the given directory.
func GetTools(env Environ, dir string) error {
	storage, path, err := findTools(env, toolsSpec{version.Current, config.CurrentSeries, config.CurrentArch})
	if err != nil {
		return err
	}
	r, err := storage.Get(path)
	if err != nil {
		return err
	}
	defer r.Close()

	r, err = gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer r.Close()

	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return err
		}
		if strings.Contains(hdr.Name, "/\\") {
			return fmt.Errorf("bad name %q in tools archive", hdr.Name)
		}

		name := filepath.Join(dir, hdr.Name)
		if err := writeFile(name, os.FileMode(hdr.Mode&0777), tr); err != nil {
			return fmt.Errorf("tar extract %q failed: %v", name, err)
		}
	}
	panic("not reached")
}

// findToolsPath is an internal version of FindTools that returns the
// storage in which the tools have been found, and the path within that storage.
func findTools(env Environ, spec toolsSpec) (storage StorageReader, path string, err error) {
	storage = env.Storage()
	path, err = findToolsPath(storage, spec)
	if _, ok := err.(*NotFoundError); ok {
		storage = env.PublicStorage()
		path, err = findToolsPath(storage, spec)
	}
	if err != nil {
		return nil, "", err
	}
	return
}

// findToolsPath looks for the tools in the given storage.
func findToolsPath(store StorageReader, spec toolsSpec) (path string, err error) {
	names, err := store.List(fmt.Sprintf("%s%d.", toolPrefix, spec.vers.Major))
	log.Printf("findTools searching for %v in %q", spec, names)
	if err != nil {
		return "", err
	}
	if len(names) == 0 {
		return "", &NotFoundError{fmt.Errorf("no compatible tools found")}
	}
	bestVersion := version.Version{Major: -1}
	bestName := ""
	for _, name := range names {
		m := toolFilePat.FindStringSubmatch(name)
		if m == nil {
			log.Printf("unexpected tools file found %q", name)
			continue
		}
		vers, err := version.Parse(m[1])
		if err != nil {
			log.Printf("failed to parse version %q: %v", name, err)
			continue
		}
		if m[2] != spec.series {
			continue
		}
		// TODO allow different architectures.
		if m[3] != spec.arch {
			continue
		}
		if vers.Major != spec.vers.Major {
			continue
		}
		if bestVersion.Less(vers) {
			bestVersion = vers
			bestName = name
		}
	}
	if bestVersion.Major < 0 {
		return "", &NotFoundError{fmt.Errorf("no compatible tools found")}
	}
	return bestName, nil
}

// bundleTools bundles all the current juju tools in gzipped tar
// format to the given writer.
func bundleTools(w io.Writer) error {
	dir, err := ioutil.TempDir("", "juju-tools")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	cmd := exec.Command("go", "install", "launchpad.net/juju-core/cmd/...")
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

func writeFile(name string, mode os.FileMode, r io.Reader) error {
	f, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, r)
	return err
}

// EmptyStorage holds a StorageReader object that contains nothing.
var EmptyStorage StorageReader = emptyStorage{}

type emptyStorage struct{}

func (s emptyStorage) Get(name string) (io.ReadCloser, error) {
	return nil, &NotFoundError{fmt.Errorf("file %q not found in empty storage", name)}
}

func (s emptyStorage) URL(string) (string, error) {
	return "", fmt.Errorf("empty storage has no URLs")
}

func (s emptyStorage) List(prefix string) ([]string, error) {
	return nil, nil
}
