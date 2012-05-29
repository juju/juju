package charm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"launchpad.net/juju/go/log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// InfoResponse is sent by the charm store in response to charm-info requests.
type InfoResponse struct {
	Revision int      `json:"revision"` // Zero is valid. Can't omitempty.
	Sha256   string   `json:"sha256,omitempty"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// Repository respresents a collection of charms.
type Repository interface {
	Get(curl *URL) (Charm, error)
	Latest(curl *URL) (int, error)
}

// store is a Repository that talks to the juju charm server (in ../store).
type store struct {
	baseURL   string
	cachePath string
}

const (
	storeURL  = "https://store.juju.ubuntu.com"
	cachePath = "$HOME/.juju/cache"
)

// Store returns a Repository that provides access to the juju charm store.
func Store() Repository {
	return &store{storeURL, os.ExpandEnv(cachePath)}
}

// info returns the revision and SHA256 digest of the charm referenced by curl.
func (s *store) info(curl *URL) (rev int, digest string, err error) {
	key := curl.String()
	resp, err := http.Get(s.baseURL + "/charm-info?charms=" + url.QueryEscape(key))
	if err != nil {
		return
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}
	infos := make(map[string]*InfoResponse)
	if err = json.Unmarshal(body, &infos); err != nil {
		return
	}
	info, found := infos[key]
	if !found {
		err = fmt.Errorf("charm: charm store returned response without charm %q", key)
		return
	}
	for _, w := range info.Warnings {
		log.Printf("WARNING: charm store reports for %q: %s", key, w)
	}
	if info.Errors != nil {
		err = fmt.Errorf(
			"charm info errors for %q: %s", key, strings.Join(info.Errors, "; "),
		)
		return
	}
	return info.Revision, info.Sha256, nil
}

// Latest returns the latest revision of the charm referenced by curl, regardless
// of the revision set on curl itself.
func (s *store) Latest(curl *URL) (int, error) {
	rev, _, err := s.info(curl.WithRevision(-1))
	return rev, err
}

// verify returns an error unless a file exists at path with a hex-encoded
// SHA256 matching digest.
func verify(path, digest string) error {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	h := sha256.New()
	h.Write(b)
	if hex.EncodeToString(h.Sum(nil)) != digest {
		return fmt.Errorf("bad SHA256 of %q", path)
	}
	return nil
}

// Get returns the charm referenced by curl.
func (s *store) Get(curl *URL) (Charm, error) {
	if err := os.MkdirAll(s.cachePath, 0755); err != nil {
		return nil, err
	}
	rev, digest, err := s.info(curl)
	if err != nil {
		return nil, err
	}
	if curl.Revision == -1 {
		curl = curl.WithRevision(rev)
	} else if curl.Revision != rev {
		return nil, fmt.Errorf("charm: store returned charm with wrong revision for %q", curl.String())
	}
	path := filepath.Join(s.cachePath, Quote(curl.String())+".charm")
	if verify(path, digest) != nil {
		resp, err := http.Get(s.baseURL + "/charm/" + url.QueryEscape(curl.Path()))
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		f, err := ioutil.TempFile(s.cachePath, "charm-download")
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
		if err := os.Rename(dlPath, path); err != nil {
			return nil, err
		}
	}
	if err := verify(path, digest); err != nil {
		return nil, err
	}
	return ReadBundle(path)
}

// LocalRepository represents a local directory containing charms organised
// by series.
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

// charms returns all charms within the subdirectory named for series.
func (r *LocalRepository) charms(series string) []Charm {
	path := filepath.Join(r.Path, series)
	infos, err := ioutil.ReadDir(path)
	if err != nil {
		return nil
	}
	var charms []Charm
	for _, info := range infos {
		chPath := filepath.Join(path, info.Name())
		if info.IsDir() {
			if ch, err := ReadDir(chPath); err != nil {
				log.Printf("WARNING: failed to load charm at %q: %s", chPath, err)
			} else {
				charms = append(charms, ch)
			}
		} else {
			if ch, err := ReadBundle(chPath); err != nil {
				log.Printf("WARNING: failed to load charm at %q: %s", chPath, err)
			} else {
				charms = append(charms, ch)
			}
		}
	}
	return charms
}

// Get returns a charm matching curl, if one exists. If curl has a revision of
// -1, it returns the latest charm that matches curl. If multiple candidates
// satisfy the foregoing, the first one encountered will be returned.
func (r *LocalRepository) Get(curl *URL) (Charm, error) {
	if curl.Schema != "local" {
		return nil, fmt.Errorf("bad schema: %q", curl.Schema)
	}
	var candidates []Charm
	for _, ch := range r.charms(curl.Series) {
		if ch.Meta().Name == curl.Name {
			if ch.Revision() == curl.Revision {
				return ch, nil
			}
			candidates = append(candidates, ch)
		}
	}
	if candidates == nil || curl.Revision != -1 {
		return nil, fmt.Errorf("no charms found matching %q", curl.String())
	}
	latest := candidates[0]
	for _, ch := range candidates[1:] {
		if ch.Revision() > latest.Revision() {
			latest = ch
		}
	}
	return latest, nil
}
