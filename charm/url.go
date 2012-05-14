package charm

import (
	"fmt"
	"launchpad.net/mgo/bson"
	"regexp"
	"strconv"
	"strings"
)

// A charm URL represents charm locations such as:
//
//     cs:~joe/oneiric/wordpress
//     cs:oneiric/wordpress-42
//     local:oneiric/wordpress
//
type URL struct {
	Schema   string // "cs" or "local"
	User     string // "joe"
	Series   string // "oneiric"
	Name     string // "wordpress"
	Revision int    // -1 if unset, N otherwise
}

// WithRevision returns a URL equivalent to url but with Revision set
// to revision.
func (url *URL) WithRevision(revision int) *URL {
	urlCopy := *url
	urlCopy.Revision = revision
	return &urlCopy
}

var validUser = regexp.MustCompile("^[a-z0-9][a-zA-Z0-9+.-]+$")
var validSeries = regexp.MustCompile("^[a-z]+([a-z-]+[a-z])?$")
var validName = regexp.MustCompile("^[a-z][a-z0-9]*(-[a-z0-9]*[a-z][a-z0-9]*)*$")

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
	u := &URL{}
	i := strings.Index(url, ":")
	if i > 0 {
		u.Schema = url[:i]
		i++
	}
	// cs: or local:
	if u.Schema != "cs" && u.Schema != "local" {
		return nil, fmt.Errorf("charm URL has invalid schema: %q", url)
	}
	parts := strings.Split(url[i:], "/")
	if len(parts) < 1 || len(parts) > 3 {
		return nil, fmt.Errorf("charm URL has invalid form: %q", url)
	}

	// ~<username>
	if strings.HasPrefix(parts[0], "~") {
		if u.Schema == "local" {
			return nil, fmt.Errorf("local charm URL with user name: %q", url)
		}
		u.User = parts[0][1:]
		if !validUser.MatchString(u.User) {
			return nil, fmt.Errorf("charm URL has invalid user name: %q", url)
		}
		parts = parts[1:]
	}

	// <series>
	if len(parts) < 2 {
		return nil, fmt.Errorf("charm URL without series: %q", url)
	}
	if len(parts) == 2 {
		u.Series = parts[0]
		if !validSeries.MatchString(u.Series) {
			return nil, fmt.Errorf("charm URL has invalid series: %q", url)
		}
		parts = parts[1:]
	}

	// <name>[-<revision>]
	u.Name = parts[0]
	u.Revision = -1
	for i := len(u.Name) - 1; i > 0; i-- {
		c := u.Name[i]
		if c >= '0' && c <= '9' {
			continue
		}
		if c == '-' && i != len(u.Name)-1 {
			var err error
			u.Revision, err = strconv.Atoi(u.Name[i+1:])
			if err != nil {
				panic(err) // We just checked it was right.
			}
			u.Name = u.Name[:i]
		}
		break
	}
	if !validName.MatchString(u.Name) {
		return nil, fmt.Errorf("charm URL has invalid charm name: %q", url)
	}
	return u, nil
}

// InferURL inserts default series and schema into the provided url string,
// where necessary, and attempts to parse the result.
func InferURL(src, defaultSeries string) (*URL, error) {
	if u, err := ParseURL(src); err == nil {
		// src was a valid charm URL already
		return u, nil
	}
	if strings.HasPrefix(src, "~") {
		return nil, fmt.Errorf("cannot infer charm URL with user but no schema: %q", src)
	}
	// infer schema if omitted
	var schema, rest string
	if i := strings.Index(src, ":"); i == -1 {
		schema = "cs"
		rest = src
	} else {
		schema = src[:i]
		rest = src[i+1:]
	}
	// construct full url string, inferring series if omitted
	full := fmt.Sprintf("%s:%s", schema, rest)
	switch parts := strings.Split(rest, "/"); len(parts) {
	case 1:
		full = fmt.Sprintf("%s:%s/%s", schema, defaultSeries, rest)
	case 2:
		if strings.HasPrefix(parts[0], "~") {
			full = fmt.Sprintf("%s:%s/%s/%s", schema, parts[0], defaultSeries, parts[1])
		}
	}
	u, err := ParseURL(full)
	if err != nil {
		if src != full {
			err = fmt.Errorf("%s (URL inferred from %q)", err, src)
		}
	}
	return u, err
}

func (u *URL) String() string {
	if u.User != "" {
		if u.Revision >= 0 {
			return fmt.Sprintf("%s:~%s/%s/%s-%d", u.Schema, u.User, u.Series, u.Name, u.Revision)
		}
		return fmt.Sprintf("%s:~%s/%s/%s", u.Schema, u.User, u.Series, u.Name)
	}
	if u.Revision >= 0 {
		return fmt.Sprintf("%s:%s/%s-%d", u.Schema, u.Series, u.Name, u.Revision)
	}
	return fmt.Sprintf("%s:%s/%s", u.Schema, u.Series, u.Name)
}

// GetBSON turns u into a bson.Getter so it can be saved directly
// on a MongoDB database with mgo.
func (u *URL) GetBSON() (interface{}, error) {
	return u.String(), nil
}

// SetBSON turns u into a bson.Setter so it can be loaded directly
// from a MongoDB database with mgo.
func (u *URL) SetBSON(raw bson.Raw) error {
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
