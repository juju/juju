package environs

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"io/ioutil"
	"launchpad.net/juju-core/downloader"
	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
	"net/http"
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

// ListTools returns all the tools found in the given storage
// that have the given major version.
func ListTools(store StorageReader, majorVersion int) ([]*state.Tools, error) {
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

// PutTools uploads the current version of the juju tools
// executables to the given storage and returns a Tools
// instance describing them.
// TODO find binaries from $PATH when not using a development
// version of juju within a $GOPATH.
func PutTools(storage Storage) (*state.Tools, error) {
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
	return &state.Tools{version.Current, url}, nil
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
func BestTools(toolsList []*state.Tools, vers version.Binary) *state.Tools {
	var bestTools *state.Tools
	for _, t := range toolsList {
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

// UnpackTools unpacks a set of tools in gzipped tar-archive
// format from the given Reader into the given directory.
func UnpackTools(r io.Reader, dir string) error {
	r, err := gzip.NewReader(r)
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

// ToolsPath returns the slash-separated path that is used to store and
// retrieve the given version of the juju tools in a Storage.
func ToolsPath(vers version.Binary) string {
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
// version from the given environment.  If no tools are found and
// there's no other error, a NotFoundError is returned.  If there's
// anything compatible in the environ's Storage, it gets precedence over
// anything in its PublicStorage.
func FindTools(env Environ, vers version.Binary) (*state.Tools, error) {
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

type DownloadStatus struct {
	Tools *state.Tools
	File *os.File
	Err error
}

// Upgrader can handle the download and upgrade of an agent's tools.
type Upgrader struct {
	agentName   string
	current     *state.Tools
	downloading *state.Tools
	downloadDone chan DownloadStatus
}

// NewUpgrader returns a new Upgrader for the agent with the
// given name, running the given tools.
func NewUpgrader(agentName string, currentTools *state.Tools) *Upgrader {
	return &Upgrader{
		current:    currentTools,
		agentName:  agentName,
		downloader: downloader.New(),
	}
}

// DownloadTools requests that the given tools be downloaded.
func (u *Upgrader) DownloadTools(t *state.Tools) {
	switch {
	case t.URL == "" || u.current != nil && *t == *u.current:
		// We don't need to be downloading anything
		// if there's no proposed URL or if the version is
		// the same as we're currently running.
		u.Stop()
		return
	case u.downloading != nil && *t == *u.downloading:
		// Leave the existing download if it's already happening.
		return
	}

	// If the tools directory already exists, we need do nothing.
	existingTools, err := readToolsDir(t)
	if err == nil {
		u.downloadDone = make(chan DownloadStatus, 1)
		u.downloadDone <- DownloadStatus{Tools: t}
		close(u.downloadDone)
		return
	}
	u.downloading = t
	file, err := os.TempFile("", "juju-download-")
	if err != nil {
		u.downloadDone = make(chan DownloadStatus, 1)
		u.downloadDone <- DownloadStatus{Err: err}
		close(u.downloadDone)
		return
	}
	done := make(chan DownloadStatus)
	go func() {
		err := download(file, t.URL)
		if err == nil {
			err = file.Seek(0, 0)
		}
		done <- DownloadStatus{
			Tools: t,
			File: file,
			Err: err,
		}
		close(done)
	}()
	u.downloadDone = done
}

func readToolsDir(t *state.Tools) (*state.Tools, error) {
	url, err := ioutil.ReadFile(filepath.Join(ToolsDir(t.Binary), urlFile))
	if err != nil {
		return err
	}
	return &state.Tools{
		URL: url,
		Binary: t.Binary,
	}
}

func (u *Upgrader) Stop() {
	if u.downloading == nil {
		return
	}
	u.downloading = nil
	// TODO(rog) make downloads interruptible and interrupt it here.
	status := <-u.downloadDone
	u.downloadDone = nil
	cleanTempFile(status.File)
}

// Done returns a channel that receives a value when
// the latest proposed tools have been successfully downloaded.
// It is only valid until the next call to DownloadTools or Stop.
// Upgrade should then be called with the received status
// to complete the upgrade.
func (u *Upgrader) Done() <-chan DownloadStatus {
	return u.downloadDone
}

// Upgrade tries to complete an upgrade by unpacking
// the tools and replacing the agent's tools directory. If the upgrade
// succeeds, Upgrade returns the newly upgraded tools.
// The file, status.File is closed and removed.
func (u *Upgrader) Upgrade(status DownloadStatus) (*state.Tools, error) {
	u.Stop()
	defer cleanTempFile(status.File)
	if status.Err != nil {
		// TODO set download error status on machine?
		return nil, fmt.Errorf("tools download %q failed: %v", status.URL, status.Err)
	}
	if status.File == nil {
		if !toolsDirExists(status.Tools) {
			return nil, fmt.Errorf("no tools archive file provided")
		}
		check dir exists
	} else {
		defer os.Remove(status.File.Name())
		defer status.File.Close()
		unpack(status.File)
	}
	defer os.Remove(status.File.Name())
	defer status.File.Close()

	err := u.replaceSymlink(tools)
	if err != nil {
		return nil, fmt.Errorf("cannot update tools symlink: %v", err)
	}
	// N.B.  there's no point in using path.ToSlash here as we don't
	// have symlinks on Windows anyway.
	err := os.Symlink(ToolsDir(tools.Binary), AgentToolsDir(u.agentName))
	if err != nil {
		return nil, fmt.Errorf("cannot update tools symlink: %v", err)
	}
	return tools, nil
}

func cleanTempFile(f *os.File) {
	if f == nil {
		return
	}
	f.Close()
	if err := os.Remove(f.Name()); err != nil {
		log.Printf("environs: cannot remove temporary download file: %v", err)
	}
}

func download(w io.Writer, url string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad http response %v", resp.Status)
	}
	_, err = io.Copy(w, resp.Body)
	return err
}
