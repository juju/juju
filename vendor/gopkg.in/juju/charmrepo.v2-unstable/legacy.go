// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmrepo // import "gopkg.in/juju/charmrepo.v2-unstable"

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/juju/utils"
	"gopkg.in/juju/charm.v6-unstable"
)

// LegacyCharmStore is a repository Interface that provides access to the
// legacy Juju charm store.
type LegacyCharmStore struct {
	BaseURL   string
	authAttrs string // a list of attr=value pairs, comma separated
	jujuAttrs string // a list of attr=value pairs, comma separated
	testMode  bool
}

var _ Interface = (*LegacyCharmStore)(nil)

var LegacyStore = &LegacyCharmStore{BaseURL: "https://store.juju.ubuntu.com"}

// WithAuthAttrs return a repository Interface with the authentication token
// list set. authAttrs is a list of attr=value pairs.
func (s *LegacyCharmStore) WithAuthAttrs(authAttrs string) Interface {
	authCS := *s
	authCS.authAttrs = authAttrs
	return &authCS
}

// WithTestMode returns a repository Interface where testMode is set to value
// passed to this method.
func (s *LegacyCharmStore) WithTestMode(testMode bool) Interface {
	newRepo := *s
	newRepo.testMode = testMode
	return &newRepo
}

// WithJujuAttrs returns a repository Interface with the Juju metadata
// attributes set. jujuAttrs is a list of attr=value pairs.
func (s *LegacyCharmStore) WithJujuAttrs(jujuAttrs string) Interface {
	jujuCS := *s
	jujuCS.jujuAttrs = jujuAttrs
	return &jujuCS
}

// Perform an http get, adding custom auth header if necessary.
func (s *LegacyCharmStore) get(url string) (resp *http.Response, err error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if s.authAttrs != "" {
		// To comply with RFC 2617, we send the authentication data in
		// the Authorization header with a custom auth scheme
		// and the authentication attributes.
		req.Header.Add("Authorization", "charmstore "+s.authAttrs)
	}
	if s.jujuAttrs != "" {
		// The use of "X-" to prefix custom header values is deprecated.
		req.Header.Add("Juju-Metadata", s.jujuAttrs)
	}
	return http.DefaultClient.Do(req)
}

// Resolve canonicalizes charm URLs any implied series in the reference.
func (s *LegacyCharmStore) Resolve(ref *charm.URL) (*charm.URL, []string, error) {
	infos, err := s.Info(ref)
	if err != nil {
		return nil, nil, err
	}
	if len(infos) == 0 {
		return nil, nil, fmt.Errorf("missing response when resolving charm URL: %q", ref)
	}
	if infos[0].CanonicalURL == "" {
		return nil, nil, fmt.Errorf("cannot resolve charm URL: %q", ref)
	}
	curl, err := charm.ParseURL(infos[0].CanonicalURL)
	if err != nil {
		return nil, nil, err
	}
	// Legacy store does not support returning the supported series.
	return curl, nil, nil
}

// Info returns details for all the specified charms in the charm store.
func (s *LegacyCharmStore) Info(curls ...charm.Location) ([]*InfoResponse, error) {
	baseURL := s.BaseURL + "/charm-info?"
	queryParams := make([]string, len(curls), len(curls)+1)
	for i, curl := range curls {
		queryParams[i] = "charms=" + url.QueryEscape(curl.String())
	}
	if s.testMode {
		queryParams = append(queryParams, "stats=0")
	}
	resp, err := s.get(baseURL + strings.Join(queryParams, "&"))
	if err != nil {
		if url_error, ok := err.(*url.Error); ok {
			switch url_error.Err.(type) {
			case *net.DNSError, *net.OpError:
				return nil, fmt.Errorf("Cannot access the charm store. Are you connected to the internet? Error details: %v", err)
			}
		}
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		errMsg := fmt.Errorf("Cannot access the charm store. Invalid response code: %q", resp.Status)
		body, readErr := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, readErr
		}
		logger.Errorf("%v Response body: %s", errMsg, body)
		return nil, errMsg
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	infos := make(map[string]*InfoResponse)
	if err = json.Unmarshal(body, &infos); err != nil {
		return nil, err
	}
	result := make([]*InfoResponse, len(curls))
	for i, curl := range curls {
		key := curl.String()
		info, found := infos[key]
		if !found {
			return nil, fmt.Errorf("charm store returned response without charm %q", key)
		}
		if len(info.Errors) == 1 && info.Errors[0] == "entry not found" {
			info.Errors[0] = fmt.Sprintf("charm not found: %s", curl)
		}
		result[i] = info
	}
	return result, nil
}

// Event returns details for a charm event in the charm store.
//
// If digest is empty, the latest event is returned.
func (s *LegacyCharmStore) Event(curl *charm.URL, digest string) (*EventResponse, error) {
	key := curl.String()
	query := key
	if digest != "" {
		query += "@" + digest
	}
	resp, err := s.get(s.BaseURL + "/charm-event?charms=" + url.QueryEscape(query))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	events := make(map[string]*EventResponse)
	if err = json.Unmarshal(body, &events); err != nil {
		return nil, err
	}
	event, found := events[key]
	if !found {
		return nil, fmt.Errorf("charm store returned response without charm %q", key)
	}
	if len(event.Errors) == 1 && event.Errors[0] == "entry not found" {
		if digest == "" {
			return nil, &NotFoundError{fmt.Sprintf("charm event not found for %q", curl)}
		} else {
			return nil, &NotFoundError{fmt.Sprintf("charm event not found for %q with digest %q", curl, digest)}
		}
	}
	return event, nil
}

// CharmRevision holds the revision number of a charm and any error
// encountered in retrieving it.
type CharmRevision struct {
	Revision int
	Sha256   string
	Err      error
}

// revisions returns the revisions of the charms referenced by curls.
func (s *LegacyCharmStore) revisions(curls ...charm.Location) (revisions []CharmRevision, err error) {
	infos, err := s.Info(curls...)
	if err != nil {
		return nil, err
	}
	revisions = make([]CharmRevision, len(infos))
	for i, info := range infos {
		for _, w := range info.Warnings {
			logger.Warningf("charm store reports for %q: %s", curls[i], w)
		}
		if info.Errors == nil {
			revisions[i].Revision = info.Revision
			revisions[i].Sha256 = info.Sha256
		} else {
			// If a charm is not found, we are more concise with the error message.
			if len(info.Errors) == 1 && strings.HasPrefix(info.Errors[0], "charm not found") {
				revisions[i].Err = fmt.Errorf(info.Errors[0])
			} else {
				revisions[i].Err = fmt.Errorf("charm info errors for %q: %s", curls[i], strings.Join(info.Errors, "; "))
			}
		}
	}
	return revisions, nil
}

// Latest returns the latest revision of the charms referenced by curls, regardless
// of the revision set on each curl.
func (s *LegacyCharmStore) Latest(curls ...*charm.URL) ([]CharmRevision, error) {
	baseCurls := make([]charm.Location, len(curls))
	for i, curl := range curls {
		baseCurls[i] = curl.WithRevision(-1)
	}
	return s.revisions(baseCurls...)
}

// BranchLocation returns the location for the branch holding the charm at curl.
func (s *LegacyCharmStore) BranchLocation(curl *charm.URL) string {
	if curl.User != "" {
		return fmt.Sprintf("lp:~%s/charms/%s/%s/trunk", curl.User, curl.Series, curl.Name)
	}
	return fmt.Sprintf("lp:charms/%s/%s", curl.Series, curl.Name)
}

var branchPrefixes = []string{
	"lp:",
	"bzr+ssh://bazaar.launchpad.net/+branch/",
	"bzr+ssh://bazaar.launchpad.net/",
	"http://launchpad.net/+branch/",
	"http://launchpad.net/",
	"https://launchpad.net/+branch/",
	"https://launchpad.net/",
	"http://code.launchpad.net/+branch/",
	"http://code.launchpad.net/",
	"https://code.launchpad.net/+branch/",
	"https://code.launchpad.net/",
}

// CharmURL returns the charm URL for the branch at location.
func (s *LegacyCharmStore) CharmURL(location string) (*charm.URL, error) {
	var l string
	if len(location) > 0 && location[0] == '~' {
		l = location
	} else {
		for _, prefix := range branchPrefixes {
			if strings.HasPrefix(location, prefix) {
				l = location[len(prefix):]
				break
			}
		}
	}
	if l != "" {
		for len(l) > 0 && l[len(l)-1] == '/' {
			l = l[:len(l)-1]
		}
		u := strings.Split(l, "/")
		if len(u) == 3 && u[0] == "charms" {
			return charm.ParseURL(fmt.Sprintf("cs:%s/%s", u[1], u[2]))
		}
		if len(u) == 4 && u[0] == "charms" && u[3] == "trunk" {
			return charm.ParseURL(fmt.Sprintf("cs:%s/%s", u[1], u[2]))
		}
		if len(u) == 5 && u[1] == "charms" && u[4] == "trunk" && len(u[0]) > 0 && u[0][0] == '~' {
			return charm.ParseURL(fmt.Sprintf("cs:%s/%s/%s", u[0], u[2], u[3]))
		}
	}
	return nil, fmt.Errorf("unknown branch location: %q", location)
}

// verify returns an error unless a file exists at path with a hex-encoded
// SHA256 matching digest.
func verify(path, digest string) error {
	hash, _, err := utils.ReadFileSHA256(path)
	if err != nil {
		return err
	}
	if hash != digest {
		return fmt.Errorf("bad SHA256 of %q", path)
	}
	return nil
}

// Get returns the charm referenced by curl.
// CacheDir must have been set, otherwise Get will panic.
func (s *LegacyCharmStore) Get(curl *charm.URL) (charm.Charm, error) {
	// The cache location must have been previously set.
	if CacheDir == "" {
		panic("charm cache directory path is empty")
	}
	if err := os.MkdirAll(CacheDir, os.FileMode(0755)); err != nil {
		return nil, err
	}
	revInfo, err := s.revisions(curl)
	if err != nil {
		return nil, err
	}
	if len(revInfo) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(revInfo))
	}
	if revInfo[0].Err != nil {
		return nil, revInfo[0].Err
	}
	rev, digest := revInfo[0].Revision, revInfo[0].Sha256
	if curl.Revision == -1 {
		curl = curl.WithRevision(rev)
	} else if curl.Revision != rev {
		return nil, fmt.Errorf("store returned charm with wrong revision %d for %q", rev, curl.String())
	}
	path := filepath.Join(CacheDir, charm.Quote(curl.String())+".charm")
	if verify(path, digest) != nil {
		store_url := s.BaseURL + "/charm/" + curl.Path()
		if s.testMode {
			store_url = store_url + "?stats=0"
		}
		resp, err := s.get(store_url)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("bad status from request for %q: %q", store_url, resp.Status)
		}
		f, err := ioutil.TempFile(CacheDir, "charm-download")
		if err != nil {
			return nil, err
		}
		dlPath := f.Name()
		_, err = io.Copy(f, resp.Body)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			os.Remove(dlPath)
			return nil, err
		}
		if err := utils.ReplaceFile(dlPath, path); err != nil {
			return nil, err
		}
	}
	if err := verify(path, digest); err != nil {
		return nil, err
	}
	return charm.ReadCharmArchive(path)
}

// GetBundle is only defined for implementing Interface.
func (s *LegacyCharmStore) GetBundle(curl *charm.URL) (charm.Bundle, error) {
	return nil, errors.New("not implemented: legacy API does not support bundles")
}

// LegacyInferRepository returns a charm repository inferred from the provided
// charm or bundle reference. Local references will use the provided path.
func LegacyInferRepository(ref *charm.URL, localRepoPath string) (repo Interface, err error) {
	switch ref.Schema {
	case "cs":
		repo = LegacyStore
	case "local":
		if localRepoPath == "" {
			return nil, errors.New("path to local repository not specified")
		}
		repo = &LocalRepository{Path: localRepoPath}
	default:
		return nil, fmt.Errorf("unknown schema for charm reference %q", ref)
	}
	return
}
