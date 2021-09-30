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

// SecretStatus is the status of a secret.
type SecretStatus string

const (
	StatusPending = SecretStatus("pending")
	StatusActive  = SecretStatus("active")
)

// IsValid returns true if s is a valid secret status.
func (s SecretStatus) IsValid() bool {
	switch s {
	case StatusActive, StatusPending:
		return true
	}
	return false
}

// SecretType is the type of a secret.
// This is used when creating a secret.
type SecretType string

const (
	TypeBlob     = SecretType("blob")
	TypePassword = SecretType("password")
)

// IsValid returns true if t is a valid secret type.
func (t SecretType) IsValid() bool {
	switch t {
	case TypeBlob, TypePassword:
		return true
	}
	return false
}

// SecretConfig is used when creating a secret.
type SecretConfig struct {
	Path           string
	RotateInterval *time.Duration
	Status         *SecretStatus
	Description    *string
	Tags           *map[string]string
	Params         map[string]interface{}
}

// NewSecretConfig is used to create an application scoped blob secret.
func NewSecretConfig(nameParts ...string) *SecretConfig {
	return &SecretConfig{
		Path: strings.Join(nameParts, "/"),
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
		Path: strings.Join(nameParts, "/"),
		Params: map[string]interface{}{
			PasswordLength:       length,
			PasswordSpecialChars: specialChars,
		},
	}
}

const (
	// AppSnippet denotes a secret belonging to an application.
	AppSnippet  = "app"
	pathSnippet = AppSnippet + `/[a-zA-Z0-9-]+((/[a-zA-Z0-9-]+)*/[a-zA-Z]+[a-zA-Z0-9-]*)*`
)

var pathRegexp = regexp.MustCompile("^" + pathSnippet + "$")

// Validate returns an error if the config is not valid.
func (c *SecretConfig) Validate() error {
	if !pathRegexp.MatchString(c.Path) {
		return errors.NotValidf("secret path %q", c.Path)
	}
	return nil
}

// URL represents a reference to a secret.
type URL struct {
	ControllerUUID string
	ModelUUID      string
	Path           string
	Attribute      string
	Revision       int
}

/*
Example secret URLs:
	secret://app/gitlab/apitoken
	secret://app/mariadb/dbpass
	secret://app/apache/catalog/password/666
	secret://app/proxy#key
	secret://app/proxy/666#key
	secret://cfed9630-053e-447a-9751-2dc4ed429d51/app/mariadb/password
	secret://cfed9630-053e-447a-9751-2dc4ed429d51/11111111-053e-447a-6666-2dc4ed429d51/app/mariadb/password
*/

const (
	uuidSnippet = `[0-9a-f]{8}\b-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-\b[0-9a-f]{12}`

	// SecretScheme is the URL prefix for a secret.
	SecretScheme = "secret"
)

var secretURLParse = regexp.MustCompile(`^` +
	fmt.Sprintf(
		`((?P<controllerUUID>%s)/)?((?P<modelUUID>%s)/)?(?P<path>%s)(/(?P<revision>[0-9]+))?(?P<attribute>#[0-9a-zA-Z]+)?`,
		uuidSnippet, uuidSnippet, pathSnippet) +
	`$`)

// ParseURL parses the specified URL string into a URL.
func ParseURL(str string) (*URL, error) {
	u, err := url.Parse(str)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if u.Scheme != SecretScheme {
		return nil, errors.NotValidf("secret URL scheme %q", u.Scheme)
	}
	spec := fmt.Sprintf("%s%s", u.Host, u.Path)

	matches := secretURLParse.FindStringSubmatch(spec)
	if matches == nil {
		return nil, errors.NotValidf("secret URL %q", str)
	}
	revision := 0
	revisionParam := matches[9]
	if revisionParam != "" {
		revision, err = strconv.Atoi(strings.Trim(revisionParam, "/"))
		if err != nil {
			return nil, errors.NotValidf("secret revision %q", revisionParam)
		}
	}
	result := &URL{
		ControllerUUID: matches[2],
		ModelUUID:      matches[4],
		Path:           matches[5],
		Attribute:      u.Fragment,
		Revision:       revision,
	}
	return result, nil
}

// NewSimpleURL returns a URL with the specified path.
func NewSimpleURL(path string) *URL {
	return &URL{
		Path: path,
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

// OwnerApplication returns the application part of a secret URL.
func (u *URL) OwnerApplication() (string, bool) {
	parts := strings.Split(u.Path, "/")
	if len(parts) < 2 || parts[0] != AppSnippet {
		return "", false
	}
	return parts[1], true
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
	var fullPath []string
	if u.ControllerUUID != "" {
		fullPath = append(fullPath, u.ControllerUUID)
	}
	if u.ModelUUID != "" {
		fullPath = append(fullPath, u.ModelUUID)
	}
	fullPath = append(fullPath, u.Path)
	if u.Revision > 0 {
		fullPath = append(fullPath, strconv.Itoa(u.Revision))
	}
	str := strings.Join(fullPath, "/")
	urlValue := url.URL{
		Scheme:   SecretScheme,
		Path:     str,
		Fragment: u.Attribute,
	}
	return urlValue.String()
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
	Status         SecretStatus
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
