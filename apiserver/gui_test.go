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

	"github.com/juju/charmrepo/v5/csclient"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/params"
	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/state/binarystorage"
	"github.com/juju/juju/testing/factory"
)

const (
	guiConfigPath = "config.js.go"
	guiIndexPath  = "index.html"
)

type guiSuite struct {
	apiserverBaseSuite
}

var _ = gc.Suite(&guiSuite{})

// guiURL returns the complete URL where the Juju GUI can be found, including
// the given hash and pathAndQuery.
func (s *guiSuite) guiURL(hash, pathAndQuery string) string {
	if pathAndQuery == "" {
		return s.URL(apiserver.GUIURLPathPrefix, nil).String()
	}
	prefix := apiserver.GUIURLPathPrefix
	if strings.HasPrefix(pathAndQuery, "static/") || pathAndQuery == "config.js" {
		prefix = ""
	}
	return s.urlFromBase(prefix, "", pathAndQuery)
}

func (s *guiSuite) guiOldURL(hash, pathAndQuery string) string {
	base := apiserver.GUIURLPathPrefix + s.State.ModelUUID() + "/"
	return s.urlFromBase(base, hash, pathAndQuery)
}

func (s *guiSuite) urlFromBase(base, hash, pathAndQuery string) string {
	if hash != "" {
		base += hash + "/"
	}
	parts := strings.SplitN(pathAndQuery, "?", 2)
	u := s.URL(base+parts[0], nil)
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
	// currentVersion optionally holds the GUI version that must be set as
	// current right after setup is called and before the test is run.
	currentVersion string
	// pathAndQuery holds the optional path and query for the request, for
	// instance "/combo?file". If not provided, the "/" path is used.
	pathAndQuery string
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
	expectedError:  "Juju Dashboard not found",
}, {
	about: "GUI directory is a file",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) string {
		err := storage.Add(strings.NewReader(""), binarystorage.Metadata{
			SHA256:  "fake-hash",
			Version: "2.1.0",
		})
		c.Assert(err, jc.ErrorIsNil)
		err = os.MkdirAll(baseDir, 0755)
		c.Assert(err, jc.ErrorIsNil)
		rootDir := filepath.Join(baseDir, "fake-hash")
		err = ioutil.WriteFile(rootDir, nil, 0644)
		c.Assert(err, jc.ErrorIsNil)
		return ""
	},
	currentVersion: "2.1.0",
	expectedStatus: http.StatusInternalServerError,
	expectedError:  "cannot use Juju Dashboard root directory .*",
}, {
	about: "GUI directory is unaccessible",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) string {
		err := storage.Add(strings.NewReader(""), binarystorage.Metadata{
			SHA256:  "fake-hash",
			Version: "2.2.0",
		})
		c.Assert(err, jc.ErrorIsNil)
		err = os.MkdirAll(baseDir, 0000)
		c.Assert(err, jc.ErrorIsNil)
		return ""
	},
	currentVersion: "2.2.0",
	expectedStatus: http.StatusInternalServerError,
	expectedError:  "cannot stat Juju Dashboard root directory: .*",
}, {
	about: "invalid GUI archive",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) string {
		err := storage.Add(strings.NewReader(""), binarystorage.Metadata{
			SHA256:  "fake-hash",
			Version: "2.3.0",
		})
		c.Assert(err, jc.ErrorIsNil)
		return ""
	},
	currentVersion: "2.3.0",
	expectedStatus: http.StatusInternalServerError,
	expectedError:  "cannot uncompress Juju Dashboard archive: cannot parse archive: .*",
}, {
	about: "GUI current version not set",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) string {
		err := storage.Add(strings.NewReader(""), binarystorage.Metadata{
			SHA256: "fake-hash",
		})
		c.Assert(err, jc.ErrorIsNil)
		return ""
	},
	expectedStatus: http.StatusNotFound,
	expectedError:  "Juju Dashboard not found",
}, {
	about: "index: not found",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) string {
		setupGUIArchive(c, storage, "2.0.42", map[string]string{})
		return ""
	},
	currentVersion: "2.0.42",
	expectedStatus: http.StatusInternalServerError,
	expectedError:  "cannot read index file: .*: no such file or directory",
}, {
	about: "config: template not found",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) string {
		return setupGUIArchive(c, storage, "2.0.42", nil)
	},
	currentVersion: "2.0.42",
	pathAndQuery:   "config.js",
	expectedStatus: http.StatusInternalServerError,
	expectedError:  "cannot parse template: .*: no such file or directory",
}, {
	about: "config: invalid template",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) string {
		return setupGUIArchive(c, storage, "2.0.47", map[string]string{
			guiConfigPath: "{{.BadWolf.47}}",
		})
	},
	currentVersion: "2.0.47",
	pathAndQuery:   "config.js",
	expectedStatus: http.StatusInternalServerError,
	expectedError:  `cannot parse template: template: config.js.go:1: unexpected ".47" .*`,
}, {
	about: "static files",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) string {
		return setupGUIArchive(c, storage, "1.0.0", map[string]string{
			"static/file.js": "static file content",
		})
	},
	currentVersion:      "1.0.0",
	pathAndQuery:        "static/file.js",
	expectedStatus:      http.StatusOK,
	expectedContentType: apiserver.JSMimeType,
	expectedBody:        "static file content",
}}

func (s *guiSuite) TestGUIHandler(c *gc.C) {
	if runtime.GOOS == "windows" {
		// Skipping the tests on Windows is not a problem as the Juju GUI is
		// only served from Linux machines.
		c.Skip("bzip2 command not available")
	}
	sendRequest := func(setup guiSetupFunc, currentVersion, pathAndQuery string) *http.Response {
		// Set up the GUI base directory.
		datadir := filepath.ToSlash(s.config.DataDir)
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

		// Set the current GUI version if required.
		if currentVersion != "" {
			err := s.State.GUISetVersion(version.MustParse(currentVersion))
			c.Assert(err, jc.ErrorIsNil)
		}

		// Send a request to the test path.
		return apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
			URL: s.guiURL(hash, pathAndQuery),
		})
	}

	for i, test := range guiHandlerTests {
		c.Logf("\n%d: %s", i, test.about)

		// Reset the db so that the GUI storage is empty in each test.
		s.TearDownTest(c)
		s.SetUpTest(c)

		// Perform the request.
		resp := sendRequest(test.setup, test.currentVersion, test.pathAndQuery)

		// Check the response.
		if test.expectedStatus == 0 {
			test.expectedStatus = http.StatusOK
		}
		if test.expectedError != "" {
			test.expectedContentType = params.ContentTypeJSON
		}
		body := apitesting.AssertResponse(c, resp, test.expectedStatus, test.expectedContentType)
		if test.expectedError == "" {
			c.Check(string(body), gc.Equals, test.expectedBody)
		} else {
			var jsonResp params.ErrorResult
			err := json.Unmarshal(body, &jsonResp)
			if !c.Check(err, jc.ErrorIsNil, gc.Commentf("body: %s", body)) {
				continue
			}
			c.Check(jsonResp.Error.Message, gc.Matches, test.expectedError)
		}
	}
}

func (s *guiSuite) TestGUIIndex(c *gc.C) {
	tests := []struct {
		about               string
		guiVersion          string
		path                string
		getURL              func(hash, pathAndQuery string) string
		expectedConfigQuery string
	}{{
		about:      "new GUI, new URL, root",
		guiVersion: "2.3.0",
		getURL:     s.guiURL,
	}, {
		about:      "new GUI, new URL, model path",
		guiVersion: "2.3.1",
		path:       "u/admin/testmodel/",
		getURL:     s.guiURL,
	}, {
		about:               "new GUI, old URL, root",
		guiVersion:          "2.42.47",
		getURL:              s.guiOldURL,
		expectedConfigQuery: "?model-uuid=" + s.State.ModelUUID() + "&base-postfix=" + s.State.ModelUUID(),
	}, {
		about:               "new GUI, old URL, model path",
		guiVersion:          "2.3.0",
		path:                "u/admin/testmodel/",
		getURL:              s.guiOldURL,
		expectedConfigQuery: "?model-uuid=" + s.State.ModelUUID() + "&base-postfix=" + s.State.ModelUUID(),
	}, {
		about:      "old GUI, new URL, root",
		guiVersion: "2.2.0",
		getURL:     s.guiURL,
	}, {
		about:               "old GUI, new URL, model path",
		guiVersion:          "2.0.0",
		path:                "u/admin/testmodel/",
		getURL:              s.guiURL,
		expectedConfigQuery: "?model-uuid=" + s.State.ModelUUID() + "&base-postfix=u/admin/testmodel",
	}, {
		about:               "old GUI, old URL, root",
		guiVersion:          "1.42.47",
		getURL:              s.guiOldURL,
		expectedConfigQuery: "?model-uuid=" + s.State.ModelUUID() + "&base-postfix=" + s.State.ModelUUID(),
	}, {
		about:               "old GUI, old URL, model path",
		guiVersion:          "2.2.9",
		path:                "u/admin/testmodel/",
		getURL:              s.guiOldURL,
		expectedConfigQuery: "?model-uuid=" + s.State.ModelUUID() + "&base-postfix=" + s.State.ModelUUID(),
	}}

	// Ensure there's an admin user with access to the testmodel model.
	s.Factory.MakeUser(c, &factory.UserParams{Name: "admin"})
	storage, err := s.State.GUIStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()

	// Create a Juju GUI archive and save it into the storage.
	indexContent := `
<!DOCTYPE html>
<html>
<body>
    not a template
</body>
</html>`

	for i, test := range tests {
		c.Logf("\n%d: %s", i, test.about)
		vers := version.MustParse(test.guiVersion)
		_ = setupGUIArchive(c, storage, vers.String(), map[string]string{
			guiIndexPath: indexContent,
		})
		err = s.State.GUISetVersion(vers)
		c.Assert(err, jc.ErrorIsNil)

		// Make a request for the Juju GUI index.
		resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
			URL: test.getURL("", test.path),
		})
		body := apitesting.AssertResponse(c, resp, http.StatusOK, "text/html; charset=utf-8")
		c.Assert(string(body), gc.Equals, indexContent)

		// Non-handled paths are served by the index handler.
		resp = apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
			URL: test.getURL("", test.path+"no-such-path/"),
		})
		body = apitesting.AssertResponse(c, resp, http.StatusOK, "text/html; charset=utf-8")
		c.Assert(string(body), gc.Equals, indexContent)
	}

}

func (s *guiSuite) TestGUIIndexVersions(c *gc.C) {
	storage, err := s.State.GUIStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()

	// Create Juju GUI archives and save it into the storage.
	setupGUIArchive(c, storage, "1.0.0", map[string]string{
		guiIndexPath: "index version 1.0.0",
	})
	vers2 := version.MustParse("2.0.0")
	setupGUIArchive(c, storage, vers2.String(), map[string]string{
		guiIndexPath: "index version 2.0.0",
	})
	vers3 := version.MustParse("3.0.0")
	setupGUIArchive(c, storage, vers3.String(), map[string]string{
		guiIndexPath: "index version 3.0.0",
	})

	// Check that the correct index version is served.
	err = s.State.GUISetVersion(vers2)
	c.Assert(err, jc.ErrorIsNil)
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		URL: s.guiURL("", ""),
	})
	body := apitesting.AssertResponse(c, resp, http.StatusOK, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Equals, "index version 2.0.0")

	err = s.State.GUISetVersion(vers3)
	c.Assert(err, jc.ErrorIsNil)
	resp = apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		URL: s.guiURL("", ""),
	})
	body = apitesting.AssertResponse(c, resp, http.StatusOK, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Equals, "index version 3.0.0")
}

func (s *guiSuite) TestGUIConfig(c *gc.C) {
	tests := []struct {
		about              string
		configPathAndQuery string
		expectedBaseURL    string
	}{{
		about:              "no uuid, no postfix",
		configPathAndQuery: "config.js",
		expectedBaseURL:    "/dashboard/",
	}}

	storage, err := s.State.GUIStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()

	// Create a Juju GUI archive and save it into the storage.
	serverHost := s.server.Listener.Addr().String()
	_ = serverHost
	configContent := `
var config = {
    // This is just an example and does not reflect the real Juju GUI config.
    baseAppURL: '{{.baseAppURL}}',
    identityProviderAvailable: {{.identityProviderAvailable}},
};`
	vers := version.MustParse("2.0.0")
	hash := setupGUIArchive(c, storage, vers.String(), map[string]string{
		guiConfigPath: configContent,
	})
	err = s.State.GUISetVersion(vers)
	c.Assert(err, jc.ErrorIsNil)

	for i, test := range tests {
		c.Logf("\n%d: %s", i, test.about)
		expectedConfigContent := fmt.Sprintf(`
var config = {
    // This is just an example and does not reflect the real Juju GUI config.
    baseAppURL: '%s',
    identityProviderAvailable: false,
};`, test.expectedBaseURL)

		// Make a request for the Juju GUI config.
		resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
			URL: s.guiURL(hash, test.configPathAndQuery),
		})
		body := apitesting.AssertResponse(c, resp, http.StatusOK, apiserver.JSMimeType)
		c.Assert(string(body), gc.Equals, expectedConfigContent)
	}
}

func (s *guiSuite) TestGUIDirectory(c *gc.C) {
	storage, err := s.State.GUIStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()

	// Create a Juju GUI archive and save it into the storage.
	indexContent := "<!DOCTYPE html><html><body>Exterminate!</body></html>"
	vers := version.MustParse("2.0.0")
	hash := setupGUIArchive(c, storage, vers.String(), map[string]string{
		guiIndexPath: indexContent,
	})
	err = s.State.GUISetVersion(vers)
	c.Assert(err, jc.ErrorIsNil)

	// Initially the GUI directory on the server is empty.
	baseDir := agenttools.SharedGUIDir(s.config.DataDir)
	c.Assert(baseDir, jc.DoesNotExist)

	// Make a request for the Juju GUI.
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		URL: s.guiURL("", ""),
	})
	body := apitesting.AssertResponse(c, resp, http.StatusOK, "text/html; charset=utf-8")
	c.Assert(string(body), gc.Equals, indexContent)

	// Now the GUI is stored on disk, in a directory corresponding to its
	// archive SHA256 hash.
	indexPath := filepath.Join(baseDir, hash, guiIndexPath)
	c.Assert(indexPath, jc.IsNonEmptyFile)
	b, err := ioutil.ReadFile(indexPath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(b), gc.Equals, indexContent)
}

type guiCandidSuite struct {
	apiserverBaseSuite
}

var _ = gc.Suite(&guiCandidSuite{})

func (s *guiCandidSuite) SetUpTest(c *gc.C) {
	s.ControllerConfig = map[string]interface{}{
		"identity-url": "https://candid.example.com",
	}
	s.apiserverBaseSuite.SetUpTest(c)
}

func (s *guiCandidSuite) TestGUIConfig(c *gc.C) {
	storage, err := s.State.GUIStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()

	// Create a Juju GUI archive and save it into the storage.
	configContent := `
var config = {
    // This is just an example and does not reflect the real Juju GUI config.
    identityProviderAvailable: {{.identityProviderAvailable}},
};`
	vers := version.MustParse("2.0.0")
	_ = setupGUIArchive(c, storage, vers.String(), map[string]string{
		guiConfigPath: configContent,
	})
	err = s.State.GUISetVersion(vers)
	c.Assert(err, jc.ErrorIsNil)

	expectedConfigContent := `
var config = {
    // This is just an example and does not reflect the real Juju GUI config.
    identityProviderAvailable: true,
};`
	// Make a request for the Juju GUI config.
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		URL: s.URL("/config.js", nil).String(),
	})
	body := apitesting.AssertResponse(c, resp, http.StatusOK, apiserver.JSMimeType)
	c.Assert(string(body), gc.Equals, expectedConfigContent)
}

type guiArchiveSuite struct {
	apiserverBaseSuite
	// guiURL holds the URL used to retrieve info on or upload Juju GUI archives.
	guiURL string
}

var _ = gc.Suite(&guiArchiveSuite{})

func (s *guiArchiveSuite) SetUpTest(c *gc.C) {
	s.apiserverBaseSuite.SetUpTest(c)
	s.guiURL = s.URL("/gui-archive", nil).String()
}

func (s *guiArchiveSuite) TestGUIArchiveMethodNotAllowed(c *gc.C) {
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method: "PUT",
		URL:    s.guiURL,
	})
	body := apitesting.AssertResponse(c, resp, http.StatusMethodNotAllowed, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Equals, "Method Not Allowed\n")
}

var guiArchiveGetTests = []struct {
	about    string
	versions []string
	current  string
}{{
	about: "empty storage",
}, {
	about:    "one version",
	versions: []string{"2.42.0"},
}, {
	about:    "one version (current)",
	versions: []string{"2.42.0"},
	current:  "2.42.0",
}, {
	about:    "multiple versions",
	versions: []string{"2.42.0", "3.0.0", "2.47.1"},
}, {
	about:    "multiple versions (current)",
	versions: []string{"2.42.0", "3.0.0", "2.47.1"},
	current:  "3.0.0",
}}

func (s *guiArchiveSuite) TestGUIArchiveGet(c *gc.C) {
	for i, test := range guiArchiveGetTests {
		c.Logf("\n%d: %s", i, test.about)

		uploadVersions := func(versions []string, current string) params.GUIArchiveResponse {
			// Open the GUI storage.
			storage, err := s.State.GUIStorage()
			c.Assert(err, jc.ErrorIsNil)
			defer storage.Close()

			// Add the versions to the storage.
			expectedVersions := make([]params.GUIArchiveVersion, len(versions))
			for i, vers := range versions {
				files := map[string]string{"file": fmt.Sprintf("content %d", i)}
				v := version.MustParse(vers)
				hash := setupGUIArchive(c, storage, vers, files)
				expectedVersions[i] = params.GUIArchiveVersion{
					Version: v,
					SHA256:  hash,
				}
				if vers == current {
					err := s.State.GUISetVersion(v)
					c.Assert(err, jc.ErrorIsNil)
					expectedVersions[i].Current = true
				}
			}
			return params.GUIArchiveResponse{
				Versions: expectedVersions,
			}
		}

		// Reset the db so that the GUI storage is empty in each test.
		s.TearDownTest(c)
		s.SetUpTest(c)

		// Send the request to retrieve GUI version information.
		expectedResponse := uploadVersions(test.versions, test.current)
		resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
			URL: s.guiURL,
		})

		// Check that a successful response is returned.
		body := apitesting.AssertResponse(c, resp, http.StatusOK, params.ContentTypeJSON)
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
		resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
			Method:      "POST",
			URL:         s.guiURL + test.query,
			ContentType: test.contentType,
			Body:        r,
		})
		body := apitesting.AssertResponse(c, resp, test.expectedStatus, params.ContentTypeJSON)
		var jsonResp params.ErrorResult
		err := json.Unmarshal(body, &jsonResp)
		c.Assert(err, jc.ErrorIsNil, gc.Commentf("body: %s", body))
		c.Assert(jsonResp.Error.Message, gc.Matches, test.expectedError)
	}
}

func (s *guiArchiveSuite) TestGUIArchivePostErrorUnauthorized(c *gc.C) {
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "POST",
		URL:         s.guiURL + "?version=2.0.0&hash=sha",
		ContentType: apiserver.BZMimeType,
		Body:        strings.NewReader("archive contents"),
	})
	body := apitesting.AssertResponse(c, resp, http.StatusUnauthorized, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Equals, "authentication failed: no credentials provided\n")
}

func (s *guiArchiveSuite) TestGUIArchivePostSuccess(c *gc.C) {
	// Create a GUI archive to be uploaded.
	vers := "2.0.42"
	r, hash, size := makeGUIArchive(c, vers, nil)

	// Prepare and send the request to upload a new GUI archive.
	v := url.Values{}
	v.Set("version", vers)
	v.Set("hash", hash)
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "POST",
		URL:         s.guiURL + "?" + v.Encode(),
		ContentType: apiserver.BZMimeType,
		Body:        r,
	})

	// Check that the response reflects a successful upload.
	body := apitesting.AssertResponse(c, resp, http.StatusOK, params.ContentTypeJSON)
	var jsonResponse params.GUIArchiveVersion
	err := json.Unmarshal(body, &jsonResponse)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("Body: %s", body))
	c.Assert(jsonResponse, jc.DeepEquals, params.GUIArchiveVersion{
		Version: version.MustParse(vers),
		SHA256:  hash,
		Current: false,
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

func (s *guiArchiveSuite) TestGUIArchivePostCurrent(c *gc.C) {
	// Add an existing GUI archive and set it as the current one.
	storage, err := s.State.GUIStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()
	vers := version.MustParse("2.0.47")
	setupGUIArchive(c, storage, vers.String(), nil)
	err = s.State.GUISetVersion(vers)
	c.Assert(err, jc.ErrorIsNil)

	// Create a GUI archive to be uploaded.
	r, hash, _ := makeGUIArchive(c, vers.String(), map[string]string{"filename": "content"})

	// Prepare and send the request to upload a new GUI archive.
	v := url.Values{}
	v.Set("version", vers.String())
	v.Set("hash", hash)
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "POST",
		URL:         s.guiURL + "?" + v.Encode(),
		ContentType: apiserver.BZMimeType,
		Body:        r,
	})

	// Check that the response reflects a successful upload.
	body := apitesting.AssertResponse(c, resp, http.StatusOK, params.ContentTypeJSON)
	var jsonResponse params.GUIArchiveVersion
	err = json.Unmarshal(body, &jsonResponse)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("Body: %s", body))
	c.Assert(jsonResponse, jc.DeepEquals, params.GUIArchiveVersion{
		Version: vers,
		SHA256:  hash,
		Current: true,
	})
}

type guiVersionSuite struct {
	apiserverBaseSuite
	// guiURL holds the URL used to select the Juju GUI archive version.
	guiURL string
}

var _ = gc.Suite(&guiVersionSuite{})

func (s *guiVersionSuite) SetUpTest(c *gc.C) {
	s.apiserverBaseSuite.SetUpTest(c)
	s.guiURL = s.URL("/gui-version", nil).String()
}

func (s *guiVersionSuite) TestGUIVersionMethodNotAllowed(c *gc.C) {
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method: "GET",
		URL:    s.guiURL,
	})
	body := apitesting.AssertResponse(c, resp, http.StatusMethodNotAllowed, params.ContentTypeJSON)
	var jsonResp params.ErrorResult
	err := json.Unmarshal(body, &jsonResp)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("Body: %s", body))
	c.Assert(jsonResp.Error.Message, gc.Matches, `unsupported method: "GET"`)
}

var guiVersionPutTests = []struct {
	about           string
	contentType     string
	body            interface{}
	expectedStatus  int
	expectedVersion string
	expectedError   string
}{{
	about:          "no content type",
	expectedStatus: http.StatusBadRequest,
	expectedError:  fmt.Sprintf(`invalid content type "": expected %q`, params.ContentTypeJSON),
}, {
	about:          "invalid content type",
	contentType:    "text/html",
	expectedStatus: http.StatusBadRequest,
	expectedError:  fmt.Sprintf(`invalid content type "text/html": expected %q`, params.ContentTypeJSON),
}, {
	about:          "invalid body",
	contentType:    params.ContentTypeJSON,
	body:           "bad wolf",
	expectedStatus: http.StatusBadRequest,
	expectedError:  "invalid request body: json: .*",
}, {
	about:       "non existing version",
	contentType: params.ContentTypeJSON,
	body: params.GUIVersionRequest{
		Version: version.MustParse("2.0.1"),
	},
	expectedStatus: http.StatusNotFound,
	expectedError:  `cannot find "2.0.1" GUI version in the storage: 2.0.1 binary metadata not found`,
}, {
	about:       "success: switch to new version",
	contentType: params.ContentTypeJSON,
	body: params.GUIVersionRequest{
		Version: version.MustParse("2.47.0"),
	},
	expectedStatus:  http.StatusOK,
	expectedVersion: "2.47.0",
}, {
	about:       "success: same version",
	contentType: params.ContentTypeJSON,
	body: params.GUIVersionRequest{
		Version: version.MustParse("2.42.0"),
	},
	expectedStatus:  http.StatusOK,
	expectedVersion: "2.42.0",
}}

func (s *guiVersionSuite) TestGUIVersionPut(c *gc.C) {
	// Prepare the initial Juju state.
	storage, err := s.State.GUIStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()
	setupGUIArchive(c, storage, "2.42.0", nil)
	setupGUIArchive(c, storage, "2.47.0", nil)
	err = s.State.GUISetVersion(version.MustParse("2.42.0"))
	c.Assert(err, jc.ErrorIsNil)

	for i, test := range guiVersionPutTests {
		c.Logf("\n%d: %s", i, test.about)

		// Prepare the request.
		content, err := json.Marshal(test.body)
		c.Assert(err, jc.ErrorIsNil)

		// Send the request and retrieve the response.
		resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
			Method:      "PUT",
			URL:         s.guiURL,
			ContentType: test.contentType,
			Body:        bytes.NewReader(content),
		})
		var body []byte
		if test.expectedError != "" {
			body = apitesting.AssertResponse(c, resp, test.expectedStatus, params.ContentTypeJSON)
			var jsonResp params.ErrorResult
			err := json.Unmarshal(body, &jsonResp)
			c.Assert(err, jc.ErrorIsNil, gc.Commentf("Body: %s", body))
			c.Assert(jsonResp.Error.Message, gc.Matches, test.expectedError)
		} else {
			// we have no body content, so this won't check the content-type anyway
			// in go-1.9 it would set content-type=text/plain
			// in go-1.10 it does not set content-type
			body = apitesting.AssertResponse(c, resp, test.expectedStatus, "")
			c.Assert(body, gc.HasLen, 0)
			vers, err := s.State.GUIVersion()
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(vers.String(), gc.Equals, test.expectedVersion)
		}
	}
}

func (s *guiVersionSuite) TestGUIVersionPutErrorUnauthorized(c *gc.C) {
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "PUT",
		URL:         s.guiURL,
		ContentType: params.ContentTypeJSON,
	})
	body := apitesting.AssertResponse(c, resp, http.StatusUnauthorized, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Equals, "authentication failed: no credentials provided\n")
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
