package charm_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/charm"
	"launchpad.net/juju/go/log"
	"launchpad.net/juju/go/testing"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

type MockStore struct {
	mux          *http.ServeMux
	lis          net.Listener
	bundleBytes  []byte
	bundleSha256 string
	downloads    []*charm.URL
}

func NewMockStore(c *C) *MockStore {
	s := &MockStore{}
	bytes, err := ioutil.ReadFile(testing.Charms.BundlePath(c.MkDir(), "dummy"))
	c.Assert(err, IsNil)
	s.bundleBytes = bytes
	h := sha256.New()
	h.Write(bytes)
	s.bundleSha256 = hex.EncodeToString(h.Sum(nil))
	s.mux = http.NewServeMux()
	s.mux.HandleFunc("/charm-info", func(w http.ResponseWriter, r *http.Request) {
		s.ServeInfo(w, r)
	})
	s.mux.HandleFunc("/charm/", func(w http.ResponseWriter, r *http.Request) {
		s.ServeCharm(w, r)
	})
	lis, err := net.Listen("tcp", "127.0.0.1:4444")
	c.Assert(err, IsNil)
	s.lis = lis
	go http.Serve(s.lis, s)
	return s
}

func (s *MockStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *MockStore) ServeInfo(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	response := map[string]*charm.InfoResponse{}
	for _, url := range r.Form["charms"] {
		cr := &charm.InfoResponse{}
		response[url] = cr
		curl := charm.MustParseURL(url)
		switch curl.Name {
		case "borken":
			cr.Errors = append(cr.Errors, "badness")
			continue
		case "unwise":
			cr.Warnings = append(cr.Warnings, "foolishness")
			fallthrough
		default:
			if curl.Revision == -1 {
				cr.Revision = 23
			} else {
				cr.Revision = curl.Revision
			}
			cr.Sha256 = s.bundleSha256
		}
	}
	data, err := json.Marshal(response)
	if err != nil {
		panic(err)
	}
	w.Header().Set("Content-Type", "application/json")
	_, err = w.Write(data)
	if err != nil {
		panic(err)
	}
}

func (s *MockStore) ServeCharm(w http.ResponseWriter, r *http.Request) {
	curl := charm.MustParseURL("cs:" + r.URL.Path[len("/charm/"):])
	s.downloads = append(s.downloads, curl)
	w.Header().Set("Connection", "close")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.Itoa(len(s.bundleBytes)))
	_, err := w.Write(s.bundleBytes)
	if err != nil {
		panic(err)
	}
}

type StoreSuite struct {
	server *MockStore
	store  charm.Repository
	cache  string
}

var _ = Suite(&StoreSuite{})

func (s *StoreSuite) SetUpSuite(c *C) {
	s.server = NewMockStore(c)
}

func (s *StoreSuite) SetUpTest(c *C) {
	s.cache = c.MkDir()
	s.store = charm.NewStore("http://127.0.0.1:4444", s.cache)
	s.server.downloads = nil
}

func (s *StoreSuite) TearDownSuite(c *C) {
	s.server.lis.Close()
}

func (s *StoreSuite) TestError(c *C) {
	curl := charm.MustParseURL("cs:series/borken")
	expect := `charm info errors for "cs:series/borken": badness`
	_, err := s.store.Latest(curl)
	c.Assert(err, ErrorMatches, expect)
	_, err = s.store.Get(curl)
	c.Assert(err, ErrorMatches, expect)
}

func (s *StoreSuite) TestWarning(c *C) {
	orig := log.Target
	log.Target = c
	defer func() { log.Target = orig }()
	curl := charm.MustParseURL("cs:series/unwise")
	expect := `.* JUJU WARNING: charm store reports for "cs:series/unwise": foolishness` + "\n"
	r, err := s.store.Latest(curl)
	c.Assert(r, Equals, 23)
	c.Assert(err, IsNil)
	c.Assert(c.GetTestLog(), Matches, expect)
	ch, err := s.store.Get(curl)
	c.Assert(ch, NotNil)
	c.Assert(err, IsNil)
	c.Assert(c.GetTestLog(), Matches, expect+expect)
}

func (s *StoreSuite) TestLatest(c *C) {
	for _, str := range []string{
		"cs:series/blah",
		"cs:series/blah-2",
		"cs:series/blah-99",
	} {
		r, err := s.store.Latest(charm.MustParseURL(str))
		c.Assert(r, Equals, 23)
		c.Assert(err, IsNil)
	}
}

func (s *StoreSuite) assertCached(c *C, curl *charm.URL) {
	s.server.downloads = nil
	ch, err := s.store.Get(curl)
	c.Assert(err, IsNil)
	c.Assert(ch, NotNil)
	c.Assert(s.server.downloads, IsNil)
}

func (s *StoreSuite) TestGetCacheImplicitRevision(c *C) {
	os.RemoveAll(s.cache)
	base := "cs:series/blah"
	curl := charm.MustParseURL(base)
	revCurl := charm.MustParseURL(base + "-23")
	ch, err := s.store.Get(curl)
	c.Assert(err, IsNil)
	c.Assert(ch, NotNil)
	c.Assert(s.server.downloads, DeepEquals, []*charm.URL{revCurl})
	s.assertCached(c, curl)
	s.assertCached(c, revCurl)
}

func (s *StoreSuite) TestGetCacheExplicitRevision(c *C) {
	os.RemoveAll(s.cache)
	base := "cs:series/blah-12"
	curl := charm.MustParseURL(base)
	ch, err := s.store.Get(curl)
	c.Assert(err, IsNil)
	c.Assert(ch, NotNil)
	c.Assert(s.server.downloads, DeepEquals, []*charm.URL{curl})
	s.assertCached(c, curl)
}

func (s *StoreSuite) TestGetBadCache(c *C) {
	base := "cs:series/blah"
	curl := charm.MustParseURL(base)
	revCurl := charm.MustParseURL(base + "-23")
	name := charm.Quote(revCurl.String()) + ".charm"
	err := ioutil.WriteFile(filepath.Join(s.cache, name), nil, 0666)
	c.Assert(err, IsNil)
	ch, err := s.store.Get(curl)
	c.Assert(err, IsNil)
	c.Assert(ch, NotNil)
	c.Assert(s.server.downloads, DeepEquals, []*charm.URL{revCurl})
	s.assertCached(c, curl)
	s.assertCached(c, revCurl)
}
