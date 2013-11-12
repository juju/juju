// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"launchpad.net/juju-core/log"
	"launchpad.net/juju-core/utils"
)

// CacheDir stores the charm cache directory path.
var CacheDir string

// InfoResponse is sent by the charm store in response to charm-info requests.
type InfoResponse struct {
	Revision int      `json:"revision"` // Zero is valid. Can't omitempty.
	Sha256   string   `json:"sha256,omitempty"`
	Digest   string   `json:"digest,omitempty"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
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

// Repository respresents a collection of charms.
type Repository interface {
	Get(curl *URL) (Charm, error)
	Latest(curl *URL) (int, error)
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
	authToken string
}

var Store = &CharmStore{BaseURL: "https://store.juju.ubuntu.com"}

// Return a copy of the store with auth token set
func (s *CharmStore) WithAuthToken(token string) Repository {
	authCS := *s
	authCS.authToken = token
	return &authCS
}

// Perform an http get, adding custom auth header if necessary.
func (s *CharmStore) get(url string) (resp *http.Response, err error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	if s.authToken != "" {
		// To comply with RFC 2617, we send the auth token in
		// the Authorization header with a custom auth scheme
		// and the token.
		req.Header.Add("Authorization", "charmstore "+s.authToken)
	}
	return http.DefaultClient.Do(req)
}

// Info returns details for a charm in the charm store.
func (s *CharmStore) Info(curl *URL) (*InfoResponse, error) {
	key := curl.String()
	resp, err := s.get(s.BaseURL + "/charm-info?charms=" + url.QueryEscape(key))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	infos := make(map[string]*InfoResponse)
	if err = json.Unmarshal(body, &infos); err != nil {
		return nil, err
	}
	info, found := infos[key]
	if !found {
		return nil, fmt.Errorf("charm: charm store returned response without charm %q", key)
	}
	if len(info.Errors) == 1 && info.Errors[0] == "entry not found" {
		return nil, &NotFoundError{fmt.Sprintf("charm not found: %s", curl)}
	}
	return info, nil
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
		return nil, fmt.Errorf("charm: charm store returned response without charm %q", key)
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

// revision returns the revision and SHA256 digest of the charm referenced by curl.
func (s *CharmStore) revision(curl *URL) (revision int, digest string, err error) {
	info, err := s.Info(curl)
	if err != nil {
		return 0, "", err
	}
	for _, w := range info.Warnings {
		log.Warningf("charm: charm store reports for %q: %s", curl, w)
	}
	if info.Errors != nil {
		return 0, "", fmt.Errorf("charm info errors for %q: %s", curl, strings.Join(info.Errors, "; "))
	}
	return info.Revision, info.Sha256, nil
}

// Latest returns the latest revision of the charm referenced by curl, regardless
// of the revision set on curl itself.
func (s *CharmStore) Latest(curl *URL) (int, error) {
	rev, _, err := s.revision(curl.WithRevision(-1))
	return rev, err
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
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != digest {
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
	if err := os.MkdirAll(CacheDir, 0755); err != nil {
		return nil, err
	}
	rev, digest, err := s.revision(curl)
	if err != nil {
		return nil, err
	}
	if curl.Revision == -1 {
		curl = curl.WithRevision(rev)
	} else if curl.Revision != rev {
		return nil, fmt.Errorf("charm: store returned charm with wrong revision for %q", curl.String())
	}
	path := filepath.Join(CacheDir, Quote(curl.String())+".charm")
	if verify(path, digest) != nil {
		resp, err := s.get(s.BaseURL + "/charm/" + url.QueryEscape(curl.Path()))
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
	Path string
}

// Latest returns the latest revision of the charm referenced by curl, regardless
// of the revision set on curl itself.
func (r *LocalRepository) Latest(curl *URL) (int, error) {
	ch, err := r.Get(curl.WithRevision(-1))
	if err != nil {
		return 0, err
	}
	return ch.Revision(), nil
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
			log.Warningf("charm: failed to load charm at %q: %s", chPath, err)
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
