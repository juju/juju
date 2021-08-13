// Copyright 2021 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package secrets

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/juju/errors"
)

// Scope represents the scope of a secret.
type Scope string

const (
	ScopeApplication = Scope("application")
)

var validScopes = map[Scope]bool{
	ScopeApplication: true,
}

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
	Type   SecretType
	Path   string
	Scope  Scope
	Params map[string]interface{}
}

// NewSecretConfig is used to create an application scoped blob secret.
func NewSecretConfig(nameParts ...string) *SecretConfig {
	return &SecretConfig{
		Type:  TypeBlob,
		Scope: ScopeApplication,
		Path:  strings.Join(nameParts, "."),
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
		Type:  TypePassword,
		Scope: ScopeApplication,
		Path:  strings.Join(nameParts, "."),
		Params: map[string]interface{}{
			PasswordLength:       length,
			PasswordSpecialChars: specialChars,
		},
	}
}

var pathRegexp = regexp.MustCompile(`^[a-zA-Z0-9]+(\.[a-zA-Z0-9]+)*$`)

// Validate returns an error if the config is not valid.
func (c *SecretConfig) Validate() error {
	if _, ok := validTypes[c.Type]; !ok {
		return errors.NotValidf("secret type %q", c.Type)
	}
	if _, ok := validScopes[c.Scope]; !ok {
		return errors.NotValidf("secret scope %q", c.Scope)
	}
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
}

/*
Example secret URLs:
	secret://v1/apitoken
	secret://v1/mariadb.dbpass
	secret://v1/apache.catalog.password
	secret://v1/proxy#key
	secret://v1/cfed9630-053e-447a-9751-2dc4ed429d51/myawscredential
	secret://v1/cfed9630-053e-447a-9751-2dc4ed429d51/11111111-053e-447a-6666-2dc4ed429d51/myawscredential
*/

const uuidSnippet = `[0-9a-f]{8}\b-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-\b[0-9a-f]{12}`

var secretURLParse = regexp.MustCompile(`^` +
	fmt.Sprintf(
		`((?P<version>v[1-9])\/)((?P<controllerUUID>%s)\/)?((?P<modelUUID>%s)\/)?(?P<path>[0-9a-zA-Z]+(\.[0-9a-zA-Z]+)*)(#[0-9a-zA-Z]+)?`,
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
	result := &URL{
		Version:        matches[2],
		ControllerUUID: matches[4],
		ModelUUID:      matches[6],
		Path:           matches[7],
		Attribute:      u.Fragment,
	}
	return result, nil
}

// ShortString prints the URL without controller or model UUID.
func (u *URL) ShortString() string {
	fullPath := []string{"secret:/", u.Version}
	fullPath = append(fullPath, u.Path)
	str := strings.Join(fullPath, "/")
	if u.Attribute != "" {
		str += "#" + u.Attribute
	}
	return str
}

// String prints the URL as a string.
func (u *URL) String() string {
	fullPath := []string{"secret:/", u.Version}
	if u.ControllerUUID != "" {
		fullPath = append(fullPath, u.ControllerUUID)
	}
	if u.ModelUUID != "" {
		fullPath = append(fullPath, u.ModelUUID)
	}
	fullPath = append(fullPath, u.Path)
	str := strings.Join(fullPath, "/")
	if u.Attribute != "" {
		str += "#" + u.Attribute
	}
	return str
}

// SecretMetadata holds metadata about a secret.
type SecretMetadata struct {
	// Read only after creation.
	Path  string
	Scope Scope

	// Version starts at 1 and is incremented
	// whenever an incompatible change is made.
	Version int

	// These can be updated after creation.
	Description string
	Tags        map[string]string

	// Set by service on creation/update.

	// ID is a Juju ID for the secret.
	ID int

	// ProviderID is the ID used by the underlying secrets provider.
	ProviderID string
	// Revision is incremented each time the corresponding
	// secret value is changed.
	Revision int

	CreateTime time.Time
	UpdateTime time.Time
}
