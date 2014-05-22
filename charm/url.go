// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"labix.org/v2/mgo/bson"

	"launchpad.net/juju-core/names"
)

// Location represents a charm location, which must declare a path component
// and a string representaion.
type Location interface {
	Path() string
	String() string
}

// Reference represents a charm location with an unresolved, untargeted series,
// such as:
//
//     cs:~joe/wordpress
//     cs:wordpress-42
type Reference struct {
	Schema   string // "cs" or "local"
	User     string // "joe"
	Name     string // "wordpress"
	Revision int    // -1 if unset, N otherwise
}

// URL represents a fully resolved charm location with a specific series, such
// as:
//
//     cs:~joe/oneiric/wordpress
//     cs:oneiric/wordpress-42
//     local:oneiric/wordpress
//
type URL struct {
	Reference
	Series string // "oneiric"
}

var ErrUnresolvedUrl error = fmt.Errorf("charm url series is not resolved")

var (
	validSeries = regexp.MustCompile("^[a-z]+([a-z0-9]+)?$")
	validName   = regexp.MustCompile("^[a-z][a-z0-9]*(-[a-z0-9]*[a-z][a-z0-9]*)*$")
)

// IsValidUser returns whether user is a valid username in charm URLs.
func IsValidUser(user string) bool {
	return names.IsUser(user)
}

// IsValidSeries returns whether series is a valid series in charm URLs.
func IsValidSeries(series string) bool {
	return validSeries.MatchString(series)
}

// IsValidName returns whether name is a valid charm name.
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
func ParseURL(url string) (*URL, error) {
	r, series, err := ParseReference(url)
	if err != nil {
		return nil, err
	}
	if series == "" {
		return nil, ErrUnresolvedUrl
	}
	return &URL{Reference: r, Series: series}, nil
}

// ParseReference parses the provided charm Reference string into its
// respective structure and the targeted series, if present.
func ParseReference(url string) (Reference, string, error) {
	r := Reference{Schema: "cs"}
	series := ""
	i := strings.Index(url, ":")
	if i >= 0 {
		r.Schema = url[:i]
		i++
	} else {
		i = 0
	}
	// cs: or local:
	if r.Schema != "cs" && r.Schema != "local" {
		return Reference{}, "", fmt.Errorf("charm URL has invalid schema: %q", url)
	}
	parts := strings.Split(url[i:], "/")
	if len(parts) < 1 || len(parts) > 3 {
		return Reference{}, "", fmt.Errorf("charm URL has invalid form: %q", url)
	}

	// ~<username>
	if strings.HasPrefix(parts[0], "~") {
		if r.Schema == "local" {
			return Reference{}, "", fmt.Errorf("local charm URL with user name: %q", url)
		}
		r.User = parts[0][1:]
		if !IsValidUser(r.User) {
			return Reference{}, "", fmt.Errorf("charm URL has invalid user name: %q", url)
		}
		parts = parts[1:]
	}

	// <series>
	if len(parts) == 2 {
		series = parts[0]
		if !IsValidSeries(series) {
			return Reference{}, "", fmt.Errorf("charm URL has invalid series: %q", url)
		}
		parts = parts[1:]
	}
	if len(parts) < 1 {
		return Reference{}, "", fmt.Errorf("charm URL without charm name: %q", url)
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
	if !IsValidName(r.Name) {
		return Reference{}, "", fmt.Errorf("charm URL has invalid charm name: %q", url)
	}
	return r, series, nil
}

// InferURL returns a charm URL inferred from src. The provided
// src may be a valid URL, in which case it is returned as-is,
// or it may be an alias in one of the following formats:
//
//    name
//    name-revision
//    series/name
//    series/name-revision
//    schema:name
//    schema:name-revision
//    cs:~user/name
//    cs:~user/name-revision
//
// The defaultSeries paramater is used to define the resulting URL
// when src does not include that information; similarly, a missing
// schema is assumed to be 'cs'.
func InferURL(src, defaultSeries string) (*URL, error) {
	r, series, err := ParseReference(src)
	if err != nil {
		return nil, err
	}
	if series != "" {
		return &URL{Reference: r, Series: series}, nil
	}
	if strings.HasPrefix(src, "~") {
		return nil, fmt.Errorf("cannot infer charm URL with user but no schema: %q", src)
	}
	orig := src
	schema := "cs"
	if i := strings.Index(src, ":"); i != -1 {
		schema, src = src[:i], src[i+1:]
	}
	var full string
	switch parts := strings.Split(src, "/"); len(parts) {
	case 1:
		if defaultSeries == "" {
			return nil, fmt.Errorf("cannot infer charm URL for %q: no series provided", orig)
		}
		full = fmt.Sprintf("%s:%s/%s", schema, defaultSeries, src)
	case 2:
		if strings.HasPrefix(parts[0], "~") {
			if defaultSeries == "" {
				return nil, fmt.Errorf("cannot infer charm URL for %q: no series provided", orig)
			}
			full = fmt.Sprintf("%s:%s/%s/%s", schema, parts[0], defaultSeries, parts[1])
		} else {
			full = fmt.Sprintf("%s:%s", schema, src)
		}
	default:
		full = fmt.Sprintf("%s:%s", schema, src)
	}
	u, err := ParseURL(full)
	if err != nil && orig != full {
		err = fmt.Errorf("%s (URL inferred from %q)", err, orig)
	}
	return u, err
}

func (u *URL) Path() string {
	return u.path(u.Series)
}

func (r Reference) path(series string) string {
	var parts []string
	if r.User != "" {
		parts = append(parts, fmt.Sprintf("~%s", r.User))
	}
	if series != "" {
		parts = append(parts, series)
	}
	if r.Revision >= 0 {
		parts = append(parts, fmt.Sprintf("%s-%d", r.Name, r.Revision))
	} else {
		parts = append(parts, r.Name)
	}
	return strings.Join(parts, "/")
}

func (r Reference) Path() string {
	return r.path("")
}

func (u *URL) String() string {
	return fmt.Sprintf("%s:%s", u.Schema, u.Path())
}

func (r Reference) String() string {
	return fmt.Sprintf("%s:%s", r.Schema, r.Path())
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

func (r *Reference) MarshalJSON() ([]byte, error) {
	return json.Marshal(r.String())
}

func (r *Reference) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	ref, _, err := ParseReference(s)
	if err != nil {
		return err
	}
	*r = ref
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
