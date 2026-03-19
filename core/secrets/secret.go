// Copyright 2021 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package secrets

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
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
	revSnippet  = `[0-9]+`

	// SecretScheme is the URL prefix for a secret.
	SecretScheme = "secret"
)

var (
	validUUID = regexp.MustCompile(uuidSnippet)

	secretURI = regexp.MustCompile(fmt.Sprintf(
		`^((?P<source>%s)/)?(?P<id>%s)$`, uuidSnippet, idSnippet,
	))
	secretURISourceIdx = secretURI.SubexpIndex("source")
	secretURIIdIdx     = secretURI.SubexpIndex("id")

	secretRevision = regexp.MustCompile(fmt.Sprintf(
		`^(?P<id>%s)-(?P<rev>%s)$`, idSnippet, revSnippet,
	))
	secretRevisionIdIdx  = secretRevision.SubexpIndex("id")
	secretRevisionRevIdx = secretRevision.SubexpIndex("rev")
)

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

	matched := secretURI.FindStringSubmatch(idStr)
	if len(matched) <= max(secretURIIdIdx, secretURISourceIdx) {
		return nil, errors.NotValidf("secret URI %q", str)
	}
	sourceUUID := matched[secretURISourceIdx]
	if sourceUUID == "" {
		sourceUUID = u.Host
	}
	idPart := matched[secretURIIdIdx]
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

var xidEncoding = base32.NewEncoding(
	"0123456789abcdefghijklmnopqrstuv",
).WithPadding(base32.NoPadding)

// NewURI returns a new secret URI.
func NewURI() *URI {
	var r [12]byte
	_, _ = rand.Read(r[:])
	id := xidEncoding.EncodeToString(r[:])
	return &URI{
		ID: id,
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

// Name generates the secret revision name.
func (u URI) Name(revision int) string {
	return RevisionName(u.ID, revision)
}

// RevisionName generates the secret revision name.
func RevisionName(secretID string, revision int) string {
	return fmt.Sprintf("%s-%d", secretID, revision)
}

// ParseRevisionName parses the provided revision name, returning the secret ID
// and the revision number, or an error.
func ParseRevisionName(revisionName string) (string, int, error) {
	matched := secretRevision.FindStringSubmatch(revisionName)
	if len(matched) <= max(secretRevisionIdIdx, secretRevisionRevIdx) {
		return "", 0, errors.NotValidf("secret revision %q", revisionName)
	}
	id := matched[secretRevisionIdIdx]
	rev, err := strconv.Atoi(matched[secretRevisionRevIdx])
	if err != nil {
		return "", 0, errors.NotValidf("secret revision %q: %v",
			revisionName, err)
	}
	return id, rev, nil
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

// SecretMetadataOwnerIdent contains enough information to identify a secret for
// an owner.
type SecretMetadataOwnerIdent struct {
	URI      *URI
	OwnerTag string
	Label    string
}

// SecretURIWithRevisions contains enough information to identify revisions that
// exist for a secret.
type SecretURIWithRevisions struct {
	URI       *URI
	Revisions []int
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
	LatestRevision         int
	LatestRevisionChecksum string
	// LatestExpireTime is the expire time of the most recent revision.
	LatestExpireTime *time.Time
	// NextRotateTime is when the secret should be rotated.
	NextRotateTime *time.Time

	// AutoPrune is true if the secret revisions should be pruned when it's not been used.
	AutoPrune bool

	// Access is a list of access information for this secret.
	Access []AccessInfo
}

// AccessInfo holds info about a secret access information.
type AccessInfo struct {
	Target string
	Scope  string
	Role   SecretRole
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

// SecretMetadataForDrain holds a secret URI and any backend references of revisions for drain.
type SecretMetadataForDrain struct {
	URI       *URI
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
	Label    *string
	Revision *int
	OwnerTag *string
}
