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
	SourceUUID string
	ID         string
}

const (
	idSnippet   = `[0-9a-z]{20}`
	uuidSnippet = `[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}`

	// SecretScheme is the URL prefix for a secret.
	SecretScheme = "secret"
)

var validUUID = regexp.MustCompile(uuidSnippet)

var secretURIParse = regexp.MustCompile(`^` +
	fmt.Sprintf(`((?P<source>%s)/)?(?P<id>%s)`, uuidSnippet, idSnippet) +
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
	if u.Host != "" && !validUUID.MatchString(u.Host) {
		return nil, errors.NotValidf("host controller UUID %q", u.Host)
	}

	idStr := strings.TrimLeft(u.Path, "/")
	if idStr == "" {
		idStr = u.Opaque
	}
	valid := secretURIParse.MatchString(idStr)
	if !valid {
		return nil, errors.NotValidf("secret URI %q", str)
	}
	sourceUUID := secretURIParse.ReplaceAllString(idStr, "$source")
	if sourceUUID == "" {
		sourceUUID = u.Host
	}
	idPart := secretURIParse.ReplaceAllString(idStr, "$id")
	id, err := xid.FromString(idPart)
	if err != nil {
		return nil, errors.NotValidf("secret URI %q", str)
	}
	result := &URI{
		SourceUUID: sourceUUID,
		ID:         id.String(),
	}
	return result, nil
}

// NewURI returns a new secret URI.
func NewURI() *URI {
	return &URI{
		ID: xid.New().String(),
	}
}

// WithSource returns a secret URI with the source.
func (u *URI) WithSource(uuid string) *URI {
	u.SourceUUID = uuid
	return u
}

// IsLocal returns true if this URI is local
// to the specified uuid.
func (u *URI) IsLocal(sourceUUID string) bool {
	return u.SourceUUID == "" || u.SourceUUID == sourceUUID
}

// Name generates the secret name.
func (u URI) Name(revision int) string {
	return fmt.Sprintf("%s-%d", u.ID, revision)
}

// String prints the URI as a string.
func (u *URI) String() string {
	if u == nil {
		return ""
	}
	var fullPath []string
	fullPath = append(fullPath, u.ID)
	str := strings.Join(fullPath, "/")
	if u.SourceUUID == "" {
		urlValue := url.URL{
			Scheme: SecretScheme,
			Opaque: str,
		}
		return urlValue.String()
	}
	urlValue := url.URL{
		Scheme: SecretScheme,
		Host:   u.SourceUUID,
		Path:   str,
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

	CreateTime time.Time
	UpdateTime time.Time

	// These are denormalised here for ease of access.

	// LatestRevision is the most recent secret revision.
	LatestRevision int
	// LatestExpireTime is the expire time of the most recent revision.
	LatestExpireTime *time.Time
	// NextRotateTime is when the secret should be rotated.
	NextRotateTime *time.Time

	// AutoPrune is true if the secret revisions should be pruned when it's not been used.
	AutoPrune bool
}

// SecretRevisionMetadata holds metadata about a secret revision.
type SecretRevisionMetadata struct {
	Revision    int
	ValueRef    *ValueRef
	BackendName *string
	CreateTime  time.Time
	UpdateTime  time.Time
	ExpireTime  *time.Time
}

// SecretOwnerMetadata holds a secret metadata and any backend references of revisions.
type SecretOwnerMetadata struct {
	Metadata  SecretMetadata
	Revisions []int
}

// SecretMetadataForDrain holds a secret metadata and any backend references of revisions for drain.
type SecretMetadataForDrain struct {
	Metadata  SecretMetadata
	Revisions []SecretRevisionMetadata
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
