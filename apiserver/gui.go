// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"archive/tar"
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

	"github.com/gorilla/handlers"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/version/v2"

	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/apiserver/common/apihttp"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/binarystorage"
	jujuversion "github.com/juju/juju/version"
)

const (
	bzMimeType       = "application/x-tar-bzip2"
	guiURLPathPrefix = "/gui/"

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

// guiEndpoints serves the Juju GUI routes.
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
//   "controllerSocket", "staticURL" and "version". It might receive more
//   variables but cannot assume them to be always provided.
func guiEndpoints(pattern, dataDir string, ctxt httpContext) []apihttp.Endpoint {
	r := &router{
		name:    "GUI",
		dataDir: dataDir,
		ctxt:    ctxt,
		pattern: pattern,
	}

	gh := &legacyGUIHandler{
		guiHandler{
			name:     "GUI",
			ctxt:     r.ctxt,
			basePath: guiURLPathPrefix,
		},
	}
	r.sourceDir = gh.sourceDir

	var endpoints []apihttp.Endpoint
	add := func(pattern string, h func(http.ResponseWriter, *http.Request)) {
		handler := handlers.CompressHandler(r.ensureFileHandler(gh, h))
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
	hashedPattern := pattern + ":hash"
	add(hashedPattern+"/config.js", gh.serveConfig)
	add(hashedPattern+"/combo", gh.serveCombo)
	add(hashedPattern+"/static/", gh.serveStatic)
	// The index is served when all remaining URLs are requested, so that
	// the single page JavaScript application can properly handles its routes.
	add(pattern, gh.serveIndex)
	return endpoints
}

// dashboardRouter serves the Juju Dashboard routes.
// Serving the Juju Dashboard is done with the following assumptions:
// - the archive is compressed in tar.bz2 format;
// - the archive includes a file version.json where
//   version is semver (like "2.0.1").
// - there's a "static" subdirectory with the Juju GUI assets to be served statically;
// - there's a "index.html" file which is used to render the Juju GUI index.
// - there's a "config.js.go" file which is used to render the Juju GUI configuration file. The template receives at
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
		guiHandler{
			name:     "Dashboard",
			ctxt:     r.ctxt,
			basePath: dashboardURLPathPrefix,
		},
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

// ensureFileHandler decorates the given function to ensure the Juju GUI files
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

// ensureFiles checks that the GUI/Dashboard files are available on disk.
// If they are not, it means this is the first time this version is
// accessed. In this case, retrieve the archive from the storage and
// uncompress it to disk. This function returns the current root directory
// and archive hash.
func (r *router) ensureFiles(req *http.Request) (rootDir string, hash string, err error) {
	// Retrieve the Juju GUI info from the GUI storage.
	st := r.ctxt.srv.shared.statePool.SystemState()
	storage, err := st.GUIStorage()
	if err != nil {
		return "", "", errors.Annotatef(err, "cannot open %s storage", r.name)
	}
	defer storage.Close()
	vers, hash, err := r.guiVersionAndHash(st, storage)
	if err != nil {
		return "", "", errors.Trace(err)
	}
	logger.Tracef("serving Juju %s version %s", r.name, vers)

	// Check if the current Juju GUI/Dashboard archive has been already expanded on disk.
	baseDir := agenttools.SharedGUIDir(r.dataDir)
	// Note that we include the hash in the root directory so that when the GUI
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

	// Fetch the Juju Dashboard archive from the GUI storage and expand it.
	_, rc, err := storage.Open(vers)
	if err != nil {
		return "", "", errors.Annotatef(err, "cannot find %s archive version %q", r.name, vers)
	}
	defer rc.Close()
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return "", "", errors.Annotatef(err, "cannot create Juju %s base directory", r.name)
	}
	if err := r.uncompressGUI(rc, vers, rootDir); err != nil {
		return "", "", errors.Annotatef(err, "cannot uncompress Juju %s archive", r.name)
	}
	return rootDir, hash, nil
}

// guiVersionAndHash returns the version and the SHA256 hash of the current
// Juju GUI archive.
func (r *router) guiVersionAndHash(st *state.State, storage binarystorage.Storage) (vers, hash string, err error) {
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

// uncompressGUI uncompresses the tar.bz2 Juju GUI archive provided in r.
// The source directory for the specified version included in the tar archive is copied to targetDir.
func (r *router) uncompressGUI(reader io.Reader, vers, targetDir string) error {
	sourceDir := r.sourceDir(vers)
	tempDir, err := ioutil.TempDir(filepath.Join(targetDir, ".."), "gui")
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

// guiHandler serves the Juju GUI.
type guiHandler struct {
	name     string
	ctxt     httpContext
	basePath string
	rootDir  string
	hash     string
}

type legacyGUIHandler struct {
	guiHandler
}

type dashboardHandler struct {
	guiHandler
}

type configureHandler interface {
	setRootDirAndHash(rootDir, hash string)
}

func (h *guiHandler) setRootDirAndHash(rootDir, hash string) {
	h.rootDir = rootDir
	h.hash = hash
}

func (h *legacyGUIHandler) sourceDir(vers string) string {
	return "jujugui-" + vers + "/jujugui"
}

// serveStatic serves the GUI static files.
func (h *legacyGUIHandler) serveStatic(w http.ResponseWriter, req *http.Request) {
	staticDir := filepath.Join(h.rootDir, "static")
	logger.Tracef("serving Juju Dashboard static files from %q", staticDir)
	fs := http.FileServer(http.Dir(staticDir))
	http.StripPrefix(h.hashedPath("static/"), fs).ServeHTTP(w, req)
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

// serveCombo serves the GUI JavaScript and CSS files, dynamically combined.
func (h *legacyGUIHandler) serveCombo(w http.ResponseWriter, req *http.Request) {
	logger.Tracef("serving Juju GUI combined files")
	ctype := ""
	// The combo query is like /combo/?path/to/file1&path/to/file2 ...
	parts := strings.Split(req.URL.RawQuery, "&")
	paths := make([]string, 0, len(parts))
	for _, p := range parts {
		fpath, err := h.getGUIComboPath(p)
		if err != nil {
			writeError(w, errors.Annotate(err, "cannot combine files"))
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
		h.sendGUIComboFile(w, fpath)
	}
}

func (h *legacyGUIHandler) getGUIComboPath(query string) (string, error) {
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
	return filepath.Join(h.rootDir, "static", "gui", "build", fname), nil
}

func (h *legacyGUIHandler) sendGUIComboFile(w io.Writer, fpath string) {
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
	fmt.Fprintf(w, "\n/* %s */\n", filepath.Base(fpath))
}

// serveIndex serves the GUI index file.
func (h *legacyGUIHandler) serveIndex(w http.ResponseWriter, req *http.Request) {
	logger.Tracef("serving Juju GUI index")
	spriteFile := filepath.Join(h.rootDir, spritePath)
	spriteContent, err := ioutil.ReadFile(spriteFile)
	if err != nil {
		writeError(w, errors.Annotate(err, "cannot read sprite file"))
		return
	}
	tmpl := filepath.Join(h.rootDir, "templates", "index.html.go")
	if err := renderGUITemplate(w, tmpl, map[string]interface{}{
		// staticURL holds the root of the static hierarchy, hence why the
		// empty string is used here.
		"staticURL": h.hashedPath(""),
		"comboURL":  h.hashedPath("combo"),
		"configURL": h.hashedPath(getConfigPath(req.URL.Path, h.ctxt)),
		// TODO frankban: make it possible to enable debug.
		"debug":         false,
		"spriteContent": string(spriteContent),
	}); err != nil {
		writeError(w, err)
	}
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

// getConfigPath returns the appropriate GUI config path for the given request
// path.
func getConfigPath(path string, ctxt httpContext) string {
	configPath := "config.js"
	// Handle requests from old clients, in which the model UUID is a fragment
	// in the request path. If this is the case, we also need to include the
	// UUID in the GUI base URL.
	uuid := uuidFromPath(path)
	if uuid != "" {
		return fmt.Sprintf("%[1]s?model-uuid=%[2]s&base-postfix=%[2]s/", configPath, uuid)
	}
	st := ctxt.srv.shared.statePool.SystemState()
	if isNewGUI(st) {
		// This is the proper case in which a new GUI is being served from a
		// new URL. No query must be included in the config path.
		return configPath
	}
	// Possibly handle requests to the new "/u/{user}/{model}" path, but
	// made from an old version of the GUI, which didn't connect to the
	// model based on the path.
	uuid, user, model := modelInfoFromPath(path, st, ctxt.srv.shared.statePool)
	if uuid != "" {
		return fmt.Sprintf("%s?model-uuid=%s&base-postfix=u/%s/%s/", configPath, uuid, user, model)
	}
	return configPath
}

// uuidFromPath checks whether the given path includes a fragment with a
// valid model UUID. An empty string is returned if the model is not found.
func uuidFromPath(path string) string {
	path = strings.TrimPrefix(path, guiURLPathPrefix)
	uuid := strings.SplitN(path, "/", 2)[0]
	if names.IsValidModel(uuid) {
		return uuid
	}
	return ""
}

// modelInfoFromPath checks whether the given path includes "/u/{user}/{model}""
// fragments identifying a model, in which case its UUID, user and model name
// are returned. Empty strings are returned if the model is not found.
func modelInfoFromPath(path string, st *state.State, pool *state.StatePool) (uuid, user, modelName string) {
	path = strings.TrimPrefix(path, guiURLPathPrefix)
	parts := strings.SplitN(path, "/", 4)
	if len(parts) < 3 || parts[0] != "u" || !names.IsValidUserName(parts[1]) || !names.IsValidModelName(parts[2]) {
		return "", "", ""
	}
	user, modelName = parts[1], parts[2]
	modelUUIDs, err := st.ModelUUIDsForUser(names.NewUserTag(user))
	if err != nil {
		return "", "", ""
	}
	for _, modelUUID := range modelUUIDs {
		model, ph, err := pool.GetModel(modelUUID)
		if err != nil {
			return "", "", ""
		}
		defer ph.Release()
		if model.Name() == modelName {
			return modelUUID, user, modelName
		}
	}
	return "", "", ""
}

// isNewGUI reports whether the version of the current GUI is >= 2.3.0.
func isNewGUI(st *state.State) bool {
	vers, err := st.GUIVersion()
	if err != nil {
		logger.Warningf("cannot retrieve GUI version: %v", err)
		// Assume a recent version of the GUI is being served.
		return true
	}
	return vers.Major > 2 || (vers.Major == 2 && vers.Minor >= 3)
}

// serveConfig serves the Juju GUI JavaScript configuration file.
func (h *legacyGUIHandler) serveConfig(w http.ResponseWriter, req *http.Request) {
	logger.Tracef("serving Juju GUI configuration")
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
	base := h.basePath
	// These query parameters may be set by the index handler.
	uuid := req.URL.Query().Get("model-uuid")
	if uuid != "" {
		base += req.URL.Query().Get("base-postfix")
	}
	tmpl := filepath.Join(h.rootDir, "templates", "config.js.go")
	if err := renderGUITemplate(w, tmpl, map[string]interface{}{
		"base":             base,
		"bakeryEnabled":    ctrl.IdentityURL() != "",
		"controllerSocket": "/api",
		"charmstoreURL":    ctrl.CharmStoreURL(),
		"host":             req.Host,
		"socket":           "/model/$uuid/api",
		// staticURL holds the root of the static hierarchy, hence why the
		// empty string is used here.
		"staticURL": h.hashedPath(""),
		"uuid":      uuid,
		"version":   jujuversion.Current.String(),
	}); err != nil {
		writeError(w, err)
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
	if err := renderGUITemplate(w, tmpl, map[string]interface{}{
		"baseAppURL":                dashboardURLPathPrefix,
		"identityProviderAvailable": ctrl.IdentityURL() != "",
		"isJuju":                    true,
	}); err != nil {
		writeError(w, err)
	}
}

func writeError(w http.ResponseWriter, err error) {
	if err2 := sendError(w, err); err2 != nil {
		logger.Errorf("%v", errors.Annotatef(err2, "gui handler: cannot send %q error to client", err))
	}
}

// hashedPath returns the full path (including the GUI archive hash) to the
// given path, that must not start with a slash.
func (h *legacyGUIHandler) hashedPath(p string) string {
	return path.Join(h.basePath, h.hash, p)
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
	defer st.Release()
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
	defer st.Release()
	storage, err := st.GUIStorage()
	if err != nil {
		return errors.Annotate(err, "cannot open GUI storage")
	}
	defer storage.Close()

	// Read and validate the archive data.
	data, hash, size, err := tmpCacheAndHash(req.Body)
	if err != nil {
		return errors.Trace(err)
	}
	defer data.Close()

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
	if err := storage.Add(data, metadata); err != nil {
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
	defer st.Release()

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
