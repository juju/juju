package environs

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/version"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var toolPrefix = "tools/juju-"

var toolFilePat = regexp.MustCompile(`^` + toolPrefix + `(.*)\.tgz$`)

// Tools describes a particular set of juju tools and where to find them.
type Tools struct {
	version.Binary
	URL string
}

// ListTools returns all the tools found in the given storage
// that have the given major version.
func ListTools(store StorageReader, majorVersion int) ([]*Tools, error) {
	dir := fmt.Sprintf("%s%d.", toolPrefix, majorVersion)
	names, err := store.List(dir)
	if err != nil {
		return nil, err
	}
	var toolsList []*Tools
	for _, name := range names {
		if !strings.HasPrefix(name, toolPrefix) || !strings.HasSuffix(name, ".tgz") {
			log.Printf("unexpected tools file found %q", name)
			continue
		}
		vers := name[len(toolPrefix):len(name)-len(".tgz")]
		var t Tools
		t.Binary, err = version.ParseBinary(vers)
		if err != nil {
			log.Printf("failed to parse %q: %v", vers, err)
			continue
		}
		if t.Major != majorVersion {
			log.Printf("tool %q found in wrong directory %q", name, dir)
			continue
		}
		t.URL, err = store.URL(name)
		if err != nil {
			log.Printf("cannot get URL for %q: %v", name, err)
			continue
		}
		toolsList = append(toolsList, &t)
	}
	return toolsList, nil
}

// PutTools uploads the current version of the juju tools
// executables to the given storage and returns a Tools
// instance describing them.
// TODO find binaries from $PATH when not using a development
// version of juju within a $GOPATH.
func PutTools(storage Storage) (*Tools, error) {
	// We create the entire archive before asking the environment to
	// start uploading so that we can be sure we have archived
	// correctly.
	f, err := ioutil.TempFile("", "juju-tgz")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	defer os.Remove(f.Name())
	err = bundleTools(f)
	if err != nil {
		return nil, err
	}
	_, err = f.Seek(0, 0)
	if err != nil {
		return nil, fmt.Errorf("cannot seek to start of tools archive: %v", err)
	}
	fi, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("cannot stat newly made tools archive: %v", err)
	}
	p := ToolsPath(version.Current)
	log.Printf("environs: putting tools %v", p)
	err = storage.Put(p, f, fi.Size())
	if err != nil {
		return nil, err
	}
	url, err := storage.URL(p)
	if err != nil {
		return nil, err
	}
	return &Tools{version.Current, url}, nil
}

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

// BestTools the most recent tools compatible with the
// given specification. It returns nil if nothing appropriate
// was found.
func BestTools(toolsList []*Tools, vers version.Binary) *Tools {
	var bestTools *Tools
	for _, t := range toolsList {
		t := t
		if t.Major != vers.Major ||
			t.Series != vers.Series ||
			t.Arch != vers.Arch {
			continue
		}
		if bestTools == nil || bestTools.Number.Less(t.Number) {
			bestTools = t
		}
	}
	return bestTools
}

// GetTools downloads a set of tools from the given URL
// into the given directory.
func GetTools(url, dir string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	r, err := gzip.NewReader(resp.Body)
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

// ToolsPath returns path that is used to store and
// retrieve the given version of the juju tools in a Storage.
func ToolsPath(vers version.Binary) string {
	return toolPrefix+vers.String()+".tgz"
}

// FindTools tries to find a set of tools compatible
// with the given version from the given environment.
// If no tools are found and there's no other error, a NotFoundError
// is returned.
func FindTools(env Environ, vers version.Binary) (*Tools, error) {
	// If there's anything compatible in the environ's Storage,
	// it gets precedence over anything in its PublicStorage.
	toolsList, err := ListTools(env.Storage(), vers.Major)
	if err != nil {
		return nil, err
	}
	tools := BestTools(toolsList, vers)
	if tools != nil {
		return tools, nil
	}
	toolsList, err = ListTools(env.PublicStorage(), vers.Major)
	if err != nil {
		return nil, err
	}
	tools = BestTools(toolsList, vers)
	if tools == nil {
		return nil, &NotFoundError{fmt.Errorf("no compatible tools found")}
	}
	return tools, nil
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
func bundleTools(w io.Writer) error {
	dir, err := ioutil.TempDir("", "juju-tools")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	cmd := exec.Command("go", "install", "launchpad.net/juju-core/cmd/...")
	cmd.Env = setenv(os.Environ(), "GOBIN="+dir)
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
