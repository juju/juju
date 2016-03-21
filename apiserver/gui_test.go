// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/binarystorage"
	jujuversion "github.com/juju/juju/version"
)

const (
	guiConfigPath = "templates/config.js.go"
	guiIndexPath  = "templates/index.html.go"
)

type guiSuite struct {
	authHttpSuite
}

var _ = gc.Suite(&guiSuite{})

// guiURL returns the complete URL where the Juju GUI can be found, including
// the given hash and pathAndquery.
func (s *guiSuite) guiURL(c *gc.C, hash, pathAndquery string) string {
	u := s.baseURL(c)
	path := "/gui/" + s.modelUUID
	if hash != "" {
		path += "/" + hash
	}
	parts := strings.SplitN(pathAndquery, "?", 2)
	u.Path = path + parts[0]
	if len(parts) == 2 {
		u.RawQuery = parts[1]
	}
	return u.String()
}

type guiSetupFunc func(c *gc.C, baseDir string, storage binarystorage.Storage) string

var guiHandlerTests = []struct {
	// about describes the test.
	about string
	// setup is optionally used to set up the test.
	// It receives the Juju GUI base directory and an empty GUI storage.
	// Optionally it can return a GUI archive hash which is used by the test
	// to build the URL path for the HTTP request.
	setup guiSetupFunc
	// pathAndquery holds the optional path and query for the request, for
	// instance "/combo?file". If not provided, the "/" path is used.
	pathAndquery string
	// expectedStatus holds the expected response HTTP status.
	// A 200 OK status is used by default.
	expectedStatus int
	// expectedContentType holds the expected response content type.
	// If expectedError is provided this field is ignored.
	expectedContentType string
	// expectedBody holds the expected response body, only used if
	// expectedError is not provided (see below).
	expectedBody string
	// expectedError holds the expected error message included in the response.
	expectedError string
}{{
	about:          "metadata not found",
	expectedStatus: http.StatusNotFound,
	expectedError:  "Juju GUI not found",
}, {
	about: "GUI directory is a file",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) string {
		err := storage.Add(strings.NewReader(""), binarystorage.Metadata{
			SHA256: "fake-hash",
		})
		c.Assert(err, jc.ErrorIsNil)
		err = os.MkdirAll(baseDir, 0755)
		c.Assert(err, jc.ErrorIsNil)
		rootDir := filepath.Join(baseDir, "fake-hash")
		err = ioutil.WriteFile(rootDir, nil, 0644)
		c.Assert(err, jc.ErrorIsNil)
		return ""
	},
	expectedStatus: http.StatusInternalServerError,
	expectedError:  "cannot use Juju GUI root directory .*",
}, {
	about: "GUI directory is unaccessible",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) string {
		err := storage.Add(strings.NewReader(""), binarystorage.Metadata{
			SHA256: "fake-hash",
		})
		c.Assert(err, jc.ErrorIsNil)
		err = os.MkdirAll(baseDir, 0000)
		c.Assert(err, jc.ErrorIsNil)
		return ""
	},
	expectedStatus: http.StatusInternalServerError,
	expectedError:  "cannot stat Juju GUI root directory: .*",
}, {
	about: "invalid GUI archive",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) string {
		err := storage.Add(strings.NewReader(""), binarystorage.Metadata{
			SHA256: "fake-hash",
		})
		c.Assert(err, jc.ErrorIsNil)
		return ""
	},
	expectedStatus: http.StatusInternalServerError,
	expectedError:  "cannot uncompress Juju GUI archive: cannot parse archive: .*",
}, {
	about: "index: sprite file not found",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) string {
		setupGUIArchive(c, storage, "2.0.42", nil)
		return ""
	},
	expectedStatus: http.StatusInternalServerError,
	expectedError:  "cannot read sprite file: .*",
}, {
	about: "index: template not found",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) string {
		setupGUIArchive(c, storage, "2.0.42", map[string]string{
			apiserver.SpritePath: "",
		})
		return ""
	},
	expectedStatus: http.StatusInternalServerError,
	expectedError:  "cannot parse template: .*: no such file or directory",
}, {
	about: "index: invalid template",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) string {
		setupGUIArchive(c, storage, "2.0.47", map[string]string{
			guiIndexPath:         "{{.BadWolf.47}}",
			apiserver.SpritePath: "",
		})
		return ""
	},
	expectedStatus: http.StatusInternalServerError,
	expectedError:  `cannot parse template: template: index.html.go:1: unexpected ".47" .*`,
}, {
	about: "index: invalid template and context",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) string {
		setupGUIArchive(c, storage, "2.0.47", map[string]string{
			guiIndexPath:         "{{range .debug}}{{end}}",
			apiserver.SpritePath: "",
		})
		return ""
	},
	expectedStatus: http.StatusInternalServerError,
	expectedError:  `cannot render template: template: .*: range can't iterate over .*`,
}, {
	about: "config: template not found",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) string {
		return setupGUIArchive(c, storage, "2.0.42", nil)
	},
	pathAndquery:   "/config.js",
	expectedStatus: http.StatusInternalServerError,
	expectedError:  "cannot parse template: .*: no such file or directory",
}, {
	about: "config: invalid template",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) string {
		return setupGUIArchive(c, storage, "2.0.47", map[string]string{
			guiConfigPath: "{{.BadWolf.47}}",
		})
	},
	pathAndquery:   "/config.js",
	expectedStatus: http.StatusInternalServerError,
	expectedError:  `cannot parse template: template: config.js.go:1: unexpected ".47" .*`,
}, {
	about: "config: invalid hash",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) string {
		setupGUIArchive(c, storage, "2.0.47", nil)
		return "invalid"
	},
	pathAndquery:   "/config.js",
	expectedStatus: http.StatusNotFound,
	expectedError:  `resource with "invalid" hash not found`,
}, {
	about: "combo: invalid file name",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) string {
		return setupGUIArchive(c, storage, "1.0.0", nil)
	},
	pathAndquery:   "/combo?foo&%%",
	expectedStatus: http.StatusBadRequest,
	expectedError:  `cannot combine files: invalid file name "%": invalid URL escape "%%"`,
}, {
	about: "combo: invalid file path",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) string {
		return setupGUIArchive(c, storage, "1.0.0", nil)
	},
	pathAndquery:   "/combo?../../../../../../etc/passwd",
	expectedStatus: http.StatusBadRequest,
	expectedError:  `cannot combine files: forbidden file path "../../../../../../etc/passwd"`,
}, {
	about: "combo: invalid hash",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) string {
		setupGUIArchive(c, storage, "2.0.47", nil)
		return "invalid"
	},
	pathAndquery:   "/combo?foo",
	expectedStatus: http.StatusNotFound,
	expectedError:  `resource with "invalid" hash not found`,
}, {
	about: "combo: success",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) string {
		return setupGUIArchive(c, storage, "1.0.0", map[string]string{
			"static/gui/build/tng/picard.js":  "enterprise",
			"static/gui/build/ds9/sisko.js":   "deep space nine",
			"static/gui/build/voy/janeway.js": "voyager",
			"static/gui/build/borg.js":        "cube",
		})
	},
	pathAndquery:        "/combo?voy/janeway.js&tng/picard.js&borg.js&ds9/sisko.js",
	expectedStatus:      http.StatusOK,
	expectedContentType: apiserver.JSMimeType,
	expectedBody: `voyager
/* janeway.js */
enterprise
/* picard.js */
cube
/* borg.js */
deep space nine
/* sisko.js */
`,
}, {
	about: "combo: non-existing files ignored + different content types",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) string {
		return setupGUIArchive(c, storage, "1.0.0", map[string]string{
			"static/gui/build/foo.css": "my-style",
		})
	},
	pathAndquery:        "/combo?no-such.css&foo.css&bad-wolf.css",
	expectedStatus:      http.StatusOK,
	expectedContentType: "text/css; charset=utf-8",
	expectedBody: `my-style
/* foo.css */
`,
}, {
	about: "static files",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) string {
		return setupGUIArchive(c, storage, "1.0.0", map[string]string{
			"static/file.js": "static file content",
		})
	},
	pathAndquery:        "/static/file.js",
	expectedStatus:      http.StatusOK,
	expectedContentType: apiserver.JSMimeType,
	expectedBody:        "static file content",
}, {
	about: "static files: invalid hash",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) string {
		setupGUIArchive(c, storage, "2.0.47", nil)
		return "bad-wolf"
	},
	pathAndquery:   "/static/file.js",
	expectedStatus: http.StatusNotFound,
	expectedError:  `resource with "bad-wolf" hash not found`,
}}

func (s *guiSuite) TestGUIHandler(c *gc.C) {
	if runtime.GOOS == "windows" {
		// Skipping the tests on Windows is not a problem as the Juju GUI is
		// only served from Linux machines.
		c.Skip("bzip2 command not available")
	}
	sendRequest := func(setup guiSetupFunc, pathAndquery string) *http.Response {
		// Set up the GUI base directory.
		datadir := filepath.ToSlash(s.DataDir())
		baseDir := filepath.FromSlash(agenttools.SharedGUIDir(datadir))
		defer func() {
			os.Chmod(baseDir, 0755)
			os.Remove(baseDir)
		}()

		// Run specific test set up.
		var hash string
		if setup != nil {
			storage, err := s.State.GUIStorage()
			c.Assert(err, jc.ErrorIsNil)
			defer storage.Close()

			// Ensure the GUI storage is empty.
			allMeta, err := storage.AllMetadata()
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(allMeta, gc.HasLen, 0)

			hash = setup(c, baseDir, storage)
		}

		// Send a request to the test path.
		if pathAndquery == "" {
			pathAndquery = "/"
		}
		return s.sendRequest(c, httpRequestParams{
			url: s.guiURL(c, hash, pathAndquery),
		})
	}

	for i, test := range guiHandlerTests {
		c.Logf("\n%d: %s", i, test.about)

		// Reset the db so that the GUI storage is empty in each test.
		s.Reset(c)

		// Perform the request.
		resp := sendRequest(test.setup, test.pathAndquery)

		// Check the response.
		if test.expectedStatus == 0 {
			test.expectedStatus = http.StatusOK
		}
		if test.expectedError != "" {
			test.expectedContentType = params.ContentTypeJSON
		}
		body := assertResponse(c, resp, test.expectedStatus, test.expectedContentType)
		if test.expectedError == "" {
			c.Assert(string(body), gc.Equals, test.expectedBody)
		} else {
			var jsonResp params.ErrorResult
			err := json.Unmarshal(body, &jsonResp)
			c.Assert(err, jc.ErrorIsNil, gc.Commentf("body: %s", body))
			c.Assert(jsonResp.Error.Message, gc.Matches, test.expectedError)
		}
	}
}

func (s *guiSuite) TestGUIIndex(c *gc.C) {
	storage, err := s.State.GUIStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()

	// Create a Juju GUI archive and save it into the storage.
	indexContent := `
<!DOCTYPE html>
<html>
<body>
    comboURL: {{.comboURL}}
    configURL: {{.configURL}}
    debug: {{.debug}}
    spriteContent: {{.spriteContent}}
</body>
</html>`
	hash := setupGUIArchive(c, storage, "2.0.0", map[string]string{
		guiIndexPath:         indexContent,
		apiserver.SpritePath: "sprite content",
	})
	expectedIndexContent := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<body>
    comboURL: /gui/%[1]s/%[2]s/combo
    configURL: /gui/%[1]s/%[2]s/config.js
    debug: false
    spriteContent: sprite content
</body>
</html>`, s.modelUUID, hash)
	// Make a request for the Juju GUI index.
	resp := s.sendRequest(c, httpRequestParams{
		url: s.guiURL(c, "", "/"),
	})
	body := assertResponse(c, resp, http.StatusOK, "text/html; charset=utf-8")
	c.Assert(string(body), gc.Equals, expectedIndexContent)

	// Non-handled paths are served by the index handler.
	resp = s.sendRequest(c, httpRequestParams{
		url: s.guiURL(c, "", "/no-such-path/"),
	})
	body = assertResponse(c, resp, http.StatusOK, "text/html; charset=utf-8")
	c.Assert(string(body), gc.Equals, expectedIndexContent)
}

func (s *guiSuite) TestGUIConfig(c *gc.C) {
	storage, err := s.State.GUIStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()

	// Create a Juju GUI archive and save it into the storage.
	configContent := `
var config = {
    // This is just an example and does not reflect the real Juju GUI config.
    base: '{{.base}}',
    host: '{{.host}}',
    socket: '{{.socket}}',
    uuid: '{{.uuid}}',
    version: '{{.version}}'
};`
	hash := setupGUIArchive(c, storage, "2.0.0", map[string]string{
		guiConfigPath: configContent,
	})
	expectedConfigContent := fmt.Sprintf(`
var config = {
    // This is just an example and does not reflect the real Juju GUI config.
    base: '/gui/%s/',
    host: '%s',
    socket: '/model/$uuid/api',
    uuid: '%s',
    version: '%s'
};`, s.modelUUID, s.baseURL(c).Host, s.modelUUID, jujuversion.Current)

	// Make a request for the Juju GUI config.
	resp := s.sendRequest(c, httpRequestParams{
		url: s.guiURL(c, hash, "/config.js"),
	})
	body := assertResponse(c, resp, http.StatusOK, apiserver.JSMimeType)
	c.Assert(string(body), gc.Equals, expectedConfigContent)
}

func (s *guiSuite) TestGUIDirectory(c *gc.C) {
	storage, err := s.State.GUIStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()

	// Create a Juju GUI archive and save it into the storage.
	indexContent := "<!DOCTYPE html><html><body>Exterminate!</body></html>"
	hash := setupGUIArchive(c, storage, "2.0.0", map[string]string{
		guiIndexPath:         indexContent,
		apiserver.SpritePath: "",
	})

	// Initially the GUI directory on the server is empty.
	baseDir := agenttools.SharedGUIDir(s.DataDir())
	c.Assert(baseDir, jc.DoesNotExist)

	// Make a request for the Juju GUI.
	resp := s.sendRequest(c, httpRequestParams{
		url: s.guiURL(c, "", "/"),
	})
	body := assertResponse(c, resp, http.StatusOK, "text/html; charset=utf-8")
	c.Assert(string(body), gc.Equals, indexContent)

	// Now the GUI is stored on disk, in a directory corresponding to its
	// archive SHA256 hash.
	indexPath := filepath.Join(baseDir, hash, guiIndexPath)
	c.Assert(indexPath, jc.IsNonEmptyFile)
	b, err := ioutil.ReadFile(indexPath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(b), gc.Equals, indexContent)
}

type guiArchiveSuite struct {
	authHttpSuite
}

var _ = gc.Suite(&guiArchiveSuite{})

// guiURL returns the URL used to retrieve info on or upload Juju GUI archives.
func (s *guiArchiveSuite) guiURL(c *gc.C) string {
	u := s.baseURL(c)
	u.Path = "/gui-archive"
	return u.String()
}

var guiArchiveGetTests = []struct {
	about    string
	versions []string
}{{
	about: "empty storage",
}, {
	about:    "one version",
	versions: []string{"2.42.0"},
}, {
	about:    "multiple versions",
	versions: []string{"2.42.0", "3.0.0", "2.47.1"},
}}

func (s *guiArchiveSuite) TestGUIArchiveGet(c *gc.C) {
	for i, test := range guiArchiveGetTests {
		c.Logf("\n%d: %s", i, test.about)

		uploadVersions := func(versions []string) params.GUIArchiveResponse {
			// Open the GUI storage.
			storage, err := s.State.GUIStorage()
			c.Assert(err, jc.ErrorIsNil)
			defer storage.Close()

			// Add the versions to the storage.
			expectedVersions := make([]params.GUIArchiveVersion, len(versions))
			for i, vers := range versions {
				files := map[string]string{"file": fmt.Sprintf("content %d", i)}
				hash := setupGUIArchive(c, storage, vers, files)
				expectedVersions[i] = params.GUIArchiveVersion{
					Version: version.MustParse(vers),
					SHA256:  hash,
					// TODO frankban: use real current one.
					Current: true,
				}
			}
			return params.GUIArchiveResponse{
				Versions: expectedVersions,
			}
		}

		// Reset the db so that the GUI storage is empty in each test.
		s.Reset(c)

		// Send the request to retrieve GUI version information.
		expectedResponse := uploadVersions(test.versions)
		resp := s.sendRequest(c, httpRequestParams{
			url: s.guiURL(c),
		})

		// Check that a successful response is returned.
		body := assertResponse(c, resp, http.StatusOK, params.ContentTypeJSON)
		var jsonResponse params.GUIArchiveResponse
		err := json.Unmarshal(body, &jsonResponse)
		c.Assert(err, jc.ErrorIsNil, gc.Commentf("body: %s", body))
		c.Assert(jsonResponse, jc.DeepEquals, expectedResponse)
	}
}

var guiArchivePostErrorsTests = []struct {
	about           string
	contentType     string
	query           string
	noContentLength bool
	expectedStatus  int
	expectedError   string
}{{
	about:          "no content type",
	expectedStatus: http.StatusBadRequest,
	expectedError:  fmt.Sprintf(`invalid content type "": expected %q`, apiserver.BZMimeType),
}, {
	about:          "invalid content type",
	contentType:    "text/html",
	expectedStatus: http.StatusBadRequest,
	expectedError:  fmt.Sprintf(`invalid content type "text/html": expected %q`, apiserver.BZMimeType),
}, {
	about:          "no version provided",
	contentType:    apiserver.BZMimeType,
	expectedStatus: http.StatusBadRequest,
	expectedError:  "version parameter not provided",
}, {
	about:          "invalid version",
	contentType:    apiserver.BZMimeType,
	query:          "?version=bad-wolf",
	expectedStatus: http.StatusBadRequest,
	expectedError:  `invalid version parameter "bad-wolf"`,
}, {
	about:           "no content length provided",
	contentType:     apiserver.BZMimeType,
	query:           "?version=2.0.42&hash=sha",
	noContentLength: true,
	expectedStatus:  http.StatusBadRequest,
	expectedError:   "content length not provided",
}, {
	about:          "no hash provided",
	contentType:    apiserver.BZMimeType,
	query:          "?version=2.0.42",
	expectedStatus: http.StatusBadRequest,
	expectedError:  "hash parameter not provided",
}, {
	about:          "content hash mismatch",
	contentType:    apiserver.BZMimeType,
	query:          "?version=2.0.42&hash=bad-wolf",
	expectedStatus: http.StatusBadRequest,
	expectedError:  "archive does not match provided hash",
}}

func (s *guiArchiveSuite) TestGUIArchivePostErrors(c *gc.C) {
	type exoticReader struct {
		io.Reader
	}
	for i, test := range guiArchivePostErrorsTests {
		c.Logf("\n%d: %s", i, test.about)

		// Prepare the request.
		var r io.Reader = strings.NewReader("archive contents")
		if test.noContentLength {
			// net/http will automatically add a Content-Length header if it
			// sees *strings.Reader, but not if it's a type it doesn't know.
			r = exoticReader{r}
		}

		// Send the request and retrieve the error response.
		resp := s.authRequest(c, httpRequestParams{
			method:      "POST",
			url:         s.guiURL(c) + test.query,
			contentType: test.contentType,
			body:        r,
		})
		body := assertResponse(c, resp, test.expectedStatus, params.ContentTypeJSON)
		var jsonResp params.ErrorResult
		err := json.Unmarshal(body, &jsonResp)
		c.Assert(err, jc.ErrorIsNil, gc.Commentf("body: %s", body))
		c.Assert(jsonResp.Error.Message, gc.Matches, test.expectedError)
	}
}

func (s *guiArchiveSuite) TestGUIArchivePostErrorUnauthorized(c *gc.C) {
	resp := s.sendRequest(c, httpRequestParams{
		method:      "POST",
		url:         s.guiURL(c) + "?version=2.0.0&hash=sha",
		contentType: apiserver.BZMimeType,
		body:        strings.NewReader("archive contents"),
	})
	body := assertResponse(c, resp, http.StatusUnauthorized, params.ContentTypeJSON)
	var jsonResp params.ErrorResult
	err := json.Unmarshal(body, &jsonResp)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("body: %s", body))
	c.Assert(jsonResp.Error.Message, gc.Matches, "cannot open state: no credentials provided")
}

func (s *guiArchiveSuite) TestGUIArchivePostSuccess(c *gc.C) {
	// Create a GUI archive to be uploaded.
	vers := "2.0.42"
	r, hash, size := makeGUIArchive(c, vers, nil)

	// Prepare and send the request to upload a new GUI archive.
	v := url.Values{}
	v.Set("version", vers)
	v.Set("hash", hash)
	resp := s.authRequest(c, httpRequestParams{
		method:      "POST",
		url:         s.guiURL(c) + "?" + v.Encode(),
		contentType: apiserver.BZMimeType,
		body:        r,
	})

	// Check that the response reflects a successful upload.
	body := assertResponse(c, resp, http.StatusOK, params.ContentTypeJSON)
	var jsonResponse params.GUIArchiveVersion
	err := json.Unmarshal(body, &jsonResponse)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("body: %s", body))
	c.Assert(jsonResponse, jc.DeepEquals, params.GUIArchiveVersion{
		Version: version.MustParse(vers),
		SHA256:  hash,
		Current: true,
	})

	// Check that the new archive is actually present in the GUI storage.
	storage, err := s.State.GUIStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()
	allMeta, err := storage.AllMetadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allMeta, gc.HasLen, 1)
	c.Assert(allMeta[0].SHA256, gc.Equals, hash)
	c.Assert(allMeta[0].Size, gc.Equals, size)
}

// makeGUIArchive creates a Juju GUI tar.bz2 archive with the given files.
// The files parameter maps file names (relative to the internal "jujugui"
// directory) to their contents. This function returns a reader for the
// archive, its hash and size.
func makeGUIArchive(c *gc.C, vers string, files map[string]string) (r io.Reader, hash string, size int64) {
	if runtime.GOOS == "windows" {
		// Skipping the tests on Windows is not a problem as the Juju GUI is
		// only served from Linux machines.
		c.Skip("bzip2 command not available")
	}
	cmd := exec.Command("bzip2", "--compress", "--stdout", "--fast")

	stdin, err := cmd.StdinPipe()
	c.Assert(err, jc.ErrorIsNil)
	stdout, err := cmd.StdoutPipe()
	c.Assert(err, jc.ErrorIsNil)

	err = cmd.Start()
	c.Assert(err, jc.ErrorIsNil)

	tw := tar.NewWriter(stdin)
	baseDir := filepath.Join("jujugui-"+vers, "jujugui")
	err = tw.WriteHeader(&tar.Header{
		Name:     baseDir,
		Mode:     0700,
		Typeflag: tar.TypeDir,
	})
	c.Assert(err, jc.ErrorIsNil)
	for path, content := range files {
		name := filepath.Join(baseDir, path)
		err = tw.WriteHeader(&tar.Header{
			Name:     filepath.Dir(name),
			Mode:     0700,
			Typeflag: tar.TypeDir,
		})
		c.Assert(err, jc.ErrorIsNil)
		err = tw.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0600,
			Size: int64(len(content)),
		})
		c.Assert(err, jc.ErrorIsNil)
		_, err = io.WriteString(tw, content)
		c.Assert(err, jc.ErrorIsNil)
	}
	err = tw.Close()
	c.Assert(err, jc.ErrorIsNil)
	err = stdin.Close()
	c.Assert(err, jc.ErrorIsNil)

	h := sha256.New()
	r = io.TeeReader(stdout, h)
	b, err := ioutil.ReadAll(r)
	c.Assert(err, jc.ErrorIsNil)

	err = cmd.Wait()
	c.Assert(err, jc.ErrorIsNil)

	return bytes.NewReader(b), fmt.Sprintf("%x", h.Sum(nil)), int64(len(b))
}

// setupGUIArchive creates a Juju GUI tar.bz2 archive with the given version
// and files and saves it into the given storage. The Juju GUI archive SHA256
// hash is returned.
func setupGUIArchive(c *gc.C, storage binarystorage.Storage, vers string, files map[string]string) (hash string) {
	r, hash, size := makeGUIArchive(c, vers, files)
	err := storage.Add(r, binarystorage.Metadata{
		Version: vers,
		Size:    size,
		SHA256:  hash,
	})
	c.Assert(err, jc.ErrorIsNil)
	return hash
}
