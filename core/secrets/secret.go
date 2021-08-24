// Copyright 2021 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package secrets

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/juju/errors"
)

// SecretType is the type of a secret.
// This is used when creating a secret.
type SecretType string

const (
	TypeBlob     = SecretType("blob")
	TypePassword = SecretType("password")
)

var validTypes = map[SecretType]bool{
	TypeBlob:     true,
	TypePassword: true,
}

// SecretConfig is used when cresting a secret.
type SecretConfig struct {
	Path           string
	RotateInterval time.Duration
	Params         map[string]interface{}
}

// NewSecretConfig is used to create an application scoped blob secret.
func NewSecretConfig(nameParts ...string) *SecretConfig {
	return &SecretConfig{
		Path: strings.Join(nameParts, "."),
	}
}

// TODO(wallyworld) - use a schema to describe the config
const (
	PasswordLength       = "password-length"
	PasswordSpecialChars = "password-special-chars"
)

// NewPasswordSecretConfig is used to create an application scoped password secret.
func NewPasswordSecretConfig(length int, specialChars bool, nameParts ...string) *SecretConfig {
	return &SecretConfig{
		Path: strings.Join(nameParts, "."),
		Params: map[string]interface{}{
			PasswordLength:       length,
			PasswordSpecialChars: specialChars,
		},
	}
}

var pathRegexp = regexp.MustCompile(`^[a-zA-Z0-9]+(\.[a-zA-Z0-9]+)*$`)

// Validate returns an error if the config is not valid.
func (c *SecretConfig) Validate() error {
	if !pathRegexp.MatchString(c.Path) {
		return errors.NotValidf("secret path %q", c.Path)
	}
	return nil
}

// URL represents a reference to a secret.
type URL struct {
	Version        string
	ControllerUUID string
	ModelUUID      string
	Path           string
	Attribute      string
	Revision       int
}

/*
Example secret URLs:
	secret://v1/apitoken
	secret://v1/mariadb.dbpass
	secret://v1/apache.catalog.password
	secret://v1/apache.catalog.password?revision=666
	secret://v1/proxy#key
	secret://v1/proxy#key?revision=666
	secret://v1/cfed9630-053e-447a-9751-2dc4ed429d51/myawscredential
	secret://v1/cfed9630-053e-447a-9751-2dc4ed429d51/11111111-053e-447a-6666-2dc4ed429d51/myawscredential
*/

const uuidSnippet = `[0-9a-f]{8}\b-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-\b[0-9a-f]{12}`

var secretURLParse = regexp.MustCompile(`^` +
	fmt.Sprintf(
		`((?P<version>v[1-9])\/)((?P<controllerUUID>%s)\/)?((?P<modelUUID>%s)\/)?(?P<path>[0-9a-zA-Z]+(\.[0-9a-zA-Z]+)*)(?P<revision>\?revision=[0-9]+)?(?P<attribute>#[0-9a-zA-Z]+)?`,
		uuidSnippet, uuidSnippet) +
	`$`)

// ParseURL parses the specified URL string into a URL.
func ParseURL(str string) (*URL, error) {
	u, err := url.Parse(str)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if u.Scheme != "secret" {
		return nil, errors.NotValidf("secret URL scheme %q", u.Scheme)
	}
	spec := fmt.Sprintf("%s%s", u.Host, u.Path)

	matches := secretURLParse.FindStringSubmatch(spec)
	if matches == nil {
		return nil, errors.NotValidf("secret URL %q", str)
	}
	revision := 0
	revisionParam := u.Query().Get("revision")
	if revisionParam != "" {
		revision, err = strconv.Atoi(revisionParam)
		if err != nil {
			return nil, errors.NotValidf("secret revision %q", revisionParam)
		}
	}
	result := &URL{
		Version:        matches[2],
		ControllerUUID: matches[4],
		ModelUUID:      matches[6],
		Path:           matches[7],
		Attribute:      u.Fragment,
		Revision:       revision,
	}
	return result, nil
}

// NewSimpleURL returns a URL with the specified path.
func NewSimpleURL(version int, path string) *URL {
	return &URL{
		Version:        fmt.Sprintf("v%d", version),
		Path:           path,
	}
}

// WithRevision returns the URL with the specified revision.
func (u *URL) WithRevision(revision int) *URL {
	if u == nil {
		return nil
	}
	uCopy := *u
	uCopy.Revision = revision
	return &uCopy
}

// WithAttribute returns the URL with the specified attribute.
func (u *URL) WithAttribute(attr string) *URL {
	if u == nil {
		return nil
	}
	uCopy := *u
	uCopy.Attribute = attr
	return &uCopy
}

// ID returns the URL string without any Attribute.
func (u *URL) ID() string {
	if u == nil {
		return ""
	}
	return u.WithAttribute("").String()
}

// ShortString prints the URL without controller or model UUID.
func (u *URL) ShortString() string {
	if u == nil {
		return ""
	}
	uCopy := *u
	uCopy.ControllerUUID = ""
	uCopy.ModelUUID = ""
	return uCopy.String()
}

// String prints the URL as a string.
func (u *URL) String() string {
	if u == nil {
		return ""
	}
	fullPath := []string{"secret:/", u.Version}
	if u.ControllerUUID != "" {
		fullPath = append(fullPath, u.ControllerUUID)
	}
	if u.ModelUUID != "" {
		fullPath = append(fullPath, u.ModelUUID)
	}
	fullPath = append(fullPath, u.Path)
	str := strings.Join(fullPath, "/")
	if u.Revision > 0 {
		str += fmt.Sprintf("?revision=%d", u.Revision)
	}
	if u.Attribute != "" {
		str += "#" + u.Attribute
	}
	return str
}

// SecretMetadata holds metadata about a secret.
type SecretMetadata struct {
	// Read only after creation.
	URL  *URL
	Path string

	// Version starts at 1 and is incremented
	// whenever an incompatible change is made.
	Version int

	// These can be updated after creation.
	Description    string
	Tags           map[string]string
	RotateInterval time.Duration

	// Set by service on creation/update.

	// ID is a Juju ID for the secret.
	ID int

	// Provider is the name of the backend secrets store.
	Provider string
	// ProviderID is the ID used by the underlying secrets provider.
	ProviderID string
	// Revision is incremented each time the corresponding
	// secret value is changed.
	Revision int

	CreateTime time.Time
	UpdateTime time.Time
}
