// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/binarystorage"
	"github.com/juju/juju/version"
)

const (
	guiConfigPath = "templates/config.js.go"
	guiIndexPath  = "templates/index.html.go"
	guiSpritePath = "static/gui/build/app/assets/stack/svg/sprite.css.svg"
)

type guiSuite struct {
	authHttpSuite
}

var _ = gc.Suite(&guiSuite{})

// guiURL returns the complete URL where the Juju GUI can be found, including
// the given pathAndquery.
func (s *guiSuite) guiURL(c *gc.C, pathAndquery string) string {
	u := s.baseURL(c)
	parts := strings.SplitN(pathAndquery, "?", 2)
	u.Path = fmt.Sprintf("/gui/%s", s.modelUUID) + parts[0]
	if len(parts) == 2 {
		u.RawQuery = parts[1]
	}
	return u.String()
}

var guiHandlerTests = []struct {
	// about describes the test.
	about string
	// setup is optionally used to set up the test.
	// It receives the Juju GUI base directory and an empty GUI storage.
	setup func(c *gc.C, baseDir string, storage binarystorage.Storage)
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
	expectedStatus: http.StatusInternalServerError,
	expectedError:  "GUI metadata not found",
}, {
	about: "GUI directory is a file",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) {
		err := storage.Add(strings.NewReader(""), binarystorage.Metadata{
			SHA256: "fake-hash",
		})
		c.Assert(err, jc.ErrorIsNil)
		err = os.MkdirAll(baseDir, 0755)
		c.Assert(err, jc.ErrorIsNil)
		rootDir := filepath.Join(baseDir, "fake-hash")
		err = ioutil.WriteFile(rootDir, nil, 0644)
		c.Assert(err, jc.ErrorIsNil)
	},
	expectedStatus: http.StatusInternalServerError,
	expectedError:  "cannot use Juju GUI root directory .*",
}, {
	about: "GUI directory is unaccessible",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) {
		err := storage.Add(strings.NewReader(""), binarystorage.Metadata{
			SHA256: "fake-hash",
		})
		c.Assert(err, jc.ErrorIsNil)
		err = os.MkdirAll(baseDir, 0000)
		c.Assert(err, jc.ErrorIsNil)
	},
	expectedStatus: http.StatusInternalServerError,
	expectedError:  "cannot stat Juju GUI root directory: .*",
}, {
	about: "invalid GUI archive",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) {
		err := storage.Add(strings.NewReader(""), binarystorage.Metadata{
			SHA256: "fake-hash",
		})
		c.Assert(err, jc.ErrorIsNil)
	},
	expectedStatus: http.StatusInternalServerError,
	expectedError:  "cannot uncompress Juju GUI archive: cannot parse archive: .*",
}, {
	about: "index: sprite file not found",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) {
		setupGUIArchive(c, storage, "2.0.42", nil)
	},
	expectedStatus: http.StatusInternalServerError,
	expectedError:  "cannot read sprite file: .*",
}, {
	about: "index: template not found",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) {
		setupGUIArchive(c, storage, "2.0.42", map[string]string{
			guiSpritePath: "",
		})
	},
	expectedStatus: http.StatusInternalServerError,
	expectedError:  "cannot parse template: .*: no such file or directory",
}, {
	about: "index: invalid template",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) {
		setupGUIArchive(c, storage, "2.0.47", map[string]string{
			guiIndexPath:  "{{.BadWolf.47}}",
			guiSpritePath: "",
		})
	},
	expectedStatus: http.StatusInternalServerError,
	expectedError:  `cannot parse template: template: index.html.go:1: unexpected ".47" in operand`,
}, {
	about: "index: invalid template and context",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) {
		setupGUIArchive(c, storage, "2.0.47", map[string]string{
			guiIndexPath:  "{{range .debug}}{{end}}",
			guiSpritePath: "",
		})
	},
	expectedStatus: http.StatusInternalServerError,
	expectedError:  `cannot render template: template: .*: range can't iterate over .*`,
}, {
	about: "config: template not found",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) {
		setupGUIArchive(c, storage, "2.0.42", nil)
	},
	pathAndquery:   "/config.js",
	expectedStatus: http.StatusInternalServerError,
	expectedError:  "cannot parse template: .*: no such file or directory",
}, {
	about: "config: invalid template",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) {
		setupGUIArchive(c, storage, "2.0.47", map[string]string{
			guiConfigPath: "{{.BadWolf.47}}",
		})
	},
	pathAndquery:   "/config.js",
	expectedStatus: http.StatusInternalServerError,
	expectedError:  `cannot parse template: template: config.js.go:1: unexpected ".47" in operand`,
}, {
	about: "combo: invalid file name",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) {
		setupGUIArchive(c, storage, "1.0.0", nil)
	},
	pathAndquery:   "/combo?foo&%%",
	expectedStatus: http.StatusBadRequest,
	expectedError:  `cannot combine files: invalid file name "%": invalid URL escape "%%"`,
}, {
	about: "combo: invalid file path",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) {
		setupGUIArchive(c, storage, "1.0.0", nil)
	},
	pathAndquery:   "/combo?../../../../../../etc/passwd",
	expectedStatus: http.StatusBadRequest,
	expectedError:  `cannot combine files: forbidden file path "../../../../../../etc/passwd"`,
}, {
	about: "combo: success",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) {
		setupGUIArchive(c, storage, "1.0.0", map[string]string{
			"static/gui/build/tng/picard.js":  "enterprise",
			"static/gui/build/ds9/sisko.js":   "deep space nine",
			"static/gui/build/voy/janeway.js": "voyager",
			"static/gui/build/borg.js":        "cube",
		})
	},
	pathAndquery:        "/combo?voy/janeway.js&tng/picard.js&borg.js&ds9/sisko.js",
	expectedStatus:      http.StatusOK,
	expectedContentType: "application/javascript",
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
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) {
		setupGUIArchive(c, storage, "1.0.0", map[string]string{
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
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) {
		setupGUIArchive(c, storage, "1.0.0", map[string]string{
			"static/file.js": "static file content",
		})
	},
	pathAndquery:        "/static/file.js",
	expectedStatus:      http.StatusOK,
	expectedContentType: "application/javascript",
	expectedBody:        "static file content",
}}

func (s *guiSuite) TestGUIHandler(c *gc.C) {
	sendRequest := func(setup func(c *gc.C, baseDir string, storage binarystorage.Storage), pathAndquery string) *http.Response {
		// Set up the GUI base directory.
		baseDir := agenttools.SharedGUIDir(s.DataDir())
		defer func() {
			os.Chmod(baseDir, 0755)
			os.Remove(baseDir)
		}()

		// Run specific test set up.
		if setup != nil {
			storage, err := s.State.GUIStorage()
			c.Assert(err, jc.ErrorIsNil)
			defer storage.Close()

			// Ensure the GUI storage is empty.
			allMeta, err := storage.AllMetadata()
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(allMeta, gc.HasLen, 0)

			setup(c, baseDir, storage)
		}

		// Send a request to the test path.
		if pathAndquery == "" {
			pathAndquery = "/"
		}
		return s.sendRequest(c, httpRequestParams{
			url: s.guiURL(c, pathAndquery),
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
			test.expectedContentType = "application/json"
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
	setupGUIArchive(c, storage, "2.0.0", map[string]string{
		guiIndexPath:  indexContent,
		guiSpritePath: "sprite content",
	})

	// Make a request for the Juju GUI index.
	resp := s.sendRequest(c, httpRequestParams{
		url: s.guiURL(c, "/"),
	})
	body := assertResponse(c, resp, http.StatusOK, "text/html; charset=utf-8")
	expectedIndexContent := fmt.Sprintf(`
<!DOCTYPE html>
<html>
<body>
    comboURL: /gui/%s/combo
    configURL: /gui/%s/config.js
    debug: false
    spriteContent: sprite content
</body>
</html>`, s.modelUUID, s.modelUUID)
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
	setupGUIArchive(c, storage, "2.0.0", map[string]string{
		guiConfigPath: configContent,
	})

	// Make a request for the Juju GUI config.
	resp := s.sendRequest(c, httpRequestParams{
		url: s.guiURL(c, "/config.js"),
	})
	body := assertResponse(c, resp, http.StatusOK, "application/javascript")
	expectedConfigContent := fmt.Sprintf(`
var config = {
    // This is just an example and does not reflect the real Juju GUI config.
    base: '/gui/%s',
    host: '%s',
    socket: '/model/$uuid/api',
    uuid: '%s',
    version: '%s'
};`, s.modelUUID, s.baseURL(c).Host, s.modelUUID, version.Current)
	c.Assert(string(body), gc.Equals, expectedConfigContent)
}

func (s *guiSuite) TestGUIDirectory(c *gc.C) {
	storage, err := s.State.GUIStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()

	// Create a Juju GUI archive and save it into the storage.
	indexContent := "<!DOCTYPE html><html><body>Exterminate!</body></html>"
	hash := setupGUIArchive(c, storage, "2.0.0", map[string]string{
		guiIndexPath:  indexContent,
		guiSpritePath: "",
	})

	// Initially the GUI directory on the server is empty.
	baseDir := agenttools.SharedGUIDir(s.DataDir())
	c.Assert(baseDir, jc.DoesNotExist)

	// Make a request for the Juju GUI.
	resp := s.sendRequest(c, httpRequestParams{
		url: s.guiURL(c, "/"),
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

// makeGUIArchive creates a Juju GUI tar.bz2 archive with the given files.
// The files parameter maps file names (relative to the internal "jujugui"
// directory) to their contents. This function returns a reader for the
// archive, its hash and size.
func makeGUIArchive(c *gc.C, vers string, files map[string]string) (r io.Reader, hash string, size int64) {
	if runtime.GOOS == "windows" {
		// Skipping the tests on Windows is not a problem as the Juju GUI is
		// only served from Linux machines.
		c.Skip("tar command not available")
	}

	// Prepare the archive files and directories.
	target := filepath.Join(c.MkDir(), "gui.tar.bz2")
	source := c.MkDir()
	baseDir := "jujugui-" + vers
	guiDir := filepath.Join(source, baseDir, "jujugui")
	err := os.MkdirAll(guiDir, 0755)
	c.Assert(err, jc.ErrorIsNil)
	for path, content := range files {
		path = filepath.Join(guiDir, path)
		err = os.MkdirAll(filepath.Dir(path), 0755)
		c.Assert(err, jc.ErrorIsNil)
		err = ioutil.WriteFile(path, []byte(content), 0644)
		c.Assert(err, jc.ErrorIsNil)
	}

	// Build the tar.bz2 archive.
	err = exec.Command("tar", "cjf", target, "-C", source, baseDir).Run()
	c.Assert(err, jc.ErrorIsNil)

	// Calculate hash and size for the archive.
	h := sha256.New()
	f, err := os.Open(target)
	c.Assert(err, jc.ErrorIsNil)
	defer f.Close()
	r = io.TeeReader(f, h)
	b, err := ioutil.ReadAll(r)
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
