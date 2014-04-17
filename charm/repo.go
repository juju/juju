// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"launchpad.net/juju-core/utils"
)

// CacheDir stores the charm cache directory path.
var CacheDir string

// InfoResponse is sent by the charm store in response to charm-info requests.
type InfoResponse struct {
	CanonicalURL string   `json:"canonical-url,omitempty"`
	Revision     int      `json:"revision"` // Zero is valid. Can't omitempty.
	Sha256       string   `json:"sha256,omitempty"`
	Digest       string   `json:"digest,omitempty"`
	Errors       []string `json:"errors,omitempty"`
	Warnings     []string `json:"warnings,omitempty"`
}

// EventResponse is sent by the charm store in response to charm-event requests.
type EventResponse struct {
	Kind     string   `json:"kind"`
	Revision int      `json:"revision"` // Zero is valid. Can't omitempty.
	Digest   string   `json:"digest,omitempty"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	Time     string   `json:"time,omitempty"`
}

// CharmRevision holds the revision number of a charm and any error
// encountered in retrieving it.
type CharmRevision struct {
	Revision int
	Sha256   string
	Err      error
}

// Repository represents a collection of charms.
type Repository interface {
	Get(curl *URL) (Charm, error)
	Latest(curls ...*URL) ([]CharmRevision, error)
	Resolve(ref Reference) (*URL, error)
}

// Latest returns the latest revision of the charm referenced by curl, regardless
// of the revision set on each curl.
// This is a helper which calls the bulk method and unpacks a single result.
func Latest(repo Repository, curl *URL) (int, error) {
	revs, err := repo.Latest(curl)
	if err != nil {
		return 0, err
	}
	if len(revs) != 1 {
		return 0, fmt.Errorf("expected 1 result, got %d", len(revs))
	}
	rev := revs[0]
	if rev.Err != nil {
		return 0, rev.Err
	}
	return rev.Revision, nil
}

// NotFoundError represents an error indicating that the requested data wasn't found.
type NotFoundError struct {
	msg string
}

func (e *NotFoundError) Error() string {
	return e.msg
}

// CharmStore is a Repository that provides access to the public juju charm store.
type CharmStore struct {
	BaseURL   string
	authAttrs string // a list of attr=value pairs, comma separated
	jujuAttrs string // a list of attr=value pairs, comma separated
	testMode  bool
}

var _ Repository = (*CharmStore)(nil)

var Store = &CharmStore{BaseURL: "https://store.juju.ubuntu.com"}

// WithAuthAttrs return a Repository with the authentication token list set.
// authAttrs is a list of attr=value pairs.
func (s *CharmStore) WithAuthAttrs(authAttrs string) Repository {
	authCS := *s
	authCS.authAttrs = authAttrs
	return &authCS
}

// WithTestMode returns a Repository where testMode is set to value passed to
// this method.
func (s *CharmStore) WithTestMode(testMode bool) Repository {
	newRepo := *s
	newRepo.testMode = testMode
	return &newRepo
}

// WithJujuAttrs returns a Repository with the Juju metadata attributes set.
// jujuAttrs is a list of attr=value pairs.
func (s *CharmStore) WithJujuAttrs(jujuAttrs string) Repository {
	jujuCS := *s
	jujuCS.jujuAttrs = jujuAttrs
	return &jujuCS
}

// Perform an http get, adding custom auth header if necessary.
func (s *CharmStore) get(url string) (resp *http.Response, err error) {
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

// Resolve canonicalizes charm URLs, resolving references and implied series.
func (s *CharmStore) Resolve(ref Reference) (*URL, error) {
	infos, err := s.Info(ref)
	if err != nil {
		return nil, err
	}
	if len(infos) == 0 {
		return nil, fmt.Errorf("missing response when resolving charm URL: %q", ref)
	}
	if infos[0].CanonicalURL == "" {
		return nil, fmt.Errorf("cannot resolve charm URL: %q", ref)
	}
	curl, err := ParseURL(infos[0].CanonicalURL)
	if err != nil {
		return nil, err
	}
	return curl, nil
}

// Info returns details for all the specified charms in the charm store.
func (s *CharmStore) Info(curls ...Location) ([]*InfoResponse, error) {
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
func (s *CharmStore) Event(curl *URL, digest string) (*EventResponse, error) {
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

// revisions returns the revisions of the charms referenced by curls.
func (s *CharmStore) revisions(curls ...Location) (revisions []CharmRevision, err error) {
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
func (s *CharmStore) Latest(curls ...*URL) ([]CharmRevision, error) {
	baseCurls := make([]Location, len(curls))
	for i, curl := range curls {
		baseCurls[i] = curl.WithRevision(-1)
	}
	return s.revisions(baseCurls...)
}

// BranchLocation returns the location for the branch holding the charm at curl.
func (s *CharmStore) BranchLocation(curl *URL) string {
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
func (s *CharmStore) CharmURL(location string) (*URL, error) {
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
			return ParseURL(fmt.Sprintf("cs:%s/%s", u[1], u[2]))
		}
		if len(u) == 4 && u[0] == "charms" && u[3] == "trunk" {
			return ParseURL(fmt.Sprintf("cs:%s/%s", u[1], u[2]))
		}
		if len(u) == 5 && u[1] == "charms" && u[4] == "trunk" && len(u[0]) > 0 && u[0][0] == '~' {
			return ParseURL(fmt.Sprintf("cs:%s/%s/%s", u[0], u[2], u[3]))
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
func (s *CharmStore) Get(curl *URL) (Charm, error) {
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
	path := filepath.Join(CacheDir, Quote(curl.String())+".charm")
	if verify(path, digest) != nil {
		store_url := s.BaseURL + "/charm/" + url.QueryEscape(curl.Path())
		if s.testMode {
			store_url = store_url + "?stats=0"
		}
		resp, err := s.get(store_url)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
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
	return ReadBundle(path)
}

// LocalRepository represents a local directory containing subdirectories
// named after an Ubuntu series, each of which contains charms targeted for
// that series. For example:
//
//   /path/to/repository/oneiric/mongodb/
//   /path/to/repository/precise/mongodb.charm
//   /path/to/repository/precise/wordpress/
type LocalRepository struct {
	Path          string
	defaultSeries string
}

var _ Repository = (*LocalRepository)(nil)

// WithDefaultSeries returns a Repository with the default series set.
func (r *LocalRepository) WithDefaultSeries(defaultSeries string) Repository {
	localRepo := *r
	localRepo.defaultSeries = defaultSeries
	return &localRepo
}

// Resolve canonicalizes charm URLs, resolving references and implied series.
func (r *LocalRepository) Resolve(ref Reference) (*URL, error) {
	if r.defaultSeries == "" {
		return nil, fmt.Errorf("cannot resolve, repository has no default series: %q", ref)
	}
	return &URL{Reference: ref, Series: r.defaultSeries}, nil
}

// Latest returns the latest revision of the charm referenced by curl, regardless
// of the revision set on curl itself.
func (r *LocalRepository) Latest(curls ...*URL) ([]CharmRevision, error) {
	result := make([]CharmRevision, len(curls))
	for i, curl := range curls {
		ch, err := r.Get(curl.WithRevision(-1))
		if err == nil {
			result[i].Revision = ch.Revision()
		} else {
			result[i].Err = err
		}
	}
	return result, nil
}

func repoNotFound(path string) error {
	return &NotFoundError{fmt.Sprintf("no repository found at %q", path)}
}

func charmNotFound(curl *URL, repoPath string) error {
	return &NotFoundError{fmt.Sprintf("charm not found in %q: %s", repoPath, curl)}
}

func mightBeCharm(info os.FileInfo) bool {
	if info.IsDir() {
		return !strings.HasPrefix(info.Name(), ".")
	}
	return strings.HasSuffix(info.Name(), ".charm")
}

// Get returns a charm matching curl, if one exists. If curl has a revision of
// -1, it returns the latest charm that matches curl. If multiple candidates
// satisfy the foregoing, the first one encountered will be returned.
func (r *LocalRepository) Get(curl *URL) (Charm, error) {
	if curl.Schema != "local" {
		return nil, fmt.Errorf("local repository got URL with non-local schema: %q", curl)
	}
	info, err := os.Stat(r.Path)
	if err != nil {
		if os.IsNotExist(err) {
			err = repoNotFound(r.Path)
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, repoNotFound(r.Path)
	}
	path := filepath.Join(r.Path, curl.Series)
	infos, err := ioutil.ReadDir(path)
	if err != nil {
		return nil, charmNotFound(curl, r.Path)
	}
	var latest Charm
	for _, info := range infos {
		chPath := filepath.Join(path, info.Name())
		if info.Mode()&os.ModeSymlink != 0 {
			var err error
			if info, err = os.Stat(chPath); err != nil {
				return nil, err
			}
		}
		if !mightBeCharm(info) {
			continue
		}
		if ch, err := Read(chPath); err != nil {
			logger.Warningf("failed to load charm at %q: %s", chPath, err)
		} else if ch.Meta().Name == curl.Name {
			if ch.Revision() == curl.Revision {
				return ch, nil
			}
			if latest == nil || ch.Revision() > latest.Revision() {
				latest = ch
			}
		}
	}
	if curl.Revision == -1 && latest != nil {
		return latest, nil
	}
	return nil, charmNotFound(curl, r.Path)
}
