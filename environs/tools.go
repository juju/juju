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
	"path/filepath"
	"strings"
)

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
	log.Debugf("listing tools in dir: %s", dir)
	names, err := store.List(dir)
	if err != nil {
		return nil, err
	}
	var toolsList []*state.Tools
	for _, name := range names {
		log.Debugf("looking at tools file %s", name)
		if !strings.HasPrefix(name, toolPrefix) || !strings.HasSuffix(name, ".tgz") {
			log.Warningf("environs: unexpected tools file found %q", name)
			continue
		}
		vers := name[len(toolPrefix) : len(name)-len(".tgz")]
		var t state.Tools
		t.Binary, err = version.ParseBinary(vers)
		if err != nil {
			log.Warningf("environs: failed to parse %q: %v", vers, err)
			continue
		}
		if t.Major != majorVersion {
			log.Warningf("environs: tool %q found in wrong directory %q", name, dir)
			continue
		}
		t.URL, err = store.URL(name)
		log.Debugf("tools URL is %s", t.URL)
		if err != nil {
			log.Warningf("environs: cannot get URL for %q: %v", name, err)
			continue
		}
		toolsList = append(toolsList, &t)
	}
	return toolsList, nil
}

// PutTools builds the current version of the juju tools, uploads them
// to the given storage, and returns a Tools instance describing them.
// If forceVersion is not nil, the uploaded tools bundle will report
// the given version number.
func PutTools(storage Storage, forceVersion *version.Number) (*state.Tools, error) {
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
	toolsVersion, err := bundleTools(f, forceVersion)
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
	p := ToolsStoragePath(toolsVersion)
	log.Infof("environs: putting tools %v (%dkB)", p, (fi.Size()+512)/1024)
	err = storage.Put(p, f, fi.Size())
	if err != nil {
		return nil, err
	}
	url, err := storage.URL(p)
	if err != nil {
		return nil, err
	}
	return &state.Tools{toolsVersion, url}, nil
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
// compatible with the given version, using flags
// to determine possible candidates.
// It returns nil if no such tools are found.
func BestTools(list *ToolsList, vers version.Binary, flags ToolsSearchFlags) *state.Tools {
	if flags&CompatVersion == 0 {
		panic("CompatVersion not implemented")
	}
	if tools := bestTools(list.Private, vers, flags); tools != nil {
		return tools
	}
	return bestTools(list.Public, vers, flags)
}

// bestTools is like BestTools but operates on a single list of tools.
func bestTools(toolsList []*state.Tools, vers version.Binary, flags ToolsSearchFlags) *state.Tools {
	var bestTools *state.Tools
	allowDev := vers.IsDev() || flags&DevVersion != 0
	allowHigher := flags&HighestVersion != 0
	log.Debugf("finding best tools for version: %v", vers)
	for _, t := range toolsList {
		log.Debugf("checking tools %v", t)
		if t.Major != vers.Major ||
			t.Series != vers.Series ||
			t.Arch != vers.Arch ||
			!allowDev && t.IsDev() ||
			!allowHigher && vers.Number.Less(t.Number) {
			continue
		}
		if bestTools == nil || bestTools.Number.Less(t.Number) {
			bestTools = t
		}
	}
	return bestTools
}

// ToolsStoragePath returns the path that is used to store and
// retrieve the given version of the juju tools in a Storage.
func ToolsStoragePath(vers version.Binary) string {
	return toolPrefix + vers.String() + ".tgz"
}

// MongoStoragePath returns the path that is used to
// retrieve the given version of mongodb in a Storage.
func MongoStoragePath(vers version.Binary) string {
	return fmt.Sprintf("tools/mongo-2.2.0-%s-%s.tgz", vers.Series, vers.Arch)
}

// ToolsSearchFlags gives options when searching
// for tools.
type ToolsSearchFlags int

const (
	// HighestVersion indicates that versions above the version being
	// searched for may be included in the search. The default behavior
	// is to search for versions <= the one provided.
	HighestVersion ToolsSearchFlags = 1 << iota

	// DevVersion includes development versions in the search, even
	// when the version to match against isn't a development version.
	DevVersion

	// CompatVersion specifies that the major version number
	// must be the same as specified. At the moment this flag is required.
	CompatVersion
)

// FindTools tries to find a set of tools compatible with the given
// version from the given environment, using flags to determine
// possible candidates.
//
// If no tools are found and there's no other error, a NotFoundError is
// returned.  If there's anything compatible in the environ's Storage,
// it gets precedence over anything in its PublicStorage.
func FindTools(env Environ, vers version.Binary, flags ToolsSearchFlags) (*state.Tools, error) {
	log.Infof("environs: searching for tools compatible with version: %v\n", vers)
	toolsList, err := ListTools(env, vers.Major)
	if err != nil {
		return nil, err
	}
	tools := BestTools(toolsList, vers, flags)
	if tools == nil {
		return tools, &NotFoundError{fmt.Errorf("no compatible tools found")}
	}
	return tools, nil
}

// MongoURL figures out from where to retrieve a copy of MongoDB compatible with
// the given version from the given environment. The search locations are (in order):
// - the environment specific storage
// - the public storage
// - a "well known" EC2 bucket
func MongoURL(env Environ, vers version.Binary) string {
	url, err := findMongo(env.Storage(), vers)
	if err == nil {
		return url
	}
	url, err = findMongo(env.PublicStorage(), vers)
	if err == nil {
		return url
	}
	url = fmt.Sprintf("http://juju-dist.s3.amazonaws.com/tools/mongo-2.2.0-%s-%s.tgz", vers.Series, vers.Arch)
	return url
}

// Return the URL of a compatible MongoDB (if it exists) from the storage,
// for the given series and architecture (in vers).
func findMongo(store StorageReader, vers version.Binary) (string, error) {
	path := MongoStoragePath(vers)
	names, err := store.List(path)
	if err != nil {
		return "", err
	}
	if len(names) != 1 {
		return "", &NotFoundError{fmt.Errorf("%s not found", path)}
	}
	url, err := store.URL(names[0])
	if err != nil {
		return "", err
	}
	return url, nil
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
