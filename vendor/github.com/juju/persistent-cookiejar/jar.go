// Copyright 2012 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package cookiejar implements an in-memory RFC 6265-compliant http.CookieJar.
//
// This implementation is a fork of net/http/cookiejar which also
// implements methods for dumping the cookies to persistent
// storage and retrieving them.
package cookiejar

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/publicsuffix"
	"gopkg.in/errgo.v1"
)

// PublicSuffixList provides the public suffix of a domain. For example:
//      - the public suffix of "example.com" is "com",
//      - the public suffix of "foo1.foo2.foo3.co.uk" is "co.uk", and
//      - the public suffix of "bar.pvt.k12.ma.us" is "pvt.k12.ma.us".
//
// Implementations of PublicSuffixList must be safe for concurrent use by
// multiple goroutines.
//
// An implementation that always returns "" is valid and may be useful for
// testing but it is not secure: it means that the HTTP server for foo.com can
// set a cookie for bar.com.
//
// A public suffix list implementation is in the package
// golang.org/x/net/publicsuffix.
type PublicSuffixList interface {
	// PublicSuffix returns the public suffix of domain.
	//
	// TODO: specify which of the caller and callee is responsible for IP
	// addresses, for leading and trailing dots, for case sensitivity, and
	// for IDN/Punycode.
	PublicSuffix(domain string) string

	// String returns a description of the source of this public suffix
	// list. The description will typically contain something like a time
	// stamp or version number.
	String() string
}

// Options are the options for creating a new Jar.
type Options struct {
	// PublicSuffixList is the public suffix list that determines whether
	// an HTTP server can set a cookie for a domain.
	//
	// If this is nil, the public suffix list implementation in golang.org/x/net/publicsuffix
	// is used.
	PublicSuffixList PublicSuffixList

	// Filename holds the file to use for storage of the cookies.
	// If it is empty, the value of DefaultCookieFile will be used.
	Filename string

	// NoPersist specifies whether no persistence should be used
	// (useful for tests). If this is true, the value of Filename will be
	// ignored.
	NoPersist bool
}

// Jar implements the http.CookieJar interface from the net/http package.
type Jar struct {
	// filename holds the file that the cookies were loaded from.
	filename string

	psList PublicSuffixList

	// mu locks the remaining fields.
	mu sync.Mutex

	// entries is a set of entries, keyed by their eTLD+1 and subkeyed by
	// their name/domain/path.
	entries map[string]map[string]entry
}

var noOptions Options

// New returns a new cookie jar. A nil *Options is equivalent to a zero
// Options.
//
// New will return an error if the cookies could not be loaded
// from the file for any reason than if the file does not exist.
func New(o *Options) (*Jar, error) {
	return newAtTime(o, time.Now())
}

// newAtTime is like New but takes the current time as a parameter.
func newAtTime(o *Options, now time.Time) (*Jar, error) {
	jar := &Jar{
		entries: make(map[string]map[string]entry),
	}
	if o == nil {
		o = &noOptions
	}
	if jar.psList = o.PublicSuffixList; jar.psList == nil {
		jar.psList = publicsuffix.List
	}
	if !o.NoPersist {
		if jar.filename = o.Filename; jar.filename == "" {
			jar.filename = DefaultCookieFile()
		}
		if err := jar.load(); err != nil {
			return nil, errgo.Notef(err, "cannot load cookies")
		}
	}
	jar.deleteExpired(now)
	return jar, nil
}

// homeDir returns the OS-specific home path as specified in the environment.
func homeDir() string {
	if runtime.GOOS == "windows" {
		return filepath.Join(os.Getenv("HOMEDRIVE"), os.Getenv("HOMEPATH"))
	}
	return os.Getenv("HOME")
}

// entry is the internal representation of a cookie.
//
// This struct type is not used outside of this package per se, but the exported
// fields are those of RFC 6265.
// Note that this structure is marshaled to JSON, so backward-compatibility
// should be preserved.
type entry struct {
	Name       string
	Value      string
	Domain     string
	Path       string
	Secure     bool
	HttpOnly   bool
	Persistent bool
	HostOnly   bool
	Expires    time.Time
	Creation   time.Time
	LastAccess time.Time

	// Updated records when the cookie was updated.
	// This is different from creation time because a cookie
	// can be changed without updating the creation time.
	Updated time.Time

	// CanonicalHost stores the original canonical host name
	// that the cookie was associated with. We store this
	// so that even if the public suffix list changes (for example
	// when storing/loading cookies) we can still get the correct
	// jar keys.
	CanonicalHost string
}

// id returns the domain;path;name triple of e as an id.
func (e *entry) id() string {
	return id(e.Domain, e.Path, e.Name)
}

// id returns the domain;path;name triple as an id.
func id(domain, path, name string) string {
	return fmt.Sprintf("%s;%s;%s", domain, path, name)
}

// shouldSend determines whether e's cookie qualifies to be included in a
// request to host/path. It is the caller's responsibility to check if the
// cookie is expired.
func (e *entry) shouldSend(https bool, host, path string) bool {
	return e.domainMatch(host) && e.pathMatch(path) && (https || !e.Secure)
}

// domainMatch implements "domain-match" of RFC 6265 section 5.1.3.
func (e *entry) domainMatch(host string) bool {
	if e.Domain == host {
		return true
	}
	return !e.HostOnly && hasDotSuffix(host, e.Domain)
}

// pathMatch implements "path-match" according to RFC 6265 section 5.1.4.
func (e *entry) pathMatch(requestPath string) bool {
	if requestPath == e.Path {
		return true
	}
	if strings.HasPrefix(requestPath, e.Path) {
		if e.Path[len(e.Path)-1] == '/' {
			return true // The "/any/" matches "/any/path" case.
		} else if requestPath[len(e.Path)] == '/' {
			return true // The "/any" matches "/any/path" case.
		}
	}
	return false
}

// hasDotSuffix reports whether s ends in "."+suffix.
func hasDotSuffix(s, suffix string) bool {
	return len(s) > len(suffix) && s[len(s)-len(suffix)-1] == '.' && s[len(s)-len(suffix):] == suffix
}

type byCanonicalHost struct {
	byPathLength
}

func (s byCanonicalHost) Less(i, j int) bool {
	e0, e1 := &s.byPathLength[i], &s.byPathLength[j]
	if e0.CanonicalHost != e1.CanonicalHost {
		return e0.CanonicalHost < e1.CanonicalHost
	}
	return s.byPathLength.Less(i, j)
}

// byPathLength is a []entry sort.Interface that sorts according to RFC 6265
// section 5.4 point 2: by longest path and then by earliest creation time.
type byPathLength []entry

func (s byPathLength) Len() int { return len(s) }

func (s byPathLength) Less(i, j int) bool {
	e0, e1 := &s[i], &s[j]
	if len(e0.Path) != len(e1.Path) {
		return len(e0.Path) > len(e1.Path)
	}
	if !e0.Creation.Equal(e1.Creation) {
		return e0.Creation.Before(e1.Creation)
	}
	// The following are not strictly necessary
	// but are useful for providing deterministic
	// behaviour in tests.
	if e0.Name != e1.Name {
		return e0.Name < e1.Name
	}
	return e0.Value < e1.Value
}

func (s byPathLength) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

// Cookies implements the Cookies method of the http.CookieJar interface.
//
// It returns an empty slice if the URL's scheme is not HTTP or HTTPS.
func (j *Jar) Cookies(u *url.URL) (cookies []*http.Cookie) {
	return j.cookies(u, time.Now())
}

// cookies is like Cookies but takes the current time as a parameter.
func (j *Jar) cookies(u *url.URL, now time.Time) (cookies []*http.Cookie) {
	if u.Scheme != "http" && u.Scheme != "https" {
		return cookies
	}
	host, err := canonicalHost(u.Host)
	if err != nil {
		return cookies
	}
	key := jarKey(host, j.psList)

	j.mu.Lock()
	defer j.mu.Unlock()

	submap := j.entries[key]
	if submap == nil {
		return cookies
	}

	https := u.Scheme == "https"
	path := u.Path
	if path == "" {
		path = "/"
	}

	var selected []entry
	for id, e := range submap {
		if !e.Expires.After(now) {
			// Save some space by deleting the value when the cookie
			// expires. We can't delete the cookie itself because then
			// we wouldn't know that the cookie had expired when
			// we merge with another cookie jar.
			if e.Value != "" {
				e.Value = ""
				submap[id] = e
			}
			continue
		}
		if !e.shouldSend(https, host, path) {
			continue
		}
		e.LastAccess = now
		submap[id] = e
		selected = append(selected, e)
	}

	sort.Sort(byPathLength(selected))
	for _, e := range selected {
		cookies = append(cookies, &http.Cookie{Name: e.Name, Value: e.Value})
	}

	return cookies
}

// AllCookies returns all cookies in the jar. The returned cookies will
// have Domain, Expires, HttpOnly, Name, Secure, Path, and Value filled
// out. Expired cookies will not be returned. This function does not
// modify the cookie jar.
func (j *Jar) AllCookies() (cookies []*http.Cookie) {
	return j.allCookies(time.Now())
}

// allCookies is like AllCookies but takes the current time as a parameter.
func (j *Jar) allCookies(now time.Time) []*http.Cookie {
	var selected []entry
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, submap := range j.entries {
		for _, e := range submap {
			if !e.Expires.After(now) {
				// Do not return expired cookies.
				continue
			}
			selected = append(selected, e)
		}
	}

	sort.Sort(byCanonicalHost{byPathLength(selected)})
	cookies := make([]*http.Cookie, len(selected))
	for i, e := range selected {
		// Note: The returned cookies do not contain sufficient
		// information to recreate the database.
		cookies[i] = &http.Cookie{
			Name:     e.Name,
			Value:    e.Value,
			Path:     e.Path,
			Domain:   e.Domain,
			Expires:  e.Expires,
			Secure:   e.Secure,
			HttpOnly: e.HttpOnly,
		}
	}

	return cookies
}

// RemoveCookie removes the cookie matching the name, domain and path
// specified by c.
func (j *Jar) RemoveCookie(c *http.Cookie) {
	j.mu.Lock()
	defer j.mu.Unlock()
	id := id(c.Domain, c.Path, c.Name)
	key := jarKey(c.Domain, j.psList)
	if e, ok := j.entries[key][id]; ok {
		e.Value = ""
		e.Expires = time.Now().Add(-1 * time.Second)
		j.entries[key][id] = e
	}
}

// merge merges all the given entries into j. More recently changed
// cookies take precedence over older ones.
func (j *Jar) merge(entries []entry) {
	for _, e := range entries {
		if e.CanonicalHost == "" {
			continue
		}
		key := jarKey(e.CanonicalHost, j.psList)
		id := e.id()
		submap := j.entries[key]
		if submap == nil {
			j.entries[key] = map[string]entry{
				id: e,
			}
			continue
		}
		oldEntry, ok := submap[id]
		if !ok || e.Updated.After(oldEntry.Updated) {
			submap[id] = e
		}
	}
}

var expiryRemovalDuration = 24 * time.Hour

// deleteExpired deletes all entries that have expired for long enough
// that we can actually expect there to be no external copies of it that
// might resurrect the dead cookie.
func (j *Jar) deleteExpired(now time.Time) {
	for tld, submap := range j.entries {
		for id, e := range submap {
			if !e.Expires.After(now) && !e.Updated.Add(expiryRemovalDuration).After(now) {
				delete(submap, id)
			}
		}
		if len(submap) == 0 {
			delete(j.entries, tld)
		}
	}
}

// RemoveAllHost removes any cookies from the jar that were set for the given host.
func (j *Jar) RemoveAllHost(host string) {
	host, err := canonicalHost(host)
	if err != nil {
		return
	}
	key := jarKey(host, j.psList)

	j.mu.Lock()
	defer j.mu.Unlock()

	expired := time.Now().Add(-1 * time.Second)
	submap := j.entries[key]
	for id, e := range submap {
		if e.CanonicalHost == host {
			// Save some space by deleting the value when the cookie
			// expires. We can't delete the cookie itself because then
			// we wouldn't know that the cookie had expired when
			// we merge with another cookie jar.
			e.Value = ""
			e.Expires = expired
			submap[id] = e
		}
	}
}

// RemoveAll removes all the cookies from the jar.
func (j *Jar) RemoveAll() {
	expired := time.Now().Add(-1 * time.Second)
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, submap := range j.entries {
		for id, e := range submap {
			// Save some space by deleting the value when the cookie
			// expires. We can't delete the cookie itself because then
			// we wouldn't know that the cookie had expired when
			// we merge with another cookie jar.
			e.Value = ""
			e.Expires = expired
			submap[id] = e
		}
	}
}

// SetCookies implements the SetCookies method of the http.CookieJar interface.
//
// It does nothing if the URL's scheme is not HTTP or HTTPS.
func (j *Jar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.setCookies(u, cookies, time.Now())
}

// setCookies is like SetCookies but takes the current time as parameter.
func (j *Jar) setCookies(u *url.URL, cookies []*http.Cookie, now time.Time) {
	if len(cookies) == 0 {
		return
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		// TODO is this really correct? It might be nice to send
		// cookies to websocket connections, for example.
		return
	}
	host, err := canonicalHost(u.Host)
	if err != nil {
		return
	}
	key := jarKey(host, j.psList)
	defPath := defaultPath(u.Path)

	j.mu.Lock()
	defer j.mu.Unlock()

	submap := j.entries[key]
	for _, cookie := range cookies {
		e, err := j.newEntry(cookie, now, defPath, host)
		if err != nil {
			continue
		}
		e.CanonicalHost = host
		id := e.id()
		if submap == nil {
			submap = make(map[string]entry)
			j.entries[key] = submap
		}
		if old, ok := submap[id]; ok {
			e.Creation = old.Creation
		} else {
			e.Creation = now
		}
		e.Updated = now
		e.LastAccess = now
		submap[id] = e
	}
}

// canonicalHost strips port from host if present and returns the canonicalized
// host name.
func canonicalHost(host string) (string, error) {
	var err error
	host = strings.ToLower(host)
	if hasPort(host) {
		host, _, err = net.SplitHostPort(host)
		if err != nil {
			return "", err
		}
	}
	if strings.HasSuffix(host, ".") {
		// Strip trailing dot from fully qualified domain names.
		host = host[:len(host)-1]
	}
	return toASCII(host)
}

// hasPort reports whether host contains a port number. host may be a host
// name, an IPv4 or an IPv6 address.
func hasPort(host string) bool {
	colons := strings.Count(host, ":")
	if colons == 0 {
		return false
	}
	if colons == 1 {
		return true
	}
	return host[0] == '[' && strings.Contains(host, "]:")
}

// jarKey returns the key to use for a jar.
func jarKey(host string, psl PublicSuffixList) string {
	if isIP(host) {
		return host
	}

	var i int
	if psl == nil {
		i = strings.LastIndex(host, ".")
		if i == -1 {
			return host
		}
	} else {
		suffix := psl.PublicSuffix(host)
		if suffix == host {
			return host
		}
		i = len(host) - len(suffix)
		if i <= 0 || host[i-1] != '.' {
			// The provided public suffix list psl is broken.
			// Storing cookies under host is a safe stopgap.
			return host
		}
	}
	prevDot := strings.LastIndex(host[:i-1], ".")
	return host[prevDot+1:]
}

// isIP reports whether host is an IP address.
func isIP(host string) bool {
	return net.ParseIP(host) != nil
}

// defaultPath returns the directory part of an URL's path according to
// RFC 6265 section 5.1.4.
func defaultPath(path string) string {
	if len(path) == 0 || path[0] != '/' {
		return "/" // Path is empty or malformed.
	}

	i := strings.LastIndex(path, "/") // Path starts with "/", so i != -1.
	if i == 0 {
		return "/" // Path has the form "/abc".
	}
	return path[:i] // Path is either of form "/abc/xyz" or "/abc/xyz/".
}

// newEntry creates an entry from a http.Cookie c. now is the current
// time and is compared to c.Expires to determine deletion of c. defPath
// and host are the default-path and the canonical host name of the URL
// c was received from.
//
// The returned entry should be removed if its expiry time is in the
// past. In this case, e may be incomplete, but it will be valid to call
// e.id (which depends on e's Name, Domain and Path).
//
// A malformed c.Domain will result in an error.
func (j *Jar) newEntry(c *http.Cookie, now time.Time, defPath, host string) (e entry, err error) {
	e.Name = c.Name
	if c.Path == "" || c.Path[0] != '/' {
		e.Path = defPath
	} else {
		e.Path = c.Path
	}

	e.Domain, e.HostOnly, err = j.domainAndType(host, c.Domain)
	if err != nil {
		return e, err
	}
	// MaxAge takes precedence over Expires.
	if c.MaxAge != 0 {
		e.Persistent = true
		e.Expires = now.Add(time.Duration(c.MaxAge) * time.Second)
		if c.MaxAge < 0 {
			return e, nil
		}
	} else if c.Expires.IsZero() {
		e.Expires = endOfTime
	} else {
		e.Persistent = true
		e.Expires = c.Expires
		if !c.Expires.After(now) {
			return e, nil
		}
	}

	e.Value = c.Value
	e.Secure = c.Secure
	e.HttpOnly = c.HttpOnly

	return e, nil
}

var (
	errIllegalDomain   = errors.New("cookiejar: illegal cookie domain attribute")
	errMalformedDomain = errors.New("cookiejar: malformed cookie domain attribute")
	errNoHostname      = errors.New("cookiejar: no host name available (IP only)")
)

// endOfTime is the time when session (non-persistent) cookies expire.
// This instant is representable in most date/time formats (not just
// Go's time.Time) and should be far enough in the future.
var endOfTime = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)

// domainAndType determines the cookie's domain and hostOnly attribute.
func (j *Jar) domainAndType(host, domain string) (string, bool, error) {
	if domain == "" {
		// No domain attribute in the SetCookie header indicates a
		// host cookie.
		return host, true, nil
	}

	if isIP(host) {
		// According to RFC 6265 domain-matching includes not being
		// an IP address.
		// TODO: This might be relaxed as in common browsers.
		return "", false, errNoHostname
	}

	// From here on: If the cookie is valid, it is a domain cookie (with
	// the one exception of a public suffix below).
	// See RFC 6265 section 5.2.3.
	if domain[0] == '.' {
		domain = domain[1:]
	}

	if len(domain) == 0 || domain[0] == '.' {
		// Received either "Domain=." or "Domain=..some.thing",
		// both are illegal.
		return "", false, errMalformedDomain
	}
	domain = strings.ToLower(domain)

	if domain[len(domain)-1] == '.' {
		// We received stuff like "Domain=www.example.com.".
		// Browsers do handle such stuff (actually differently) but
		// RFC 6265 seems to be clear here (e.g. section 4.1.2.3) in
		// requiring a reject.  4.1.2.3 is not normative, but
		// "Domain Matching" (5.1.3) and "Canonicalized Host Names"
		// (5.1.2) are.
		return "", false, errMalformedDomain
	}

	// See RFC 6265 section 5.3 #5.
	if j.psList != nil {
		if ps := j.psList.PublicSuffix(domain); ps != "" && !hasDotSuffix(domain, ps) {
			if host == domain {
				// This is the one exception in which a cookie
				// with a domain attribute is a host cookie.
				return host, true, nil
			}
			return "", false, errIllegalDomain
		}
	}

	// The domain must domain-match host: www.mycompany.com cannot
	// set cookies for .ourcompetitors.com.
	if host != domain && !hasDotSuffix(host, domain) {
		return "", false, errIllegalDomain
	}

	return domain, false, nil
}

// DefaultCookieFile returns the default cookie file to use
// for persisting cookie data.
// The following names will be used in decending order of preference:
//	- the value of the $GOCOOKIES environment variable.
//	- $HOME/.go-cookies
func DefaultCookieFile() string {
	if f := os.Getenv("GOCOOKIES"); f != "" {
		return f
	}
	return filepath.Join(homeDir(), ".go-cookies")
}
