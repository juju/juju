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
	"github.com/rs/xid"
)

// SecretConfig is used when creating a secret.
type SecretConfig struct {
	RotatePolicy   *RotatePolicy
	NextRotateTime *time.Time
	ExpireTime     *time.Time
	Description    *string
	Label          *string
	Params         map[string]interface{}
}

// Validate returns an error if params are invalid.
func (c *SecretConfig) Validate() error {
	if c.RotatePolicy != nil && !c.RotatePolicy.IsValid() {
		return errors.NotValidf("secret rotate policy %q", c.RotatePolicy)
	}
	if c.RotatePolicy.WillRotate() && c.NextRotateTime == nil {
		return errors.New("cannot specify a secret rotate policy without a next rotate time")
	}
	if !c.RotatePolicy.WillRotate() && c.NextRotateTime != nil {
		return errors.New("cannot specify a secret rotate time without a rotate policy")
	}
	return nil
}

// URI represents a reference to a secret.
type URI struct {
	ID             string
	ControllerUUID string
}

const (
	uuidSnippet = `[0-9a-f]{8}\b-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-\b[0-9a-f]{12}`

	idSnippet = `[0-9a-z]{20}`

	// SecretScheme is the URL prefix for a secret.
	SecretScheme = "secret"
)

var secretURIParse = regexp.MustCompile(`^` +
	fmt.Sprintf(`((?P<ControllerUUID>%s)/)?(?P<id>%s)`, uuidSnippet, idSnippet) +
	`$`)

// ParseURI parses the specified string into a URI.
func ParseURI(str string) (*URI, error) {
	u, err := url.Parse(str)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if u.Scheme == "" {
		u.Scheme = SecretScheme
	} else if u.Scheme != SecretScheme {
		return nil, errors.NotValidf("secret URI scheme %q", u.Scheme)
	}
	spec := fmt.Sprintf("%s%s", u.Host, u.Opaque)
	if spec == "" {
		spec = u.Path
	}

	matches := secretURIParse.FindStringSubmatch(spec)
	if matches == nil {
		return nil, errors.NotValidf("secret URI %q", str)
	}
	id, err := xid.FromString(matches[3])
	if err != nil {
		return nil, errors.NotValidf("secret URI %q", str)
	}
	result := &URI{
		ControllerUUID: matches[2],
		ID:             id.String(),
	}
	return result, nil
}

// Raw returns the URI with just the ID part.
// Used in tests.
func (u *URI) Raw() *URI {
	c := *u
	c.ControllerUUID = ""
	return &c
}

// NewURI returns a new secret URI.
func NewURI() *URI {
	return &URI{
		ID: xid.New().String(),
	}
}

// ShortString prints the URI without controller UUID.
func (u *URI) ShortString() string {
	if u == nil {
		return ""
	}
	uCopy := *u
	uCopy.ControllerUUID = ""
	return uCopy.String()
}

// String prints the URI as a string.
func (u *URI) String() string {
	if u == nil {
		return ""
	}
	var fullPath []string
	if u.ControllerUUID != "" {
		fullPath = append(fullPath, u.ControllerUUID)
	}
	fullPath = append(fullPath, u.ID)
	str := strings.Join(fullPath, "/")
	urlValue := url.URL{
		Scheme: SecretScheme,
		Opaque: str,
	}
	return urlValue.String()
}

// SecretMetadata holds metadata about a secret.
type SecretMetadata struct {
	// Read only after creation.
	URI *URI

	// Version starts at 1 and is incremented
	// whenever an incompatible change is made.
	Version int

	// These can be updated after creation.
	Description  string
	Label        string
	RotatePolicy RotatePolicy

	// Set by service on creation/update.

	// OwnerTag is the entity which created the secret.
	OwnerTag string
	// ProviderID is the ID/URI used by the underlying secrets provider.
	ProviderID string

	CreateTime time.Time
	UpdateTime time.Time

	// These are denormalised here for ease of access.

	// LatestRevision is the most recent secret revision.
	LatestRevision int
	// LatestExpireTime is the expire time of the most recent revision.
	LatestExpireTime *time.Time
	// NextRotateTime is when the secret should be rotated.
	NextRotateTime *time.Time
}

// SecretRevisionMetadata holds metadata about a secret revision.
type SecretRevisionMetadata struct {
	Revision   int
	CreateTime time.Time
	UpdateTime time.Time
	ExpireTime *time.Time
}

// SecretConsumerMetadata holds metadata about a secret
// for a consumer of the secret.
type SecretConsumerMetadata struct {
	// Label is used when notifying the consumer
	// about changes to the secret.
	Label string
	// CurrentRevision is current revision the
	// consumer wants to read.
	CurrentRevision int
	// LatestRevision is the latest secret revision.
	LatestRevision int
}

// SecretRevisionInfo holds info used to read a secret vale.
type SecretRevisionInfo struct {
	Revision int
	Label    string
}

// Filter is used when querying secrets.
type Filter struct {
	URI      *URI
	Revision *int
	OwnerTag *string
}
