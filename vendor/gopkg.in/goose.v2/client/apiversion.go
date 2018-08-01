package client

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"

	goosehttp "gopkg.in/goose.v2/http"
	"gopkg.in/goose.v2/logging"
)

type apiVersion struct {
	major int
	minor int
}

type apiVersionInfo struct {
	Version apiVersion       `json:"id"`
	Links   []apiVersionLink `json:"links"`
	Status  string           `json:"status"`
}

type apiVersionLink struct {
	Href string `json:"href"`
	Rel  string `json:"rel"`
}

type apiURLVersion struct {
	rootURL          url.URL
	serviceURLSuffix string
	versions         []apiVersionInfo
}

// getAPIVersionURL returns a full formed serviceURL based on the API version requested,
// the rootURL and the serviceURLSuffix.  If there is no match to the requested API
// version an error is returned.  If only the major number is defined for the requested
// version, the first match found is returned.
func (c *authenticatingClient) getAPIVersionURL(apiURLVersionInfo *apiURLVersion, requested apiVersion) (string, error) {
	var match string
	for _, v := range apiURLVersionInfo.versions {
		if v.Version.major != requested.major {
			continue
		}
		if requested.minor != -1 && v.Version.minor != requested.minor {
			continue
		}
		for _, link := range v.Links {
			if link.Rel != "self" {
				continue
			}
			hrefURL, err := url.Parse(link.Href)
			if err != nil {
				return "", err
			}
			match = hrefURL.Path
		}
		if requested.minor != -1 {
			break
		}
	}
	if match == "" {
		return "", fmt.Errorf("could not find matching URL")
	}
	versionURL := apiURLVersionInfo.rootURL

	// https://bugs.launchpad.net/juju/+bug/1756135:
	// some hrefURL.Path contain more than the version, with
	// overlap on the apiURLVersionInfo.rootURL
	if strings.HasPrefix(match, "/"+versionURL.Path) {
		logger := logging.FromCompat(c.logger)
		logger.Tracef("version href path %q overlaps with url path %q, using version href", match, versionURL.Path)
		versionURL.Path = "/"
	}

	versionURL.Path = path.Join(versionURL.Path, match, apiURLVersionInfo.serviceURLSuffix)
	return versionURL.String(), nil
}

func (v *apiVersion) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	parsed, err := parseVersion(s)
	if err != nil {
		return err
	}
	*v = parsed
	return nil
}

// parseVersion takes a version string into the major and minor ints for an apiVersion
// structure. The string part of the data is returned by a request to List API versions
// send to an OpenStack service.  It is in the format "v<major>.<minor>". If apiVersion
// is empty, return {-1, -1}, to differentiate with "v0".
func parseVersion(s string) (apiVersion, error) {
	if s == "" {
		return apiVersion{-1, -1}, nil
	}
	s = strings.TrimPrefix(s, "v")
	parts := strings.SplitN(s, ".", 2)
	if len(parts) == 0 || len(parts) > 2 {
		return apiVersion{}, fmt.Errorf("invalid API version %q", s)
	}
	var minor int = -1
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return apiVersion{}, err
	}
	if len(parts) == 2 {
		var err error
		minor, err = strconv.Atoi(parts[1])
		if err != nil {
			return apiVersion{}, err
		}
	}
	return apiVersion{major, minor}, nil
}

func unmarshallVersion(Versions json.RawMessage) ([]apiVersionInfo, error) {
	// Some services respond with {"versions":[...]}, and
	// some respond with {"versions":{"values":[...]}}.
	var object interface{}
	var versions []apiVersionInfo
	if err := json.Unmarshal(Versions, &object); err != nil {
		return versions, err
	}
	if _, ok := object.(map[string]interface{}); ok {
		var valuesObject struct {
			Values []apiVersionInfo `json:"values"`
		}
		if err := json.Unmarshal(Versions, &valuesObject); err != nil {
			return versions, err
		}
		versions = valuesObject.Values
	} else {
		if err := json.Unmarshal(Versions, &versions); err != nil {
			return versions, err
		}
	}
	return versions, nil
}

// getAPIVersions returns data on the API versions supported by the specified
// service endpoint. Some OpenStack clouds do not support the version endpoint,
// in which case this method will return an empty set of versions in the result
// structure.
func (c *authenticatingClient) getAPIVersions(serviceCatalogURL string) (*apiURLVersion, error) {
	c.apiVersionMu.Lock()
	defer c.apiVersionMu.Unlock()
	logger := logging.FromCompat(c.logger)

	// Make sure we haven't already received the version info.
	// Cache done on serviceCatalogURL, https://<url.Host> is not
	// guarenteed to be unique.
	if apiInfo, ok := c.apiURLVersions[serviceCatalogURL]; ok {
		return apiInfo, nil
	}

	url, err := url.Parse(serviceCatalogURL)
	if err != nil {
		return nil, err
	}

	// Identify the version in the URL, if there is one, and record
	// everything proceeding it. We will need to append this to the
	// API version-specific base URL.
	var pathParts, origPathParts []string
	if url.Path != "/" {
		// If a version is included in the serviceCatalogURL, the
		// part before the version will end up in url, the part after
		// the version will end up in pathParts.  origPathParts is a
		// special case for "object-store"
		// e.g. https://storage101.dfw1.clouddrive.com/v1/MossoCloudFS_1019383
		// 		becomes: https://storage101.dfw1.clouddrive.com/ and MossoCloudFS_1019383
		// https://x.x.x.x/image
		// 		becomes: https://x.x.x.x/image/
		// https://x.x.x.x/cloudformation/v1
		// 		becomes: https://x.x.x.x/cloudformation/
		// https://x.x.x.x/compute/v2/9032a0051293421eb20b64da69d46252
		// 		becomes: https://x.x.x.x/compute/ and 9032a0051293421eb20b64da69d46252
		// https://x.x.x.x/volumev1/v2
		// 		becomes: https://x.x.x.x/volumev1/
		// http://y.y.y.y:9292
		// 		becomes: http://y.y.y.y:9292/
		// http://y.y.y.y:8774/v2/010ab46135ba414882641f663ec917b6
		//		becomes: http://y.y.y.y:8774/ and 010ab46135ba414882641f663ec917b6
		origPathParts = strings.Split(strings.Trim(url.Path, "/"), "/")
		pathParts = origPathParts
		found := false
		for i, p := range pathParts {
			if _, err := parseVersion(p); err == nil {
				found = true
				if i == 0 {
					pathParts = pathParts[1:]
					url.Path = "/"
				} else {
					url.Path = pathParts[0] + "/"
					pathParts = pathParts[2:]
				}
				break
			}
		}
		if !found {
			url.Path = path.Join(pathParts...) + "/"
			pathParts = []string{}
		}
	}
	logger.Tracef("api version will be inserted between %q and %q", url.String(), path.Join(pathParts...)+"/")

	getVersionURL := url.String()

	// If this is an object-store serviceType, or an object-store container endpoint,
	// there is no list version API call to make. Return a apiURLVersion which will
	// satisfy a requested api version of "", "v1" or "v1.0"
	if c.serviceURLs["object-store"] != "" && strings.Contains(serviceCatalogURL, c.serviceURLs["object-store"]) {
		url.Path = "/"
		objectStoreLink := apiVersionLink{Href: url.String(), Rel: "self"}
		objectStoreApiVersionInfo := []apiVersionInfo{
			{
				Version: apiVersion{major: 1, minor: 0},
				Links:   []apiVersionLink{objectStoreLink},
				Status:  "stable",
			},
			{
				Version: apiVersion{major: -1, minor: -1},
				Links:   []apiVersionLink{objectStoreLink},
				Status:  "stable",
			},
		}
		apiURLVersionInfo := &apiURLVersion{*url, strings.Join(origPathParts, "/"), objectStoreApiVersionInfo}
		c.apiURLVersions[serviceCatalogURL] = apiURLVersionInfo
		return apiURLVersionInfo, nil
	}

	var raw struct {
		Versions json.RawMessage `json:"versions"`
	}
	requestData := &goosehttp.RequestData{
		RespValue: &raw,
		ExpectedStatus: []int{
			http.StatusOK,
			http.StatusMultipleChoices,
		},
	}
	apiURLVersionInfo := &apiURLVersion{
		rootURL:          *url,
		serviceURLSuffix: strings.Join(pathParts, "/"),
	}
	if err := c.sendRequest("GET", getVersionURL, c.Token(), requestData); err != nil {
		logger.Warningf("API version discovery failed: %v", err)
		c.apiURLVersions[serviceCatalogURL] = apiURLVersionInfo
		return apiURLVersionInfo, nil
	}

	versions, err := unmarshallVersion(raw.Versions)
	if err != nil {
		logger.Debugf("API version discovery unmarshallVersion failed: %v", err)
		c.apiURLVersions[serviceCatalogURL] = apiURLVersionInfo
		return apiURLVersionInfo, nil
	}
	apiURLVersionInfo.versions = versions
	logger.Debugf("discovered API versions: %+v", versions)

	// Cache the result.
	c.apiURLVersions[serviceCatalogURL] = apiURLVersionInfo

	return apiURLVersionInfo, nil
}
