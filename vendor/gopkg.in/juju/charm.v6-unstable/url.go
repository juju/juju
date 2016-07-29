// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm

import (
	"encoding/json"
	"fmt"
	gourl "net/url"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/juju/names.v2"
	"gopkg.in/mgo.v2/bson"
)

// Location represents a charm location, which must declare a path component
// and a string representaion.
type Location interface {
	Path() string
	String() string
}

// URL represents a charm or bundle location:
//
//     cs:~joe/oneiric/wordpress
//     cs:oneiric/wordpress-42
//     local:oneiric/wordpress
//     cs:~joe/wordpress
//     cs:wordpress
//     cs:precise/wordpress-20
//     cs:development/precise/wordpress-20
//     cs:~joe/development/wordpress
//
type URL struct {
	Schema   string  // "cs" or "local".
	User     string  // "joe".
	Name     string  // "wordpress".
	Revision int     // -1 if unset, N otherwise.
	Series   string  // "precise" or "" if unset; "bundle" if it's a bundle.
	Channel  Channel // "development" or "" if no channel.
}

var ErrUnresolvedUrl error = fmt.Errorf("charm or bundle url series is not resolved")

// Channel represents different stages in the development of a charm or bundle.
type Channel string

const (
	// DevelopmentChannel is the channel used for charms or bundles under
	// development.
	DevelopmentChannel Channel = "development"
)

var (
	validSeries = regexp.MustCompile("^[a-z]+([a-z0-9]+)?$")
	validName   = regexp.MustCompile("^[a-z][a-z0-9]*(-[a-z0-9]*[a-z][a-z0-9]*)*$")
)

// IsValidSeries reports whether series is a valid series in charm or bundle
// URLs.
func IsValidSeries(series string) bool {
	return validSeries.MatchString(series)
}

// IsValidChannel reports whether channel is a valid channel in charm or bundle
// URLs.
func IsValidChannel(channel Channel) bool {
	return channel == DevelopmentChannel
}

// IsValidName reports whether name is a valid charm or bundle name.
func IsValidName(name string) bool {
	return validName.MatchString(name)
}

// WithRevision returns a URL equivalent to url but with Revision set
// to revision.
func (url *URL) WithRevision(revision int) *URL {
	urlCopy := *url
	urlCopy.Revision = revision
	return &urlCopy
}

// WithChannel returns a URL equivalent to url but with the given channel.
func (url *URL) WithChannel(channel Channel) *URL {
	urlCopy := *url
	urlCopy.Channel = channel
	return &urlCopy
}

// MustParseURL works like ParseURL, but panics in case of errors.
func MustParseURL(url string) *URL {
	u, err := ParseURL(url)
	if err != nil {
		panic(err)
	}
	return u
}

// ParseURL parses the provided charm URL string into its respective
// structure.
//
// Additionally, fully-qualified charmstore URLs are supported; note that this
// currently assumes that they will map to jujucharms.com (that is,
// fully-qualified URLs currently map to the 'cs' schema):
//
//    https://jujucharms.com/name
//    https://jujucharms.com/name/series
//    https://jujucharms.com/name/revision
//    https://jujucharms.com/name/series/revision
//    https://jujucharms.com/u/user/name
//    https://jujucharms.com/u/user/name/series
//    https://jujucharms.com/u/user/name/revision
//    https://jujucharms.com/u/user/name/series/revision
//    https://jujucharms.com/channel/name
//    https://jujucharms.com/channel/name/series
//    https://jujucharms.com/channel/name/revision
//    https://jujucharms.com/channel/name/series/revision
//    https://jujucharms.com/u/user/channel/name
//    https://jujucharms.com/u/user/channel/name/series
//    https://jujucharms.com/u/user/channel/name/revision
//    https://jujucharms.com/u/user/channel/name/series/revision
//
// A missing schema is assumed to be 'cs'.
func ParseURL(url string) (*URL, error) {
	// Check if we're dealing with a v1 or v2 URL.
	u, err := gourl.Parse(url)
	if err != nil {
		return nil, fmt.Errorf("cannot parse charm or bundle URL: %q", url)
	}
	if u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return nil, fmt.Errorf("charm or bundle URL %q has unrecognized parts", url)
	}
	var curl *URL
	switch {
	case u.Opaque != "":
		// Shortcut old-style URLs.
		u.Path = u.Opaque
		curl, err = parseV1URL(u, url)
	case u.Scheme == "http" || u.Scheme == "https":
		// Shortcut new-style URLs.
		curl, err = parseV2URL(u)
	default:
		// TODO: for now, fall through to parsing v1 references; this will be
		// expanded to be more robust in the future.
		curl, err = parseV1URL(u, url)
	}
	if err != nil {
		return nil, err
	}
	if curl.Schema == "" {
		curl.Schema = "cs"
	}
	return curl, nil
}

func parseV1URL(url *gourl.URL, originalURL string) (*URL, error) {
	var r URL
	if url.Scheme != "" {
		r.Schema = url.Scheme
		if r.Schema != "cs" && r.Schema != "local" {
			return nil, fmt.Errorf("charm or bundle URL has invalid schema: %q", originalURL)
		}
	}
	i := 0
	parts := strings.Split(url.Path[i:], "/")
	if len(parts) < 1 || len(parts) > 4 {
		return nil, fmt.Errorf("charm or bundle URL has invalid form: %q", originalURL)
	}

	// ~<username>
	if strings.HasPrefix(parts[0], "~") {
		if r.Schema == "local" {
			return nil, fmt.Errorf("local charm or bundle URL with user name: %q", originalURL)
		}
		r.User, parts = parts[0][1:], parts[1:]
	}

	// <channel>
	if len(parts) > 1 {
		if IsValidChannel(Channel(parts[0])) {
			if r.Schema == "local" {
				return nil, fmt.Errorf("local charm or bundle URL with channel: %q", originalURL)
			}
			r.Channel, parts = Channel(parts[0]), parts[1:]
		}
	}

	if len(parts) > 2 {
		return nil, fmt.Errorf("charm or bundle URL has invalid form: %q", originalURL)
	}

	// <series>
	if len(parts) == 2 {
		r.Series, parts = parts[0], parts[1:]
		if !IsValidSeries(r.Series) {
			return nil, fmt.Errorf("charm or bundle URL has invalid series: %q", originalURL)
		}
	}
	if len(parts) < 1 {
		return nil, fmt.Errorf("URL without charm or bundle name: %q", originalURL)
	}

	// <name>[-<revision>]
	r.Name = parts[0]
	r.Revision = -1
	for i := len(r.Name) - 1; i > 0; i-- {
		c := r.Name[i]
		if c >= '0' && c <= '9' {
			continue
		}
		if c == '-' && i != len(r.Name)-1 {
			var err error
			r.Revision, err = strconv.Atoi(r.Name[i+1:])
			if err != nil {
				panic(err) // We just checked it was right.
			}
			r.Name = r.Name[:i]
		}
		break
	}
	if r.User != "" {
		if !names.IsValidUser(r.User) {
			return nil, fmt.Errorf("charm or bundle URL has invalid user name: %q", originalURL)
		}
	}
	if !IsValidName(r.Name) {
		return nil, fmt.Errorf("URL has invalid charm or bundle name: %q", originalURL)
	}
	return &r, nil
}

func parseV2URL(url *gourl.URL) (*URL, error) {
	var r URL
	r.Schema = "cs"
	parts := strings.Split(strings.Trim(url.Path, "/"), "/")
	if parts[0] == "u" {
		if len(parts) < 3 {
			return nil, fmt.Errorf(`charm or bundle URL %q malformed, expected "/u/<user>/<name>"`, url)
		}
		r.User, parts = parts[1], parts[2:]
	}
	if len(parts) > 1 && IsValidChannel(Channel(parts[0])) {
		r.Channel, parts = Channel(parts[0]), parts[1:]
	}
	r.Name, parts = parts[0], parts[1:]
	r.Revision = -1
	if len(parts) > 0 {
		revision, err := strconv.Atoi(parts[0])
		if err == nil {
			r.Revision = revision
		} else {
			r.Series = parts[0]
			if !IsValidSeries(r.Series) {
				return nil, fmt.Errorf("charm or bundle URL has invalid series: %q", url)
			}
			parts = parts[1:]
			if len(parts) == 1 {
				r.Revision, err = strconv.Atoi(parts[0])
				if err != nil {
					return nil, fmt.Errorf("charm or bundle URL has malformed revision: %q in %q", parts[0], url)
				}
			} else {
				if len(parts) != 0 {
					return nil, fmt.Errorf("charm or bundle URL has invalid form: %q", url)
				}
			}
		}
	}
	if r.User != "" {
		if !names.IsValidUser(r.User) {
			return nil, fmt.Errorf("charm or bundle URL has invalid user name: %q", url)
		}
	}
	if !IsValidName(r.Name) {
		return nil, fmt.Errorf("URL has invalid charm or bundle name: %q", url)
	}
	return &r, nil
}

func (r *URL) path() string {
	var parts []string
	if r.User != "" {
		parts = append(parts, fmt.Sprintf("~%s", r.User))
	}
	if r.Channel != "" {
		parts = append(parts, string(r.Channel))
	}
	if r.Series != "" {
		parts = append(parts, r.Series)
	}
	if r.Revision >= 0 {
		parts = append(parts, fmt.Sprintf("%s-%d", r.Name, r.Revision))
	} else {
		parts = append(parts, r.Name)
	}
	return strings.Join(parts, "/")
}

func (r URL) Path() string {
	return r.path()
}

// InferURL parses src as a reference, fills out the series in the
// returned URL using defaultSeries if necessary.
//
// This function is deprecated. New code should use ParseURL instead.
func InferURL(src, defaultSeries string) (*URL, error) {
	u, err := ParseURL(src)
	if err != nil {
		return nil, err
	}
	if u.Series == "" {
		if defaultSeries == "" {
			return nil, fmt.Errorf("cannot infer charm or bundle URL for %q: charm or bundle url series is not resolved", src)
		}
		u.Series = defaultSeries
	}
	return u, nil
}

func (u URL) String() string {
	return fmt.Sprintf("%s:%s", u.Schema, u.Path())
}

// GetBSON turns u into a bson.Getter so it can be saved directly
// on a MongoDB database with mgo.
func (u *URL) GetBSON() (interface{}, error) {
	if u == nil {
		return nil, nil
	}
	return u.String(), nil
}

// SetBSON turns u into a bson.Setter so it can be loaded directly
// from a MongoDB database with mgo.
func (u *URL) SetBSON(raw bson.Raw) error {
	if raw.Kind == 10 {
		return bson.SetZero
	}
	var s string
	err := raw.Unmarshal(&s)
	if err != nil {
		return err
	}
	url, err := ParseURL(s)
	if err != nil {
		return err
	}
	*u = *url
	return nil
}

func (u *URL) MarshalJSON() ([]byte, error) {
	if u == nil {
		panic("cannot marshal nil *charm.URL")
	}
	return json.Marshal(u.String())
}

func (u *URL) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	url, err := ParseURL(s)
	if err != nil {
		return err
	}
	*u = *url
	return nil
}

// MarshalText implements encoding.TextMarshaler by
// returning u.String()
func (u *URL) MarshalText() ([]byte, error) {
	if u == nil {
		return nil, nil
	}
	return []byte(u.String()), nil
}

// UnmarshalText implements encoding.TestUnmarshaler by
// parsing the data with ParseURL.
func (u *URL) UnmarshalText(data []byte) error {
	url, err := ParseURL(string(data))
	if err != nil {
		return err
	}
	*u = *url
	return nil
}

// Quote translates a charm url string into one which can be safely used
// in a file path.  ASCII letters, ASCII digits, dot and dash stay the
// same; other characters are translated to their hex representation
// surrounded by underscores.
func Quote(unsafe string) string {
	safe := make([]byte, 0, len(unsafe)*4)
	for i := 0; i < len(unsafe); i++ {
		b := unsafe[i]
		switch {
		case b >= 'a' && b <= 'z',
			b >= 'A' && b <= 'Z',
			b >= '0' && b <= '9',
			b == '.',
			b == '-':
			safe = append(safe, b)
		default:
			safe = append(safe, fmt.Sprintf("_%02x_", b)...)
		}
	}
	return string(safe)
}
