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

	"github.com/juju/errors"
	"github.com/juju/mgo/v3/bson"
	"github.com/juju/utils/v4/arch"
)

// Schema represents the different types of valid schemas.
type Schema string

const (
	// Local represents a local charm URL, describes as a file system path.
	Local Schema = "local"

	// CharmHub schema represents the charmhub charm repository.
	CharmHub Schema = "ch"
)

// Prefix creates a url with the given prefix, useful for typed schemas.
func (s Schema) Prefix(url string) string {
	return fmt.Sprintf("%s:%s", s, url)
}

// Matches attempts to compare if a schema string matches the schema.
func (s Schema) Matches(other string) bool {
	return string(s) == other
}

func (s Schema) String() string {
	return string(s)
}

// Location represents a charm location, which must declare a path component
// and a string representation.
type Location interface {
	Path() string
	String() string
}

// URL represents a charm or bundle location:
//
//	local:oneiric/wordpress
//	ch:wordpress
//	ch:amd64/jammy/wordpress-30
type URL struct {
	Schema       string // "ch" or "local".
	Name         string // "wordpress".
	Revision     int    // -1 if unset, N otherwise.
	Series       string // "precise" or "" if unset; "bundle" if it's a bundle.
	Architecture string // "amd64" or "" if unset for charmstore (v1) URLs.
}

var (
	validArch   = regexp.MustCompile("^[a-z]+([a-z0-9]+)?$")
	validSeries = regexp.MustCompile("^[a-z]+([a-z0-9]+)?$")
	validName   = regexp.MustCompile("^[a-z][a-z0-9]*(-[a-z0-9]*[a-z][a-z0-9]*)*$")
)

// ValidateSchema returns an error if the schema is invalid.
//
// Valid schemas for the URL are:
// - ch: charm hub
// - local: local file

func ValidateSchema(schema string) error {
	switch schema {
	case CharmHub.String(), Local.String():
		return nil
	}
	return errors.NotValidf("schema %q", schema)
}

// IsValidSeries reports whether series is a valid series in charm or bundle
// URLs.
func IsValidSeries(series string) bool {
	return validSeries.MatchString(series)
}

// ValidateSeries returns an error if the given series is invalid.
func ValidateSeries(series string) error {
	if IsValidSeries(series) {
		return nil
	}
	return errors.NotValidf("series name %q", series)
}

// IsValidArchitecture reports whether the architecture is a valid architecture
// in charm or bundle URLs.
func IsValidArchitecture(architecture string) bool {
	return validArch.MatchString(architecture) && arch.IsSupportedArch(architecture)
}

// ValidateArchitecture returns an error if the given architecture is invalid.
func ValidateArchitecture(arch string) error {
	if IsValidArchitecture(arch) {
		return nil
	}
	return errors.NotValidf("architecture name %q", arch)
}

// IsValidName reports whether name is a valid charm or bundle name.
func IsValidName(name string) bool {
	return validName.MatchString(name)
}

// ValidateName returns an error if the given name is invalid.
func ValidateName(name string) error {
	if IsValidName(name) {
		return nil
	}
	return errors.NotValidf("name %q", name)
}

// WithRevision returns a URL equivalent to url but with Revision set
// to revision.
func (u *URL) WithRevision(revision int) *URL {
	urlCopy := *u
	urlCopy.Revision = revision
	return &urlCopy
}

// WithArchitecture returns a URL equivalent to url but with Architecture set
// to architecture.
func (u *URL) WithArchitecture(arch string) *URL {
	urlCopy := *u
	urlCopy.Architecture = arch
	return &urlCopy
}

// WithSeries returns a URL equivalent to url but with Series set
// to series.
func (u *URL) WithSeries(series string) *URL {
	urlCopy := *u
	urlCopy.Series = series
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
// A missing schema is assumed to be 'ch'.
func ParseURL(url string) (*URL, error) {
	u, err := gourl.Parse(url)
	if err != nil {
		return nil, errors.Errorf("cannot parse charm or bundle URL: %q", url)
	}
	if u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return nil, errors.Errorf("charm or bundle URL %q has unrecognized parts", url)
	}
	var curl *URL
	switch {
	case CharmHub.Matches(u.Scheme):
		// Handle talking to the new style of the schema.
		curl, err = parseCharmhubURL(u)
	case u.Opaque != "":
		u.Path = u.Opaque
		curl, err = parseLocalURL(u, url)
	default:
		// Handle the fact that anything without a prefix is now a CharmHub
		// charm URL.
		curl, err = parseCharmhubURL(u)
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	if curl.Schema == "" {
		return nil, errors.Errorf("expected schema for charm or bundle URL: %q", url)
	}
	return curl, nil
}

func parseLocalURL(url *gourl.URL, originalURL string) (*URL, error) {
	if !Local.Matches(url.Scheme) {
		return nil, errors.NotValidf("cannot parse URL %q: schema %q", url, url.Scheme)
	}
	r := URL{Schema: Local.String()}

	parts := strings.Split(url.Path[0:], "/")
	if len(parts) < 1 || len(parts) > 4 {
		return nil, errors.Errorf("charm or bundle URL has invalid form: %q", originalURL)
	}

	// ~<username>
	if strings.HasPrefix(parts[0], "~") {
		return nil, errors.Errorf("local charm or bundle URL with user name: %q", originalURL)
	}

	if len(parts) > 2 {
		return nil, errors.Errorf("charm or bundle URL has invalid form: %q", originalURL)
	}

	// <series>
	if len(parts) == 2 {
		r.Series, parts = parts[0], parts[1:]
		if err := ValidateSeries(r.Series); err != nil {
			return nil, errors.Annotatef(err, "cannot parse URL %q", originalURL)
		}
	}
	if len(parts) < 1 {
		return nil, errors.Errorf("URL without charm or bundle name: %q", originalURL)
	}

	// <name>[-<revision>]
	r.Name, r.Revision = extractRevision(parts[0])
	if err := ValidateName(r.Name); err != nil {
		return nil, errors.Annotatef(err, "cannot parse URL %q", url)
	}
	return &r, nil
}

func (u *URL) path() string {
	var parts []string
	if u.Architecture != "" {
		parts = append(parts, u.Architecture)
	}
	if u.Series != "" {
		parts = append(parts, u.Series)
	}
	if u.Revision >= 0 {
		parts = append(parts, fmt.Sprintf("%s-%d", u.Name, u.Revision))
	} else {
		parts = append(parts, u.Name)
	}
	return strings.Join(parts, "/")
}

// FullPath returns the full path of a URL path including the schema.
func (u *URL) FullPath() string {
	return fmt.Sprintf("%s:%s", u.Schema, u.Path())
}

// Path returns the path of the URL without the schema.
func (u *URL) Path() string {
	return u.path()
}

// String returns the string representation of the URL.
func (u *URL) String() string {
	return u.FullPath()
}

// GetBSON turns u into a bson.Getter so it can be saved directly
// on a MongoDB database with mgo.
//
// TODO (stickupkid): This should not be here, as this is purely for mongo
// data stores and that should be implemented at the site of data store, not
// dependant on the library.
func (u *URL) GetBSON() (interface{}, error) {
	if u == nil {
		return nil, nil
	}
	return u.String(), nil
}

// SetBSON turns u into a bson.Setter so it can be loaded directly
// from a MongoDB database with mgo.
//
// TODO (stickupkid): This should not be here, as this is purely for mongo
// data stores and that should be implemented at the site of data store, not
// dependant on the library.
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

// MarshalJSON will marshal the URL into a slice of bytes in a JSON
// representation.
func (u *URL) MarshalJSON() ([]byte, error) {
	if u == nil {
		panic("cannot marshal nil *charm.URL")
	}
	return json.Marshal(u.FullPath())
}

// UnmarshalJSON will unmarshal the URL from a JSON representation.
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
// returning u.FullPath()
func (u *URL) MarshalText() ([]byte, error) {
	if u == nil {
		return nil, nil
	}
	return []byte(u.FullPath()), nil
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

// parseCharmhubURL will attempt to parse an identifier URL. The identifier
// URL is split up into 3 parts, some of which are optional and some are
// mandatory.
//
//   - architecture (optional)
//   - series (optional)
//   - name
//   - revision (optional)
//
// Examples are as follows:
//
//   - ch:amd64/foo-1
//   - ch:amd64/focal/foo-1
//   - ch:foo-1
//   - ch:foo
//   - ch:amd64/focal/foo
func parseCharmhubURL(url *gourl.URL) (*URL, error) {
	r := URL{
		Schema:   CharmHub.String(),
		Revision: -1,
	}

	path := url.Path
	if url.Opaque != "" {
		path = url.Opaque
	}

	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || len(parts) > 3 {
		return nil, errors.Errorf(`charm or bundle URL %q malformed`, url)
	}

	// ~<username>
	if strings.HasPrefix(parts[0], "~") {
		return nil, errors.NotValidf("charmhub charm or bundle URL with user name: %q", url)
	}

	var nameRev string
	switch len(parts) {
	case 3:
		r.Architecture, r.Series, nameRev = parts[0], parts[1], parts[2]

		if err := ValidateArchitecture(r.Architecture); err != nil {
			return nil, errors.Annotatef(err, "in URL %q", url)
		}
	case 2:
		// Since both the architecture and series are optional,
		// the first part can be either architecture or series.
		// To differentiate between them, we go ahead and try to
		// validate the first part as an architecture to decide.

		if err := ValidateArchitecture(parts[0]); err == nil {
			r.Architecture, nameRev = parts[0], parts[1]
		} else {
			r.Series, nameRev = parts[0], parts[1]
		}

	default:
		nameRev = parts[0]
	}

	// Mandatory
	r.Name, r.Revision = extractRevision(nameRev)
	if err := ValidateName(r.Name); err != nil {
		return nil, errors.Annotatef(err, "cannot parse name and/or revision in URL %q", url)
	}

	// Optional
	if r.Series != "" {
		if err := ValidateSeries(r.Series); err != nil {
			return nil, errors.Annotatef(err, "in URL %q", url)
		}
	}

	return &r, nil
}

// EnsureSchema will ensure that the scheme for a given URL is correct and
// valid. If the url does not specify a schema, the provided defaultSchema
// will be injected to it.
func EnsureSchema(url string, defaultSchema Schema) (string, error) {
	u, err := gourl.Parse(url)
	if err != nil {
		return "", errors.Errorf("cannot parse charm or bundle URL: %q", url)
	}
	switch Schema(u.Scheme) {
	case CharmHub, Local:
		return url, nil
	case Schema(""):
		// If the schema is empty, we fall back to the default schema.
		return defaultSchema.Prefix(url), nil
	default:
		return "", errors.NotValidf("schema %q", u.Scheme)
	}
}

func extractRevision(name string) (string, int) {
	revision := -1
	for i := len(name) - 1; i > 0; i-- {
		c := name[i]
		if c >= '0' && c <= '9' {
			continue
		}
		if c == '-' && i != len(name)-1 {
			var err error
			revision, err = strconv.Atoi(name[i+1:])
			if err != nil {
				panic(err) // We just checked it was right.
			}
			name = name[:i]
		}
		break
	}
	return name, revision
}
