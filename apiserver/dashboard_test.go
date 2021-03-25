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
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	agenttools "github.com/juju/juju/agent/tools"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/params"
	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/state/binarystorage"
	"github.com/juju/juju/testing/factory"
)

const (
	dashboardConfigPath = "config.js.go"
	dashboardIndexPath  = "index.html"
)

type dashboardSuite struct {
	apiserverBaseSuite
}

var _ = gc.Suite(&dashboardSuite{})

// dashboardURL returns the complete URL where the Juju Dashboard can be found, including
// the pathAndQuery.
func (s *dashboardSuite) dashboardURL(pathAndQuery string) string {
	if pathAndQuery == "" {
		return s.URL(apiserver.DashboardURLPathPrefix, nil).String()
	}
	prefix := apiserver.DashboardURLPathPrefix
	if strings.HasPrefix(pathAndQuery, "static/") || pathAndQuery == "config.js" {
		prefix = ""
	}
	return s.urlFromBase(prefix, "", pathAndQuery)
}

func (s *dashboardSuite) urlFromBase(base, hash, pathAndQuery string) string {
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

type dashboardSetupFunc func(c *gc.C, baseDir string, storage binarystorage.Storage) string

var dashboardHandlerTests = []struct {
	// about describes the test.
	about string
	// setup is optionally used to set up the test.
	// It receives the Juju Dashboard base directory and an empty Dashboard storage.
	// Optionally it can return a Dashboard archive hash which is used by the test
	// to build the URL path for the HTTP request.
	setup dashboardSetupFunc
	// currentVersion optionally holds the Dashboard version that must be set as
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
	about: "Dashboard directory is a file",
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
	about: "Dashboard directory is unaccessible",
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
	about: "invalid Dashboard archive",
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
	about: "Dashboard current version not set",
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
		setupDashboardArchive(c, storage, "2.0.42", map[string]string{})
		return ""
	},
	currentVersion: "2.0.42",
	expectedStatus: http.StatusInternalServerError,
	expectedError:  "cannot read index file: .*: no such file or directory",
}, {
	about: "config: template not found",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) string {
		return setupDashboardArchive(c, storage, "2.0.42", nil)
	},
	currentVersion: "2.0.42",
	pathAndQuery:   "config.js",
	expectedStatus: http.StatusInternalServerError,
	expectedError:  "cannot parse template: .*: no such file or directory",
}, {
	about: "config: invalid template",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) string {
		return setupDashboardArchive(c, storage, "2.0.47", map[string]string{
			dashboardConfigPath: "{{.BadWolf.47}}",
		})
	},
	currentVersion: "2.0.47",
	pathAndQuery:   "config.js",
	expectedStatus: http.StatusInternalServerError,
	expectedError:  `cannot parse template: template: config.js.go:1: unexpected ".47" .*`,
}, {
	about: "static files",
	setup: func(c *gc.C, baseDir string, storage binarystorage.Storage) string {
		return setupDashboardArchive(c, storage, "1.0.0", map[string]string{
			"static/file.js": "static file content",
		})
	},
	currentVersion:      "1.0.0",
	pathAndQuery:        "static/file.js",
	expectedStatus:      http.StatusOK,
	expectedContentType: apiserver.JSMimeType,
	expectedBody:        "static file content",
}}

func (s *dashboardSuite) TestDashboardHandler(c *gc.C) {
	if runtime.GOOS == "windows" {
		// Skipping the tests on Windows is not a problem as the Juju Dashboard is
		// only served from Linux machines.
		c.Skip("bzip2 command not available")
	}
	sendRequest := func(setup dashboardSetupFunc, currentVersion, pathAndQuery string) *http.Response {
		// Set up the Dashboard base directory.
		datadir := filepath.ToSlash(s.config.DataDir)
		baseDir := filepath.FromSlash(agenttools.SharedDashboardDir(datadir))
		defer func() {
			os.Chmod(baseDir, 0755)
			os.Remove(baseDir)
		}()

		// Run specific test set up.
		if setup != nil {
			storage, err := s.State.DashboardStorage()
			c.Assert(err, jc.ErrorIsNil)
			defer storage.Close()

			// Ensure the Dashboard storage is empty.
			allMeta, err := storage.AllMetadata()
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(allMeta, gc.HasLen, 0)

			// Dashboard doesn't care about the hash.
			_ = setup(c, baseDir, storage)
		}

		// Set the current Dashboard version if required.
		if currentVersion != "" {
			err := s.State.DashboardSetVersion(version.MustParse(currentVersion))
			c.Assert(err, jc.ErrorIsNil)
		}

		// Send a request to the test path.
		return apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
			URL: s.dashboardURL(pathAndQuery),
		})
	}

	for i, test := range dashboardHandlerTests {
		c.Logf("\n%d: %s", i, test.about)

		// Reset the db so that the Dashboard storage is empty in each test.
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

func (s *dashboardSuite) TestDashboardIndex(c *gc.C) {
	tests := []struct {
		about               string
		dashboardVersion    string
		path                string
		expectedConfigQuery string
	}{{
		about:            "new Dashboard, new URL, root",
		dashboardVersion: "2.3.0",
	}, {
		about:            "new Dashboard, new URL, model path",
		dashboardVersion: "2.3.1",
		path:             "u/admin/testmodel/",
	}, {
		about:            "old Dashboard, new URL, root",
		dashboardVersion: "2.2.0",
	}, {
		about:               "old Dashboard, new URL, model path",
		dashboardVersion:    "2.0.0",
		path:                "u/admin/testmodel/",
		expectedConfigQuery: "?model-uuid=" + s.State.ModelUUID() + "&base-postfix=u/admin/testmodel",
	}}

	// Ensure there's an admin user with access to the testmodel model.
	s.Factory.MakeUser(c, &factory.UserParams{Name: "admin"})
	storage, err := s.State.DashboardStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()

	// Create a Juju Dashboard archive and save it into the storage.
	indexContent := `
<!DOCTYPE html>
<html>
<body>
    not a template
</body>
</html>`

	for i, test := range tests {
		c.Logf("\n%d: %s", i, test.about)
		vers := version.MustParse(test.dashboardVersion)
		_ = setupDashboardArchive(c, storage, vers.String(), map[string]string{
			dashboardIndexPath: indexContent,
		})
		err = s.State.DashboardSetVersion(vers)
		c.Assert(err, jc.ErrorIsNil)

		// Make a request for the Juju Dashboard index.
		resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
			URL: s.dashboardURL(test.path),
		})
		body := apitesting.AssertResponse(c, resp, http.StatusOK, "text/html; charset=utf-8")
		c.Assert(string(body), gc.Equals, indexContent)

		// Non-handled paths are served by the index handler.
		resp = apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
			URL: s.dashboardURL(test.path + "no-such-path/"),
		})
		body = apitesting.AssertResponse(c, resp, http.StatusOK, "text/html; charset=utf-8")
		c.Assert(string(body), gc.Equals, indexContent)
	}
}

func (s *dashboardSuite) TestDashboardIndexVersions(c *gc.C) {
	storage, err := s.State.DashboardStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()

	// Create Juju Dashboard archives and save it into the storage.
	setupDashboardArchive(c, storage, "1.0.0", map[string]string{
		dashboardIndexPath: "index version 1.0.0",
	})
	vers2 := version.MustParse("2.0.0")
	setupDashboardArchive(c, storage, vers2.String(), map[string]string{
		dashboardIndexPath: "index version 2.0.0",
	})
	vers3 := version.MustParse("3.0.0")
	setupDashboardArchive(c, storage, vers3.String(), map[string]string{
		dashboardIndexPath: "index version 3.0.0",
	})

	// Check that the correct index version is served.
	err = s.State.DashboardSetVersion(vers2)
	c.Assert(err, jc.ErrorIsNil)
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		URL: s.dashboardURL(""),
	})
	body := apitesting.AssertResponse(c, resp, http.StatusOK, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Equals, "index version 2.0.0")

	err = s.State.DashboardSetVersion(vers3)
	c.Assert(err, jc.ErrorIsNil)
	resp = apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		URL: s.dashboardURL(""),
	})
	body = apitesting.AssertResponse(c, resp, http.StatusOK, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Equals, "index version 3.0.0")
}

func (s *dashboardSuite) TestDashboardConfig(c *gc.C) {
	tests := []struct {
		about              string
		configPathAndQuery string
		expectedBaseURL    string
	}{{
		about:              "no uuid, no postfix",
		configPathAndQuery: "config.js",
		expectedBaseURL:    "/dashboard/",
	}}

	storage, err := s.State.DashboardStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()

	// Create a Juju Dashboard archive and save it into the storage.
	serverHost := s.server.Listener.Addr().String()
	_ = serverHost
	configContent := `
var config = {
    // This is just an example and does not reflect the real Juju Dashboard config.
    baseAppURL: '{{.baseAppURL}}',
    identityProviderAvailable: {{.identityProviderAvailable}},
};`
	vers := version.MustParse("2.0.0")
	// Dashboard doesn't care about the hash.
	_ = setupDashboardArchive(c, storage, vers.String(), map[string]string{
		dashboardConfigPath: configContent,
	})
	err = s.State.DashboardSetVersion(vers)
	c.Assert(err, jc.ErrorIsNil)

	for i, test := range tests {
		c.Logf("\n%d: %s", i, test.about)
		expectedConfigContent := fmt.Sprintf(`
var config = {
    // This is just an example and does not reflect the real Juju Dashboard config.
    baseAppURL: '%s',
    identityProviderAvailable: false,
};`, test.expectedBaseURL)

		// Make a request for the Juju Dashboard config.
		resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
			URL: s.dashboardURL(test.configPathAndQuery),
		})
		body := apitesting.AssertResponse(c, resp, http.StatusOK, apiserver.JSMimeType)
		c.Assert(string(body), gc.Equals, expectedConfigContent)
	}
}

func (s *dashboardSuite) TestDashboardDirectory(c *gc.C) {
	storage, err := s.State.DashboardStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()

	// Create a Juju Dashboard archive and save it into the storage.
	indexContent := "<!DOCTYPE html><html><body>Exterminate!</body></html>"
	vers := version.MustParse("2.0.0")
	hash := setupDashboardArchive(c, storage, vers.String(), map[string]string{
		dashboardIndexPath: indexContent,
	})
	err = s.State.DashboardSetVersion(vers)
	c.Assert(err, jc.ErrorIsNil)

	// Initially the Dashboard directory on the server is empty.
	baseDir := agenttools.SharedDashboardDir(s.config.DataDir)
	c.Assert(baseDir, jc.DoesNotExist)

	// Make a request for the Juju Dashboard.
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		URL: s.dashboardURL(""),
	})
	body := apitesting.AssertResponse(c, resp, http.StatusOK, "text/html; charset=utf-8")
	c.Assert(string(body), gc.Equals, indexContent)

	// Now the Dashboard is stored on disk, in a directory corresponding to its
	// archive SHA256 hash.
	indexPath := filepath.Join(baseDir, hash, dashboardIndexPath)
	c.Assert(indexPath, jc.IsNonEmptyFile)
	b, err := ioutil.ReadFile(indexPath)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(b), gc.Equals, indexContent)
}

type dashboardCandidSuite struct {
	apiserverBaseSuite
}

var _ = gc.Suite(&dashboardCandidSuite{})

func (s *dashboardCandidSuite) SetUpTest(c *gc.C) {
	s.ControllerConfig = map[string]interface{}{
		"identity-url": "https://candid.example.com",
	}
	s.apiserverBaseSuite.SetUpTest(c)
}

func (s *dashboardCandidSuite) TestDashboardConfig(c *gc.C) {
	storage, err := s.State.DashboardStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()

	// Create a Juju Dashboard archive and save it into the storage.
	configContent := `
var config = {
    // This is just an example and does not reflect the real Juju Dashboard config.
    identityProviderAvailable: {{.identityProviderAvailable}},
};`
	vers := version.MustParse("2.0.0")
	_ = setupDashboardArchive(c, storage, vers.String(), map[string]string{
		dashboardConfigPath: configContent,
	})
	err = s.State.DashboardSetVersion(vers)
	c.Assert(err, jc.ErrorIsNil)

	expectedConfigContent := `
var config = {
    // This is just an example and does not reflect the real Juju Dashboard config.
    identityProviderAvailable: true,
};`
	// Make a request for the Juju Dashboard config.
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		URL: s.URL("/config.js", nil).String(),
	})
	body := apitesting.AssertResponse(c, resp, http.StatusOK, apiserver.JSMimeType)
	c.Assert(string(body), gc.Equals, expectedConfigContent)
}

type dashboardArchiveSuite struct {
	apiserverBaseSuite
	// dashboardURL holds the URL used to retrieve info on or upload Juju Dashboard archives.
	dashboardURL string
}

var _ = gc.Suite(&dashboardArchiveSuite{})

func (s *dashboardArchiveSuite) SetUpTest(c *gc.C) {
	s.apiserverBaseSuite.SetUpTest(c)
	s.dashboardURL = s.URL("/dashboard-archive", nil).String()
}

func (s *dashboardArchiveSuite) TestDashboardArchiveMethodNotAllowed(c *gc.C) {
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method: "PUT",
		URL:    s.dashboardURL,
	})
	body := apitesting.AssertResponse(c, resp, http.StatusMethodNotAllowed, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Equals, "Method Not Allowed\n")
}

var dashboardArchiveGetTests = []struct {
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

func (s *dashboardArchiveSuite) TestDashboardArchiveGet(c *gc.C) {
	for i, test := range dashboardArchiveGetTests {
		c.Logf("\n%d: %s", i, test.about)

		uploadVersions := func(versions []string, current string) params.DashboardArchiveResponse {
			// Open the Dashboard storage.
			storage, err := s.State.DashboardStorage()
			c.Assert(err, jc.ErrorIsNil)
			defer storage.Close()

			// Add the versions to the storage.
			expectedVersions := make([]params.DashboardArchiveVersion, len(versions))
			for i, vers := range versions {
				files := map[string]string{"file": fmt.Sprintf("content %d", i)}
				v := version.MustParse(vers)
				hash := setupDashboardArchive(c, storage, vers, files)
				expectedVersions[i] = params.DashboardArchiveVersion{
					Version: v,
					SHA256:  hash,
				}
				if vers == current {
					err := s.State.DashboardSetVersion(v)
					c.Assert(err, jc.ErrorIsNil)
					expectedVersions[i].Current = true
				}
			}
			return params.DashboardArchiveResponse{
				Versions: expectedVersions,
			}
		}

		// Reset the db so that the Dashboard storage is empty in each test.
		s.TearDownTest(c)
		s.SetUpTest(c)

		// Send the request to retrieve Dashboard version information.
		expectedResponse := uploadVersions(test.versions, test.current)
		resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
			URL: s.dashboardURL,
		})

		// Check that a successful response is returned.
		body := apitesting.AssertResponse(c, resp, http.StatusOK, params.ContentTypeJSON)
		var jsonResponse params.DashboardArchiveResponse
		err := json.Unmarshal(body, &jsonResponse)
		c.Assert(err, jc.ErrorIsNil, gc.Commentf("body: %s", body))
		c.Assert(jsonResponse, jc.DeepEquals, expectedResponse)
	}
}

var dashboardArchivePostErrorsTests = []struct {
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

func (s *dashboardArchiveSuite) TestDashboardArchivePostErrors(c *gc.C) {
	type exoticReader struct {
		io.Reader
	}
	for i, test := range dashboardArchivePostErrorsTests {
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
			URL:         s.dashboardURL + test.query,
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

func (s *dashboardArchiveSuite) TestDashboardArchivePostErrorUnauthorized(c *gc.C) {
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "POST",
		URL:         s.dashboardURL + "?version=2.0.0&hash=sha",
		ContentType: apiserver.BZMimeType,
		Body:        strings.NewReader("archive contents"),
	})
	body := apitesting.AssertResponse(c, resp, http.StatusUnauthorized, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Equals, "authentication failed: no credentials provided\n")
}

func (s *dashboardArchiveSuite) TestDashboardArchivePostSuccess(c *gc.C) {
	// Create a Dashboard archive to be uploaded.
	vers := "2.0.42"
	r, hash, size := makeDashboardOrDashboardArchive(c, ".", vers, nil)

	// Prepare and send the request to upload a new Dashboard archive.
	v := url.Values{}
	v.Set("version", vers)
	v.Set("hash", hash)
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "POST",
		URL:         s.dashboardURL + "?" + v.Encode(),
		ContentType: apiserver.BZMimeType,
		Body:        r,
	})

	// Check that the response reflects a successful upload.
	body := apitesting.AssertResponse(c, resp, http.StatusOK, params.ContentTypeJSON)
	var jsonResponse params.DashboardArchiveVersion
	err := json.Unmarshal(body, &jsonResponse)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("Body: %s", body))
	c.Assert(jsonResponse, jc.DeepEquals, params.DashboardArchiveVersion{
		Version: version.MustParse(vers),
		SHA256:  hash,
		Current: false,
	})

	// Check that the new archive is actually present in the Dashboard storage.
	storage, err := s.State.DashboardStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()
	allMeta, err := storage.AllMetadata()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(allMeta, gc.HasLen, 1)
	c.Assert(allMeta[0].SHA256, gc.Equals, hash)
	c.Assert(allMeta[0].Size, gc.Equals, size)
}

func (s *dashboardArchiveSuite) TestDashboardArchivePostCurrent(c *gc.C) {
	// Add an existing Dashboard archive and set it as the current one.
	storage, err := s.State.DashboardStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()
	vers := version.MustParse("2.0.47")
	setupDashboardArchive(c, storage, vers.String(), nil)
	err = s.State.DashboardSetVersion(vers)
	c.Assert(err, jc.ErrorIsNil)

	// Create a Dashboard archive to be uploaded.
	r, hash, _ := makeDashboardOrDashboardArchive(c, ".", vers.String(), map[string]string{"filename": "content"})

	// Prepare and send the request to upload a new Dashboard archive.
	v := url.Values{}
	v.Set("version", vers.String())
	v.Set("hash", hash)
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "POST",
		URL:         s.dashboardURL + "?" + v.Encode(),
		ContentType: apiserver.BZMimeType,
		Body:        r,
	})

	// Check that the response reflects a successful upload.
	body := apitesting.AssertResponse(c, resp, http.StatusOK, params.ContentTypeJSON)
	var jsonResponse params.DashboardArchiveVersion
	err = json.Unmarshal(body, &jsonResponse)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("Body: %s", body))
	c.Assert(jsonResponse, jc.DeepEquals, params.DashboardArchiveVersion{
		Version: vers,
		SHA256:  hash,
		Current: true,
	})
}

type dashboardVersionSuite struct {
	apiserverBaseSuite
	// dashboardURL holds the URL used to select the Juju Dashboard archive version.
	dashboardURL string
}

var _ = gc.Suite(&dashboardVersionSuite{})

func (s *dashboardVersionSuite) SetUpTest(c *gc.C) {
	s.apiserverBaseSuite.SetUpTest(c)
	s.dashboardURL = s.URL("/dashboard-version", nil).String()
}

func (s *dashboardVersionSuite) TestDashboardVersionMethodNotAllowed(c *gc.C) {
	resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method: "GET",
		URL:    s.dashboardURL,
	})
	body := apitesting.AssertResponse(c, resp, http.StatusMethodNotAllowed, params.ContentTypeJSON)
	var jsonResp params.ErrorResult
	err := json.Unmarshal(body, &jsonResp)
	c.Assert(err, jc.ErrorIsNil, gc.Commentf("Body: %s", body))
	c.Assert(jsonResp.Error.Message, gc.Matches, `unsupported method: "GET"`)
}

var dashboardVersionPutTests = []struct {
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
	body: params.DashboardVersionRequest{
		Version: version.MustParse("2.0.1"),
	},
	expectedStatus: http.StatusNotFound,
	expectedError:  `cannot find "2.0.1" Dashboard version in the storage: 2.0.1 binary metadata not found`,
}, {
	about:       "success: switch to new version",
	contentType: params.ContentTypeJSON,
	body: params.DashboardVersionRequest{
		Version: version.MustParse("2.47.0"),
	},
	expectedStatus:  http.StatusOK,
	expectedVersion: "2.47.0",
}, {
	about:       "success: same version",
	contentType: params.ContentTypeJSON,
	body: params.DashboardVersionRequest{
		Version: version.MustParse("2.42.0"),
	},
	expectedStatus:  http.StatusOK,
	expectedVersion: "2.42.0",
}}

func (s *dashboardVersionSuite) TestDashboardVersionPut(c *gc.C) {
	// Prepare the initial Juju state.
	storage, err := s.State.DashboardStorage()
	c.Assert(err, jc.ErrorIsNil)
	defer storage.Close()
	setupDashboardArchive(c, storage, "2.42.0", nil)
	setupDashboardArchive(c, storage, "2.47.0", nil)
	err = s.State.DashboardSetVersion(version.MustParse("2.42.0"))
	c.Assert(err, jc.ErrorIsNil)

	for i, test := range dashboardVersionPutTests {
		c.Logf("\n%d: %s", i, test.about)

		// Prepare the request.
		content, err := json.Marshal(test.body)
		c.Assert(err, jc.ErrorIsNil)

		// Send the request and retrieve the response.
		resp := s.sendHTTPRequest(c, apitesting.HTTPRequestParams{
			Method:      "PUT",
			URL:         s.dashboardURL,
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
			vers, err := s.State.DashboardVersion()
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(vers.String(), gc.Equals, test.expectedVersion)
		}
	}
}

func (s *dashboardVersionSuite) TestDashboardVersionPutErrorUnauthorized(c *gc.C) {
	resp := apitesting.SendHTTPRequest(c, apitesting.HTTPRequestParams{
		Method:      "PUT",
		URL:         s.dashboardURL,
		ContentType: params.ContentTypeJSON,
	})
	body := apitesting.AssertResponse(c, resp, http.StatusUnauthorized, "text/plain; charset=utf-8")
	c.Assert(string(body), gc.Equals, "authentication failed: no credentials provided\n")
}

// makeDashboardOrDashboardArchive creates a Juju Dashboard tar.bz2 archive with the given files.
// The files parameter maps file names (relative to the internal "jujudashboard"
// directory) to their contents. This function returns a reader for the
// archive, its hash and size.
func makeDashboardOrDashboardArchive(c *gc.C, baseDir, vers string, files map[string]string) (r io.Reader, hash string, size int64) {
	if runtime.GOOS == "windows" {
		// Skipping the tests on Windows is not a problem as the Juju Dashboard is
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
	// baseDir maybe "." which is omitted by filepath.Join()
	versionFile := fmt.Sprintf("%s/version.json", baseDir)
	err = tw.WriteHeader(&tar.Header{
		Name:     filepath.Dir(versionFile),
		Mode:     0700,
		Typeflag: tar.TypeDir,
	})
	c.Assert(err, jc.ErrorIsNil)
	versionData := fmt.Sprintf(`{"version": %q}`, vers)
	err = tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeReg,
		Name:     versionFile,
		Size:     int64(len(versionData)),
		Mode:     0700,
	})
	c.Assert(err, jc.ErrorIsNil)
	_, err = io.WriteString(tw, versionData)
	c.Assert(err, jc.ErrorIsNil)
	for path, content := range files {
		fileDir := fmt.Sprintf("%s/%s", baseDir, filepath.Dir(path))
		fileName := fmt.Sprintf("%s/%s", baseDir, path)
		err = tw.WriteHeader(&tar.Header{
			Name:     fileDir,
			Mode:     0700,
			Typeflag: tar.TypeDir,
		})
		c.Assert(err, jc.ErrorIsNil)
		err = tw.WriteHeader(&tar.Header{
			Name: fileName,
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

func setupDashboardArchive(c *gc.C, storage binarystorage.Storage, vers string, files map[string]string) (hash string) {
	r, hash, size := makeDashboardOrDashboardArchive(c, ".", vers, files)
	err := storage.Add(r, binarystorage.Metadata{
		Version: vers,
		Size:    size,
		SHA256:  hash,
	})
	c.Assert(err, jc.ErrorIsNil)
	return hash
}
