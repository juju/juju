package environs

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

// VarDir is the directory where juju data is stored.
// The tools directories are stored inside the "tools" subdirectory
// inside VarDir.
var VarDir = "/var/lib/juju"

var toolPrefix = "tools/juju-"

// ToolsList holds a list of available tools.  Private tools take
// precedence over public tools, even if they have a lower
// version number.
type ToolsList struct {
	Private []*state.Tools
	Public  []*state.Tools
}

// ListTools returns a ToolsList holding all the tools
// available in the given environment that have the
// given major version.
func ListTools(env Environ, majorVersion int) (*ToolsList, error) {
	private, err := listTools(env.Storage(), majorVersion)
	if err != nil {
		return nil, err
	}
	public, err := listTools(env.PublicStorage(), majorVersion)
	if err != nil {
		return nil, err
	}
	return &ToolsList{
		Private: private,
		Public:  public,
	}, nil
}

// listTools is like ListTools, but only returns the tools from
// a particular storage.
func listTools(store StorageReader, majorVersion int) ([]*state.Tools, error) {
	dir := fmt.Sprintf("%s%d.", toolPrefix, majorVersion)
	names, err := store.List(dir)
	if err != nil {
		return nil, err
	}
	var toolsList []*state.Tools
	for _, name := range names {
		if !strings.HasPrefix(name, toolPrefix) || !strings.HasSuffix(name, ".tgz") {
			log.Printf("unexpected tools file found %q", name)
			continue
		}
		vers := name[len(toolPrefix) : len(name)-len(".tgz")]
		var t state.Tools
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

// PutTools builds the current version of the juju tools, uploads them
// to the given storage, and returns a Tools instance describing them.
// If vers is non-nil it will override the current version in the uploaded
// tools.
func PutTools(storage Storage, vers *version.Binary) (*state.Tools, error) {
	// TODO(rog) find binaries from $PATH when not using a development
	// version of juju within a $GOPATH.

	// We create the entire archive before asking the environment to
	// start uploading so that we can be sure we have archived
	// correctly.
	f, err := ioutil.TempFile("", "juju-tgz")
	if err != nil {
		return nil, err
	}
	defer f.Close()
	defer os.Remove(f.Name())
	tvers, err := bundleTools(f, vers)
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
	p := ToolsStoragePath(tvers)
	log.Printf("environs: putting tools %v", p)
	err = storage.Put(p, f, fi.Size())
	if err != nil {
		return nil, err
	}
	url, err := storage.URL(p)
	if err != nil {
		return nil, err
	}
	return &state.Tools{tvers, url}, nil
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

// BestTools returns the most recent version
// from the set of tools in the ToolsList that are
// compatible with the given version, and with a version
// number <= vers.Number, or nil if no such tools are found.
// If dev is true, it will consider development versions of the tools
// even if vers is not a development version.
func BestTools(list *ToolsList, vers version.Binary, dev bool) *state.Tools {
	if tools := bestTools(list.Private, vers, dev); tools != nil {
		return tools
	}
	return bestTools(list.Public, vers, dev)
}

// bestTools is like BestTools but operates on a single list of tools.
func bestTools(toolsList []*state.Tools, vers version.Binary, dev bool) *state.Tools {
	var bestTools *state.Tools
	allowDev := vers.IsDev() || dev
	for _, t := range toolsList {
		if t.Major != vers.Major ||
			t.Series != vers.Series ||
			t.Arch != vers.Arch ||
			!allowDev && t.IsDev() ||
			vers.Number.Less(t.Number) {
			continue
		}
		if bestTools == nil || bestTools.Number.Less(t.Number) {
			bestTools = t
		}
	}
	return bestTools
}

const urlFile = "downloaded-url.txt"

// toolsParentDir returns the tools parent directory.
func toolsParentDir() string {
	return path.Join(VarDir, "tools")
}

// UnpackTools reads a set of juju tools in gzipped tar-archive
// format and unpacks them into the appropriate tools directory.
// If a valid tools directory already exists, UnpackTools returns
// without error.
func UnpackTools(tools *state.Tools, r io.Reader) (err error) {
	zr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer zr.Close()

	// Make a temporary directory in the tools directory,
	// first ensuring that the tools directory exists.
	err = os.MkdirAll(toolsParentDir(), 0755)
	if err != nil {
		return err
	}
	dir, err := ioutil.TempDir(toolsParentDir(), "unpacking-")
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
			return fmt.Errorf("bad file type %#c in file %q in tools archive", hdr.Typeflag, hdr.Name)
		}
		name := filepath.Join(dir, hdr.Name)
		if err := writeFile(name, os.FileMode(hdr.Mode&0777), tr); err != nil {
			return fmt.Errorf("tar extract %q failed: %v", name, err)
		}
	}
	err = ioutil.WriteFile(filepath.Join(dir, urlFile), []byte(tools.URL), 0644)
	if err != nil {
		return err
	}

	err = os.Rename(dir, ToolsDir(tools.Binary))
	// If we've failed to rename the directory, it may be because
	// the directory already exists - if ReadTools succeeds, we
	// assume all's ok.
	if err != nil {
		_, err := ReadTools(tools.Binary)
		if err == nil {
			return nil
		}
	}
	return nil
}

func removeAll(dir string) {
	err := os.RemoveAll(dir)
	if err == nil || os.IsNotExist(err) {
		return
	}
	log.Printf("environs: cannot remove %q: %v", dir, err)
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
// and returns a Tools instance describing them.
func ReadTools(vers version.Binary) (*state.Tools, error) {
	dir := ToolsDir(vers)
	urlData, err := ioutil.ReadFile(filepath.Join(dir, urlFile))
	if err != nil {
		return nil, fmt.Errorf("cannot read URL in tools directory: %v", err)
	}
	url := strings.TrimSpace(string(urlData))
	if len(url) == 0 {
		return nil, fmt.Errorf("empty URL in tools directory %q", dir)
	}
	// TODO(rog): do more verification here too, such as checking
	// for the existence of certain files.
	return &state.Tools{
		URL:    url,
		Binary: vers,
	}, nil
}

// ChangeAgentTools atomically replaces the agent-specific symlink
// under the tools directory so it points to the previously unpacked
// version vers. It returns the new tools read.
func ChangeAgentTools(agentName string, vers version.Binary) (*state.Tools, error) {
	tools, err := ReadTools(vers)
	if err != nil {
		return nil, err
	}
	tmpName := AgentToolsDir("tmplink-" + agentName)
	err = os.Symlink(tools.Binary.String(), tmpName)
	if err != nil {
		return nil, fmt.Errorf("cannot create tools symlink: %v", err)
	}
	err = os.Rename(tmpName, AgentToolsDir(agentName))
	if err != nil {
		return nil, fmt.Errorf("cannot update tools symlink: %v", err)
	}
	return tools, nil
}

// ToolsStoragePath returns the slash-separated path that is used to store and
// retrieve the given version of the juju tools in a Storage.
func ToolsStoragePath(vers version.Binary) string {
	return toolPrefix + vers.String() + ".tgz"
}

// ToolsDir returns the slash-separated directory name that is used to
// store binaries for the given version of the juju tools.
func ToolsDir(vers version.Binary) string {
	return path.Join(VarDir, "tools", vers.String())
}

// AgentToolsDir returns the slash-separated directory name that is used
// to store binaries for the tools used by the given agent.
// Conventionally it is a symbolic link to the actual tools directory.
func AgentToolsDir(agentName string) string {
	return path.Join(VarDir, "tools", agentName)
}

// FindTools tries to find a set of tools compatible with the given
// version from the given environment.  The latest version found with a
// number <= vers.Number will be used, unless best is true, in
// which case the latest version with the same major version number
// will be used.
// 
// If no tools are found and there's no other error, a NotFoundError is
// returned.  If there's anything compatible in the environ's Storage,
// it gets precedence over anything in its PublicStorage.
func FindTools(env Environ, vers version.Binary, best bool) (*state.Tools, error) {
	if best {
		// Use a stupidly large minor version number
		// so we'll get the highest compatible version available.
		vers.Minor, vers.Patch = 1<<30, 0
	}
	toolsList, err := ListTools(env, vers.Major)
	if err != nil {
		return nil, err
	}
	log.Printf("findTools got tools %v", toolsList)
	tools := BestTools(toolsList, vers, false)
	if tools == nil {
		return tools, &NotFoundError{fmt.Errorf("no compatible tools found")}
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
func bundleTools(w io.Writer, vers *version.Binary) (version.Binary, error) {
	dir, err := ioutil.TempDir("", "juju-tools")
	if err != nil {
		return version.Binary{}, err
	}
	defer os.RemoveAll(dir)

	cmd := exec.Command("go", "install", "launchpad.net/juju-core/cmd/...")
	cmd.Env = setenv(os.Environ(), "GOBIN="+dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return version.Binary{}, fmt.Errorf("build failed: %v; %s", err, out)
	}
	if vers != nil {
		if err := ioutil.WriteFile(filepath.Join(dir, "FORCE-VERSION"), []byte((*vers).String()), 0666); err != nil {
			return version.Binary{}, err
		}
	}
	cmd = exec.Command(filepath.Join(dir, "jujud"), "version")
	out, err = cmd.CombinedOutput()
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
