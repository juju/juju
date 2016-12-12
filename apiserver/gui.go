// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"archive/tar"
	"bytes"
	"compress/bzip2"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/juju/errors"
	"github.com/juju/version"

	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/apihttp"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
	jujuversion "github.com/juju/juju/version"
)

const (
	bzMimeType = "application/x-tar-bzip2"
)

var (
	jsMimeType = mime.TypeByExtension(".js")
	spritePath = filepath.FromSlash("static/gui/build/app/assets/stack/svg/sprite.css.svg")
)

// guiRouter serves the Juju GUI routes.
// Serving the Juju GUI is done with the following assumptions:
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
//   following variables in its context: "staticURL", comboURL", "configURL",
//   "debug" and "spriteContent". It might receive more variables but cannot
//   assume them to be always provided;
// - the "jujugui" directory includes a "templates/config.js.go" file which is
//   used to render the Juju GUI configuration file. The template receives at
//   least the following variables in its context: "base", "host", "socket",
//   "controllerSocket", "staticURL", "uuid" and "version". It might receive
//   more variables but cannot assume them to be always provided.
type guiRouter struct {
	dataDir string
	ctxt    httpContext
	pattern string
}

func guiEndpoints(pattern, dataDir string, ctxt httpContext) []apihttp.Endpoint {
	gr := &guiRouter{
		dataDir: dataDir,
		ctxt:    ctxt,
		pattern: pattern,
	}
	var endpoints []apihttp.Endpoint
	add := func(pattern string, h func(*guiHandler, http.ResponseWriter, *http.Request)) {
		handler := gr.ensureFileHandler(h)
		// TODO: We can switch from all methods to specific ones for entries
		// where we only want to support specific request methods. However, our
		// tests currently assert that errors come back as application/json and
		// pat only does "text/plain" responses.
		for _, method := range common.DefaultHTTPMethods {
			endpoints = append(endpoints, apihttp.Endpoint{
				Pattern: pattern,
				Method:  method,
				Handler: handler,
			})
		}
	}
	hashedPattern := pattern + ":hash"
	add(hashedPattern+"/config.js", (*guiHandler).serveConfig)
	add(hashedPattern+"/combo", (*guiHandler).serveCombo)
	add(hashedPattern+"/static/", (*guiHandler).serveStatic)
	// The index is served when all remaining URLs are requested, so that
	// the single page JavaScript application can properly handles its routes.
	add(pattern, (*guiHandler).serveIndex)
	return endpoints
}

// ensureFileHandler decorates the given function to ensure the Juju GUI files
// are available on disk.
func (gr *guiRouter) ensureFileHandler(h func(gh *guiHandler, w http.ResponseWriter, req *http.Request)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		rootDir, hash, err := gr.ensureFiles(req)
		if err != nil {
			// Note that ensureFiles also checks that the model UUID is valid.
			if err := sendError(w, err); err != nil {
				logger.Errorf("%v", err)
			}
			return
		}
		qhash := req.URL.Query().Get(":hash")
		if qhash != "" && qhash != hash {
			if err := sendError(w, errors.NotFoundf("resource with %q hash", qhash)); err != nil {
				logger.Errorf("%v", err)
			}
			return
		}
		uuid := req.URL.Query().Get(":modeluuid")
		gh := &guiHandler{
			rootDir:     rootDir,
			baseURLPath: strings.Replace(gr.pattern, ":modeluuid", uuid, -1),
			hash:        hash,
			uuid:        uuid,
		}
		h(gh, w, req)
	})
}

// ensureFiles checks that the GUI files are available on disk.
// If they are not, it means this is the first time this Juju GUI version is
// accessed. In this case, retrieve the Juju GUI archive from the storage and
// uncompress it to disk. This function returns the current GUI root directory
// and archive hash.
func (gr *guiRouter) ensureFiles(req *http.Request) (rootDir string, hash string, err error) {
	// Retrieve the Juju GUI info from the GUI storage.
	st, err := gr.ctxt.stateForRequestUnauthenticated(req)
	if err != nil {
		return "", "", errors.Annotate(err, "cannot open state")
	}
	defer gr.ctxt.release(st)
	storage, err := st.GUIStorage()
	if err != nil {
		return "", "", errors.Annotate(err, "cannot open GUI storage")
	}
	defer storage.Close()
	vers, hash, err := guiVersionAndHash(st, storage)
	if err != nil {
		return "", "", errors.Trace(err)
	}
	logger.Debugf("serving Juju GUI version %s", vers)

	// Check if the current Juju GUI archive has been already expanded on disk.
	baseDir := agenttools.SharedGUIDir(gr.dataDir)
	// Note that we include the hash in the root directory so that when the GUI
	// archive changes we can be sure that clients will not use files from
	// mixed versions.
	rootDir = filepath.Join(baseDir, hash)
	info, err := os.Stat(rootDir)
	if err == nil {
		if info.IsDir() {
			return rootDir, hash, nil
		}
		return "", "", errors.Errorf("cannot use Juju GUI root directory %q: not a directory", rootDir)
	}
	if !os.IsNotExist(err) {
		return "", "", errors.Annotate(err, "cannot stat Juju GUI root directory")
	}

	// Fetch the Juju GUI archive from the GUI storage and expand it.
	_, r, err := storage.Open(vers)
	if err != nil {
		return "", "", errors.Annotatef(err, "cannot find GUI archive version %q", vers)
	}
	defer r.Close()
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return "", "", errors.Annotate(err, "cannot create Juju GUI base directory")
	}
	guiDir := "jujugui-" + vers + "/jujugui"
	if err := uncompressGUI(r, guiDir, rootDir); err != nil {
		return "", "", errors.Annotate(err, "cannot uncompress Juju GUI archive")
	}
	return rootDir, hash, nil
}

// guiVersionAndHash returns the version and the SHA256 hash of the current
// Juju GUI archive.
func guiVersionAndHash(st *state.State, storage binarystorage.Storage) (vers, hash string, err error) {
	currentVers, err := st.GUIVersion()
	if errors.IsNotFound(err) {
		return "", "", errors.NotFoundf("Juju GUI")
	}
	if err != nil {
		return "", "", errors.Annotate(err, "cannot retrieve current GUI version")
	}
	metadata, err := storage.Metadata(currentVers.String())
	if err != nil {
		return "", "", errors.Annotate(err, "cannot retrieve GUI metadata")
	}
	return metadata.Version, metadata.SHA256, nil
}

// uncompressGUI uncompresses the tar.bz2 Juju GUI archive provided in r.
// The sourceDir directory included in the tar archive is copied to targetDir.
func uncompressGUI(r io.Reader, sourceDir, targetDir string) error {
	tempDir, err := ioutil.TempDir(filepath.Join(targetDir, ".."), "gui")
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
		if hdr.Name != sourceDir && !strings.HasPrefix(hdr.Name, sourceDir+"/") {
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
	hash        string
	uuid        string
}

// serveStatic serves the GUI static files.
func (h *guiHandler) serveStatic(w http.ResponseWriter, req *http.Request) {
	staticDir := filepath.Join(h.rootDir, "static")
	fs := http.FileServer(http.Dir(staticDir))
	http.StripPrefix(h.hashedPath("static/"), fs).ServeHTTP(w, req)
}

// serveCombo serves the GUI JavaScript and CSS files, dynamically combined.
func (h *guiHandler) serveCombo(w http.ResponseWriter, req *http.Request) {
	ctype := ""
	// The combo query is like /combo/?path/to/file1&path/to/file2 ...
	parts := strings.Split(req.URL.RawQuery, "&")
	paths := make([]string, 0, len(parts))
	for _, p := range parts {
		fpath, err := getGUIComboPath(h.rootDir, p)
		if err != nil {
			if err := sendError(w, errors.Annotate(err, "cannot combine files")); err != nil {
				logger.Errorf("%v", err)
			}
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
	fname = filepath.Clean(fname)
	if fname == ".." || strings.HasPrefix(fname, "../") {
		return "", errors.BadRequestf("forbidden file path %q", k)
	}
	return filepath.Join(rootDir, "static", "gui", "build", fname), nil
}

func sendGUIComboFile(w io.Writer, fpath string) {
	f, err := os.Open(fpath)
	if err != nil {
		logger.Infof("cannot send combo file %q: %s", fpath, err)
		return
	}
	defer f.Close()
	if _, err := io.Copy(w, f); err != nil {
		return
	}
	fmt.Fprintf(w, "\n/* %s */\n", filepath.Base(fpath))
}

// serveIndex serves the GUI index file.
func (h *guiHandler) serveIndex(w http.ResponseWriter, req *http.Request) {
	spriteFile := filepath.Join(h.rootDir, spritePath)
	spriteContent, err := ioutil.ReadFile(spriteFile)
	if err != nil {
		if err := sendError(w, errors.Annotate(err, "cannot read sprite file")); err != nil {
			logger.Errorf("%v", err)
		}
		return
	}
	tmpl := filepath.Join(h.rootDir, "templates", "index.html.go")
	if err := renderGUITemplate(w, tmpl, map[string]interface{}{
		// staticURL holds the root of the static hierarchy, hence why the
		// empty string is used here.
		"staticURL": h.hashedPath(""),
		"comboURL":  h.hashedPath("combo"),
		"configURL": h.hashedPath("config.js"),
		// TODO frankban: make it possible to enable debug.
		"debug":         false,
		"spriteContent": string(spriteContent),
	}); err != nil {
		if err := sendError(w, err); err != nil {
			logger.Errorf("%v", errors.Annotate(err, "cannot send error to client from rendering GUI template"))
		}
	}
}

// serveConfig serves the Juju GUI JavaScript configuration file.
func (h *guiHandler) serveConfig(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", jsMimeType)
	tmpl := filepath.Join(h.rootDir, "templates", "config.js.go")
	if err := renderGUITemplate(w, tmpl, map[string]interface{}{
		"base":             h.baseURLPath,
		"host":             req.Host,
		"controllerSocket": "/api",
		"socket":           "/model/$uuid/api",
		// staticURL holds the root of the static hierarchy, hence why the
		// empty string is used here.
		"staticURL": h.hashedPath(""),
		"uuid":      h.uuid,
		"version":   jujuversion.Current.String(),
	}); err != nil {
		if err := sendError(w, err); err != nil {
			logger.Errorf("%v", errors.Annotate(err, "cannot send error to client from rendering GUI template"))
		}
	}
}

// hashedPath returns the gull path (including the GUI archive hash) to the
// given path, that must not start with a slash.
func (h *guiHandler) hashedPath(p string) string {
	return path.Join(h.baseURLPath, h.hash, p)
}

func renderGUITemplate(w http.ResponseWriter, tmpl string, ctx map[string]interface{}) error {
	// TODO frankban: cache parsed template.
	t, err := template.ParseFiles(tmpl)
	if err != nil {
		return errors.Annotate(err, "cannot parse template")
	}
	return errors.Annotate(t.Execute(w, ctx), "cannot render template")
}

// guiArchiveHandler serves the Juju GUI archive endpoints, used for uploading
// and retrieving information about GUI archives.
type guiArchiveHandler struct {
	ctxt httpContext
}

// ServeHTTP implements http.Handler.
func (h *guiArchiveHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var handler func(http.ResponseWriter, *http.Request) error
	switch req.Method {
	case "GET":
		handler = h.handleGet
	case "POST":
		handler = h.handlePost
	default:
		if err := sendError(w, errors.MethodNotAllowedf("unsupported method: %q", req.Method)); err != nil {
			logger.Errorf("%v", err)
		}
		return
	}
	if err := handler(w, req); err != nil {
		if err := sendError(w, errors.Trace(err)); err != nil {
			logger.Errorf("%v", err)
		}
	}
}

// handleGet returns information on Juju GUI archives in the controller.
func (h *guiArchiveHandler) handleGet(w http.ResponseWriter, req *http.Request) error {
	// Open the GUI archive storage.
	st, err := h.ctxt.stateForRequestUnauthenticated(req)
	if err != nil {
		return errors.Annotate(err, "cannot open state")
	}
	defer h.ctxt.release(st)
	storage, err := st.GUIStorage()
	if err != nil {
		return errors.Annotate(err, "cannot open GUI storage")
	}
	defer storage.Close()

	// Retrieve metadata information.
	allMeta, err := storage.AllMetadata()
	if err != nil {
		return errors.Annotate(err, "cannot retrieve GUI metadata")
	}

	// Prepare and send the response.
	var currentVersion string
	vers, err := st.GUIVersion()
	if err == nil {
		currentVersion = vers.String()
	} else if !errors.IsNotFound(err) {
		return errors.Annotate(err, "cannot retrieve current GUI version")
	}
	versions := make([]params.GUIArchiveVersion, len(allMeta))
	for i, m := range allMeta {
		vers, err := version.Parse(m.Version)
		if err != nil {
			return errors.Annotate(err, "cannot parse GUI version")
		}
		versions[i] = params.GUIArchiveVersion{
			Version: vers,
			SHA256:  m.SHA256,
			Current: m.Version == currentVersion,
		}
	}
	return errors.Trace(sendStatusAndJSON(w, http.StatusOK, params.GUIArchiveResponse{
		Versions: versions,
	}))
}

// handlePost is used to upload new Juju GUI archives to the controller.
func (h *guiArchiveHandler) handlePost(w http.ResponseWriter, req *http.Request) error {
	// Validate the request.
	if ctype := req.Header.Get("Content-Type"); ctype != bzMimeType {
		return errors.BadRequestf("invalid content type %q: expected %q", ctype, bzMimeType)
	}
	if err := req.ParseForm(); err != nil {
		return errors.Annotate(err, "cannot parse form")
	}
	versParam := req.Form.Get("version")
	if versParam == "" {
		return errors.BadRequestf("version parameter not provided")
	}
	vers, err := version.Parse(versParam)
	if err != nil {
		return errors.BadRequestf("invalid version parameter %q", versParam)
	}
	hashParam := req.Form.Get("hash")
	if hashParam == "" {
		return errors.BadRequestf("hash parameter not provided")
	}
	if req.ContentLength == -1 {
		return errors.BadRequestf("content length not provided")
	}

	// Open the GUI archive storage.
	st, err := h.ctxt.stateForRequestAuthenticatedUser(req)
	if err != nil {
		return errors.Annotate(err, "cannot open state")
	}
	defer h.ctxt.release(st)
	storage, err := st.GUIStorage()
	if err != nil {
		return errors.Annotate(err, "cannot open GUI storage")
	}
	defer storage.Close()

	// Read and validate the archive data.
	data, hash, err := readAndHash(req.Body)
	size := int64(len(data))
	if size != req.ContentLength {
		return errors.BadRequestf("archive does not match provided content length")
	}
	if hash != hashParam {
		return errors.BadRequestf("archive does not match provided hash")
	}

	// Add the archive to the GUI storage.
	metadata := binarystorage.Metadata{
		Version: vers.String(),
		Size:    size,
		SHA256:  hash,
	}
	if err := storage.Add(bytes.NewReader(data), metadata); err != nil {
		return errors.Annotate(err, "cannot add GUI archive to storage")
	}

	// Prepare and return the response.
	resp := params.GUIArchiveVersion{
		Version: vers,
		SHA256:  hash,
	}
	if currentVers, err := st.GUIVersion(); err == nil {
		if currentVers == vers {
			resp.Current = true
		}
	} else if !errors.IsNotFound(err) {
		return errors.Annotate(err, "cannot retrieve current GUI version")

	}
	return errors.Trace(sendStatusAndJSON(w, http.StatusOK, resp))
}

// guiVersionHandler is used to select the Juju GUI version served by the
// controller. The specified version must be available in the controller.
type guiVersionHandler struct {
	ctxt httpContext
}

// ServeHTTP implements http.Handler.
func (h *guiVersionHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != "PUT" {
		if err := sendError(w, errors.MethodNotAllowedf("unsupported method: %q", req.Method)); err != nil {
			logger.Errorf("%v", err)
		}
		return
	}
	if err := h.handlePut(w, req); err != nil {
		if err := sendError(w, errors.Trace(err)); err != nil {
			logger.Errorf("%v", err)
		}
	}
}

// handlePut is used to switch to a specific Juju GUI version.
func (h *guiVersionHandler) handlePut(w http.ResponseWriter, req *http.Request) error {
	// Validate the request.
	if ctype := req.Header.Get("Content-Type"); ctype != params.ContentTypeJSON {
		return errors.BadRequestf("invalid content type %q: expected %q", ctype, params.ContentTypeJSON)
	}

	// Authenticate the request and retrieve the Juju state.
	st, err := h.ctxt.stateForRequestAuthenticatedUser(req)
	if err != nil {
		return errors.Annotate(err, "cannot open state")
	}
	defer h.ctxt.release(st)

	var selected params.GUIVersionRequest
	decoder := json.NewDecoder(req.Body)
	if err := decoder.Decode(&selected); err != nil {
		return errors.NewBadRequest(err, "invalid request body")
	}

	// Switch to the provided GUI version.
	if err = st.GUISetVersion(selected.Version); err != nil {
		return errors.Trace(err)
	}
	return nil
}
