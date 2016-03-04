// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"archive/tar"
	"compress/bzip2"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/juju/errors"

	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/state/binarystorage"
	"github.com/juju/juju/version"
)

// guiRouter serves the Juju GUI routes.
// Serving the Juju GUI is done with the following assumptions:
// - at least one Juju GUI archive is present in the GUI storage;
// - the archive is compressed in tar.bz2 format;
// - the archive includes a top directory named "jujugui-{version}" where
//   version is semver (like "2.0.1"). This directory includes another
//   "jujugui" directory where the actual Juju GUI files live;
// - the "jujugui" directory includes a "static" subdirectory with the Juju
//   GUI assets to be served statically;
// - the "jujugui" directory specifically includes a
//   "static/gui/build/app/assets/stack/svg/sprite.css.svg" file, which is
//   required to render the Juju GUI index file;
// - the "jujugui" directory includes a "templates/index.html.go" file which is
//   used to render the Juju GUI index. The template receives at least the
//   following variables in its context: "comboURL", "configURL", "debug"
//   and "spriteContent". It might receive more variables but cannot assume
//   them to be always provided;
// - the "jujugui" directory includes a "templates/config.js.go" file which is
//   used to render the Juju GUI configuration file. The template receives at
//   least the following variables in its context: "base", "host", "socket",
//   "uuid" and "version". It might receive more variables but cannot assume
//   them to be always provided.
type guiRouter struct {
	dataDir string
	ctxt    httpContext
}

func (rt *guiRouter) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	rootDir, err := rt.ensureFiles(req)
	if err != nil {
		sendError(w, err)
		return
	}
	staticDir := filepath.Join(rootDir, "static")
	fs := http.FileServer(http.Dir(staticDir))

	uuid := req.URL.Query().Get(":modeluuid")
	parts := strings.SplitAfterN(req.URL.Path, uuid, 2)
	req.URL.Path = parts[1]

	h := &guiHandler{
		baseURLPath: parts[0],
		rootDir:     rootDir,
		uuid:        uuid,
	}
	mux := http.NewServeMux()
	mux.Handle("/static/", http.StripPrefix("/static/", fs))
	mux.HandleFunc("/config.js", h.serveConfig)
	mux.HandleFunc("/combo", h.serveCombo)
	mux.HandleFunc("/", h.serveIndex)
	mux.ServeHTTP(w, req)
}

// ensureFiles checks that the GUI files are available on disk.
// If they are not, it means this is the first time this Juju GUI version is
// accessed. In this case, retrieve the Juju GUI archive from the storage and
// uncompress it to disk.
func (rt *guiRouter) ensureFiles(req *http.Request) (string, error) {
	// Retrieve the Juju GUI info from the GUI storage.
	st, err := rt.ctxt.stateForRequestUnauthenticated(req)
	if err != nil {
		return "", errors.Annotate(err, "cannot open state")
	}
	storage, err := st.GUIStorage()
	if err != nil {
		return "", errors.Annotate(err, "cannot open GUI storage")
	}
	defer storage.Close()
	vers, hash, err := guiVersionAndHash(storage)
	if err != nil {
		return "", errors.Trace(err)
	}
	logger.Debugf("serving Juju GUI version %s", vers)

	// Check if the Juju GUI archive has been already expanded on disk.
	baseDir := agenttools.SharedGUIDir(rt.dataDir)
	rootDir := filepath.Join(baseDir, hash)
	info, err := os.Stat(rootDir)
	if err == nil {
		if info.IsDir() {
			return rootDir, nil
		}
		return "", errors.Errorf("cannot use Juju GUI root directory %q: not a directory", rootDir)
	}
	if !os.IsNotExist(err) {
		return "", errors.Annotate(err, "cannot stat Juju GUI root directory")
	}

	// Fetch the Juju GUI archive from the GUI storage and expand it.
	_, r, err := storage.Open(vers)
	if err != nil {
		return "", errors.Annotatef(err, "cannot find GUI archive version %q", vers)
	}
	defer r.Close()
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return "", errors.Annotate(err, "cannot create Juju GUI base directory")
	}
	guiDir := "jujugui-" + vers + "/jujugui"
	if err := uncompressGUI(r, guiDir, rootDir); err != nil {
		return "", errors.Annotate(err, "cannot uncompress Juju GUI archive")
	}
	return rootDir, nil
}

// guiVersionAndHash returns the version and the SHA256 hash of the current
// Juju GUI archive.
func guiVersionAndHash(storage binarystorage.Storage) (vers, hash string, err error) {
	// TODO frankban: retrieve current GUI version from somewhere. For now
	// just assume we have a single version in the storage.
	allMeta, err := storage.AllMetadata()
	if err != nil {
		return "", "", errors.Annotate(err, "cannot retrieve GUI metadata")
	}
	if len(allMeta) != 1 {
		return "", "", errors.New("GUI metadata not found")
	}
	return allMeta[0].Version, allMeta[0].SHA256, nil
}

// uncompressGUI uncompresses the tar.bz2 Juju GUI archive provided in r.
// The sourceDir directory included in the tar archive is copied to targetDir.
func uncompressGUI(r io.Reader, sourceDir, targetDir string) error {
	tempDir, err := ioutil.TempDir("", "gui")
	if err != nil {
		return errors.Annotate(err, "cannot create Juju GUI temporary directory")
	}
	defer os.Remove(tempDir)
	tr := tar.NewReader(bzip2.NewReader(r))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.Annotate(err, "cannot parse archive")
		}
		if !strings.HasPrefix(hdr.Name, sourceDir+"/") {
			continue
		}
		path := filepath.Join(tempDir, hdr.Name)
		info := hdr.FileInfo()
		if info.IsDir() {
			if err := os.MkdirAll(path, info.Mode()); err != nil {
				return errors.Annotate(err, "cannot create directory")
			}
			continue
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode())
		if err != nil {
			return errors.Annotate(err, "cannot open file")
		}
		defer f.Close()
		if _, err := io.Copy(f, tr); err != nil {
			return errors.Annotate(err, "cannot copy file content")
		}
	}
	if err := os.Rename(filepath.Join(tempDir, sourceDir), targetDir); err != nil {
		return errors.Annotate(err, "cannot rename Juju GUI root directory")
	}
	return nil
}

// guiHandler serves the Juju GUI.
type guiHandler struct {
	baseURLPath string
	rootDir     string
	uuid        string
}

// serveCombo serves the GUI JavaScript and CSS files, dynamically combined.
func (h *guiHandler) serveCombo(w http.ResponseWriter, req *http.Request) {
	ctype := ""
	parts := strings.Split(req.URL.RawQuery, "&")
	paths := make([]string, 0, len(parts))
	for _, p := range parts {
		fpath, err := getGUIComboPath(h.rootDir, p)
		if err != nil {
			sendError(w, errors.Annotate(err, "cannot combine files"))
			return
		}
		if fpath == "" {
			continue
		}
		paths = append(paths, fpath)
		// Assume the Juju GUI does not mix different content types when
		// combining contents.
		if ctype == "" {
			ctype = mime.TypeByExtension(filepath.Ext(fpath))
		}
	}
	w.Header().Set("Content-Type", ctype)
	for _, fpath := range paths {
		sendGUIComboFile(w, fpath)
	}
}

func getGUIComboPath(rootDir, query string) (string, error) {
	k := strings.SplitN(query, "=", 2)[0]
	fname, err := url.QueryUnescape(k)
	if err != nil {
		return "", errors.NewBadRequest(err, fmt.Sprintf("invalid file name %q", k))
	}
	// Ignore pat injected queries.
	if strings.HasPrefix(fname, ":") {
		return "", nil
	}
	// The Juju GUI references its combined files starting from the
	// "static/gui/build" directory.
	fpath := filepath.Join(rootDir, "static", "gui", "build", fname)
	rel, err := filepath.Rel(rootDir, fpath)
	if err != nil {
		return "", errors.NewBadRequest(err, fmt.Sprintf("invalid file path %q", k))
	}
	if strings.HasPrefix(rel, "..") {
		return "", errors.BadRequestf("forbidden file path %q", k)
	}
	return fpath, nil
}

func sendGUIComboFile(w io.Writer, fpath string) {
	f, err := os.Open(fpath)
	if err != nil {
		logger.Infof("cannot send combo file %q: %s", fpath, err)
		return
	}
	defer f.Close()
	if _, err := io.Copy(w, f); err != nil {
		logger.Infof("cannot copy combo file %q: %s", fpath, err)
		return
	}
	fmt.Fprintf(w, "\n/* %s */\n", filepath.Base(f.Name()))
}

// serveIndex serves the GUI index file.
func (h *guiHandler) serveIndex(w http.ResponseWriter, req *http.Request) {
	spriteFile := filepath.Join(h.rootDir, "static", "gui", "build", "app", "assets", "stack", "svg", "sprite.css.svg")
	spriteContent, err := ioutil.ReadFile(spriteFile)
	if err != nil {
		sendError(w, errors.Annotate(err, "cannot read sprite file"))
		return
	}
	tmpl := filepath.Join(h.rootDir, "templates", "index.html.go")
	renderGUITemplate(w, tmpl, map[string]interface{}{
		"comboURL":  h.baseURLPath + "/combo",
		"configURL": h.baseURLPath + "/config.js",
		// TODO frankban: make it possible to enable debug.
		"debug":         false,
		"spriteContent": string(spriteContent),
	})
}

// serveConfig serves the Juju GUI JavaScript configuration file.
func (h *guiHandler) serveConfig(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", mime.TypeByExtension(".js"))
	tmpl := filepath.Join(h.rootDir, "templates", "config.js.go")
	renderGUITemplate(w, tmpl, map[string]interface{}{
		"base":    h.baseURLPath,
		"host":    req.Host,
		"socket":  "/model/$uuid/api",
		"uuid":    h.uuid,
		"version": version.Current.String(),
	})
}

func renderGUITemplate(w http.ResponseWriter, tmpl string, ctx map[string]interface{}) {
	t, err := template.ParseFiles(tmpl)
	if err != nil {
		sendError(w, errors.Annotate(err, "cannot parse template"))
		return
	}
	if err := t.Execute(w, ctx); err != nil {
		sendError(w, errors.Annotate(err, "cannot render template"))
	}
}
