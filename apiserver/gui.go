// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"archive/tar"
	"bytes"
	"compress/bzip2"
	"encoding/json"
	"io"
	"io/ioutil"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/gorilla/handlers"
	"github.com/juju/errors"
	"github.com/juju/version"

	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/apiserver/common/apihttp"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
)

const (
	bzMimeType             = "application/x-tar-bzip2"
	dashboardURLPathPrefix = "/dashboard/"
)

var (
	jsMimeType = mime.TypeByExtension(".js")
	spritePath = filepath.FromSlash("static/gui/build/app/assets/stack/svg/sprite.css.svg")
)

type router struct {
	name      string
	dataDir   string
	ctxt      httpContext
	pattern   string
	sourceDir func(vers string) string
}

// dashboardRouter serves the Juju Dashboard routes.
// Serving the Juju Dashboard is done with the following assumptions:
// - the archive is compressed in tar.bz2 format;
// - the archive includes a file version.json where
//   version is semver (like "2.0.1").
// - there's a "static" subdirectory with the Juju Dashboard assets to be served statically;
// - there's a "index.html" file which is used to render the Juju Dashboard index.
// - there's a "config.js.go" file which is used to render the Juju Dashboard configuration file. The template receives at
//   least the following variables in its context: "baseAppURL", "identityProviderAvailable",. It might receive more
//   variables but cannot assume them to be always provided.
func dashboardEndpoints(pattern, dataDir string, ctxt httpContext) []apihttp.Endpoint {
	r := &router{
		name:    "Dashboard",
		dataDir: dataDir,
		ctxt:    ctxt,
		pattern: pattern,
	}

	dh := &dashboardHandler{
		name:     "Dashboard",
		ctxt:     r.ctxt,
		basePath: dashboardURLPathPrefix,
	}
	r.sourceDir = dh.sourceDir

	var endpoints []apihttp.Endpoint
	add := func(pattern string, h func(http.ResponseWriter, *http.Request)) {
		handler := handlers.CompressHandler(r.ensureFileHandler(dh, h))
		// TODO: We can switch from all methods to specific ones for entries
		// where we only want to support specific request methods. However, our
		// tests currently assert that errors come back as application/json and
		// pat only does "text/plain" responses.
		for _, method := range defaultHTTPMethods {
			endpoints = append(endpoints, apihttp.Endpoint{
				Pattern: pattern,
				Method:  method,
				Handler: handler,
			})
		}
	}
	add("/config.js", dh.serveConfig)
	add("/static/", dh.serveStatic)
	// The index is served when all remaining URLs are requested, so that
	// the single page JavaScript application can properly handles its routes.
	add(pattern, dh.serveIndex)
	return endpoints
}

// ensureFileHandler decorates the given function to ensure the Juju Dashboard files
// are available on disk.
func (r *router) ensureFileHandler(c configureHandler, h func(w http.ResponseWriter, req *http.Request)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		rootDir, hash, err := r.ensureFiles(req)
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
		c.setRootDirAndHash(rootDir, hash)
		h(w, req)
	})
}

// ensureFiles checks that the Dashboard files are available on disk.
// If they are not, it means this is the first time this version is
// accessed. In this case, retrieve the archive from the storage and
// uncompress it to disk. This function returns the current root directory
// and archive hash.
func (r *router) ensureFiles(req *http.Request) (rootDir string, hash string, err error) {
	// Retrieve the Juju Dashboard info from storage.
	st := r.ctxt.srv.shared.statePool.SystemState()
	storage, err := st.GUIStorage()
	if err != nil {
		return "", "", errors.Annotatef(err, "cannot open %s storage", r.name)
	}
	defer storage.Close()
	vers, hash, err := r.dashboardVersionAndHash(st, storage)
	if err != nil {
		return "", "", errors.Trace(err)
	}
	logger.Tracef("serving Juju %s version %s", r.name, vers)

	// Check if the current Juju Dashboard archive has been already expanded on disk.
	baseDir := agenttools.SharedDashboardDir(r.dataDir)
	// Note that we include the hash in the root directory so that when the Dashboard
	// archive changes we can be sure that clients will not use files from
	// mixed versions.
	rootDir = filepath.Join(baseDir, hash)
	info, err := os.Stat(rootDir)
	if err == nil {
		if info.IsDir() {
			return rootDir, hash, nil
		}
		return "", "", errors.Errorf("cannot use Juju %s root directory %q: not a directory", r.name, rootDir)
	}
	if !os.IsNotExist(err) {
		return "", "", errors.Annotatef(err, "cannot stat Juju %s root directory", r.name)
	}

	// Fetch the Juju Dashboard archive from the Dashboard storage and expand it.
	_, rc, err := storage.Open(vers)
	if err != nil {
		return "", "", errors.Annotatef(err, "cannot find %s archive version %q", r.name, vers)
	}
	defer rc.Close()
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return "", "", errors.Annotatef(err, "cannot create Juju %s base directory", r.name)
	}
	if err := r.uncompressDashboard(rc, vers, rootDir); err != nil {
		return "", "", errors.Annotatef(err, "cannot uncompress Juju %s archive", r.name)
	}
	return rootDir, hash, nil
}

// dashboardVersionAndHash returns the version and the SHA256 hash of the current
// Juju Dashboard archive.
func (r *router) dashboardVersionAndHash(st *state.State, storage binarystorage.Storage) (vers, hash string, err error) {
	currentVers, err := st.GUIVersion()
	if errors.IsNotFound(err) {
		return "", "", errors.NotFoundf("Juju %s", r.name)
	}
	if err != nil {
		return "", "", errors.Annotatef(err, "cannot retrieve current %s version", r.name)
	}
	metadata, err := storage.Metadata(currentVers.String())
	if err != nil {
		return "", "", errors.Annotatef(err, "cannot retrieve %s metadata", r.name)
	}
	return metadata.Version, metadata.SHA256, nil
}

// uncompressDashboard uncompresses the tar.bz2 Juju Dashboard archive provided in r.
// The source directory for the specified version included in the tar archive is copied to targetDir.
func (r *router) uncompressDashboard(reader io.Reader, vers, targetDir string) error {
	sourceDir := r.sourceDir(vers)
	tempDir, err := ioutil.TempDir(filepath.Join(targetDir, ".."), "dashboard")
	if err != nil {
		return errors.Annotatef(err, "cannot create Juju %s temporary directory", r.name)
	}
	defer os.Remove(tempDir)
	tr := tar.NewReader(bzip2.NewReader(reader))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.Annotate(err, "cannot parse archive")
		}
		hName := hdr.Name
		if sourceDir != "." && strings.HasPrefix(hName, "./") {
			hName = hName[2:]
		}
		if hName != sourceDir && !strings.HasPrefix(hName, sourceDir+"/") {
			logger.Tracef("skipping unknown dashboard file %q", hdr.Name)
			continue
		}
		path := filepath.Join(tempDir, hdr.Name)
		logger.Tracef("writing file %q", path)
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
	logger.Tracef("renaming %q to %q", filepath.Join(tempDir, sourceDir), targetDir)
	if err := os.Rename(filepath.Join(tempDir, sourceDir), targetDir); err != nil {
		return errors.Annotatef(err, "cannot rename Juju %s root directory", r.name)
	}
	return nil
}

type dashboardHandler struct {
	name     string
	ctxt     httpContext
	basePath string
	rootDir  string
	hash     string
}

type configureHandler interface {
	setRootDirAndHash(rootDir, hash string)
}

func (h *dashboardHandler) setRootDirAndHash(rootDir, hash string) {
	h.rootDir = rootDir
	h.hash = hash
}

func (h *dashboardHandler) sourceDir(vers string) string {
	// The dashboard serves files from the root dir.
	return "."
}

// serveStatic serves the Dashboard static files.
func (h *dashboardHandler) serveStatic(w http.ResponseWriter, req *http.Request) {
	staticDir := filepath.Join(h.rootDir, "static")
	logger.Tracef("serving Juju Dashboard static files from %q", staticDir)
	fs := http.FileServer(http.Dir(staticDir))
	http.StripPrefix("/static/", fs).ServeHTTP(w, req)
}

// serveIndex serves the Dashboard index file.
func (h *dashboardHandler) serveIndex(w http.ResponseWriter, req *http.Request) {
	logger.Tracef("serving Juju Dashboard index")
	indexFile := filepath.Join(h.rootDir, "index.html")

	b, err := ioutil.ReadFile(indexFile)
	if err != nil {
		writeError(w, errors.Annotate(err, "cannot read index file"))
		return
	}
	if _, err := w.Write(b); err != nil {
		writeError(w, errors.Annotate(err, "cannot write index file"))
	}
}

// serveConfig serves the Juju Dashboard JavaScript configuration file.
func (h *dashboardHandler) serveConfig(w http.ResponseWriter, req *http.Request) {
	logger.Tracef("serving Juju Dashboard configuration")
	st, err := h.ctxt.stateForRequestUnauthenticated(req)
	if err != nil {
		writeError(w, errors.Annotate(err, "cannot open state"))
		return
	}
	ctrl, err := st.ControllerConfig()
	if err != nil {
		writeError(w, errors.Annotate(err, "cannot open controller config"))
		return
	}
	w.Header().Set("Content-Type", jsMimeType)
	// These query parameters may be set by the index handler.
	tmpl := filepath.Join(h.rootDir, "config.js.go")
	if err := renderDashboardTemplate(w, tmpl, map[string]interface{}{
		"baseAppURL":                dashboardURLPathPrefix,
		"identityProviderAvailable": ctrl.IdentityURL() != "",
		"isJuju":                    true,
	}); err != nil {
		writeError(w, err)
	}
}

func writeError(w http.ResponseWriter, err error) {
	if err2 := sendError(w, err); err2 != nil {
		logger.Errorf("%v", errors.Annotatef(err2, "dashboard handler: cannot send %q error to client", err))
	}
}

func renderDashboardTemplate(w http.ResponseWriter, tmpl string, ctx map[string]interface{}) error {
	// TODO frankban: cache parsed template.
	t, err := template.ParseFiles(tmpl)
	if err != nil {
		return errors.Annotate(err, "cannot parse template")
	}
	return errors.Annotate(t.Execute(w, ctx), "cannot render template")
}

// dashboardArchiveHandler serves the Juju Dashboard archive endpoints, used for uploading
// and retrieving information about Dashboard archives.
type dashboardArchiveHandler struct {
	ctxt httpContext
}

// ServeHTTP implements http.Handler.
func (h *dashboardArchiveHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
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

// handleGet returns information on Juju Dashboard archives in the controller.
func (h *dashboardArchiveHandler) handleGet(w http.ResponseWriter, req *http.Request) error {
	// Open the Dashboard archive storage.
	st, err := h.ctxt.stateForRequestUnauthenticated(req)
	if err != nil {
		return errors.Annotate(err, "cannot open state")
	}
	defer st.Release()
	storage, err := st.GUIStorage()
	if err != nil {
		return errors.Annotate(err, "cannot open Dashboard storage")
	}
	defer storage.Close()

	// Retrieve metadata information.
	allMeta, err := storage.AllMetadata()
	if err != nil {
		return errors.Annotate(err, "cannot retrieve Dashboard metadata")
	}

	// Prepare and send the response.
	var currentVersion string
	vers, err := st.GUIVersion()
	if err == nil {
		currentVersion = vers.String()
	} else if !errors.IsNotFound(err) {
		return errors.Annotate(err, "cannot retrieve current Dashboard version")
	}
	versions := make([]params.DashboardArchiveVersion, len(allMeta))
	for i, m := range allMeta {
		vers, err := version.Parse(m.Version)
		if err != nil {
			return errors.Annotate(err, "cannot parse Dashboard version")
		}
		versions[i] = params.DashboardArchiveVersion{
			Version: vers,
			SHA256:  m.SHA256,
			Current: m.Version == currentVersion,
		}
	}
	return errors.Trace(sendStatusAndJSON(w, http.StatusOK, params.DashboardArchiveResponse{
		Versions: versions,
	}))
}

// handlePost is used to upload new Juju Dashboard archives to the controller.
func (h *dashboardArchiveHandler) handlePost(w http.ResponseWriter, req *http.Request) error {
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

	// Open the Dashboard archive storage.
	st, err := h.ctxt.stateForRequestAuthenticatedUser(req)
	if err != nil {
		return errors.Annotate(err, "cannot open state")
	}
	defer st.Release()
	storage, err := st.GUIStorage()
	if err != nil {
		return errors.Annotate(err, "cannot open Dashboard storage")
	}
	defer storage.Close()

	// Read and validate the archive data.
	data, hash, err := readAndHash(req.Body)
	if err != nil {
		return errors.Trace(err)
	}
	size := int64(len(data))
	if size != req.ContentLength {
		return errors.BadRequestf("archive does not match provided content length")
	}
	if hash != hashParam {
		return errors.BadRequestf("archive does not match provided hash")
	}

	// Add the archive to the Dashboard storage.
	metadata := binarystorage.Metadata{
		Version: vers.String(),
		Size:    size,
		SHA256:  hash,
	}
	if err := storage.Add(bytes.NewReader(data), metadata); err != nil {
		return errors.Annotate(err, "cannot add Dashboard archive to storage")
	}

	// Prepare and return the response.
	resp := params.DashboardArchiveVersion{
		Version: vers,
		SHA256:  hash,
	}
	if currentVers, err := st.GUIVersion(); err == nil {
		if currentVers == vers {
			resp.Current = true
		}
	} else if !errors.IsNotFound(err) {
		return errors.Annotate(err, "cannot retrieve current Dashboard version")

	}
	return errors.Trace(sendStatusAndJSON(w, http.StatusOK, resp))
}

// dashboardVersionHandler is used to select the Juju Dashboard version served by the
// controller. The specified version must be available in the controller.
type dashboardVersionHandler struct {
	ctxt httpContext
}

// ServeHTTP implements http.Handler.
func (h *dashboardVersionHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
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

// handlePut is used to switch to a specific Juju Dashboard version.
func (h *dashboardVersionHandler) handlePut(w http.ResponseWriter, req *http.Request) error {
	// Validate the request.
	if ctype := req.Header.Get("Content-Type"); ctype != params.ContentTypeJSON {
		return errors.BadRequestf("invalid content type %q: expected %q", ctype, params.ContentTypeJSON)
	}

	// Authenticate the request and retrieve the Juju state.
	st, err := h.ctxt.stateForRequestAuthenticatedUser(req)
	if err != nil {
		return errors.Annotate(err, "cannot open state")
	}
	defer st.Release()

	var selected params.DashboardVersionRequest
	decoder := json.NewDecoder(req.Body)
	if err := decoder.Decode(&selected); err != nil {
		return errors.NewBadRequest(err, "invalid request body")
	}

	// Switch to the provided Dashboard version.
	if err = st.GUISetVersion(selected.Version); err != nil {
		return errors.Trace(err)
	}
	return nil
}
