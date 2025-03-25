// Copyright 2021 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package secrets

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/rs/xid"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
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
		return errors.Errorf("secret rotate policy %q %w", c.RotatePolicy, coreerrors.NotValid)
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
		return nil, errors.Capture(err)
	}
	if u.Scheme == "" {
		u.Scheme = SecretScheme
	} else if u.Scheme != SecretScheme {
		return nil, errors.Errorf("secret URI scheme %q %w", u.Scheme, coreerrors.NotValid)
	}
	if u.Host != "" && !validUUID.MatchString(u.Host) {
		return nil, errors.Errorf("host controller UUID %q %w", u.Host, coreerrors.NotValid)
	}

	idStr := strings.TrimLeft(u.Path, "/")
	if idStr == "" {
		idStr = u.Opaque
	}
	valid := secretURIParse.MatchString(idStr)
	if !valid {
		return nil, errors.Errorf("secret URI %q %w", str, coreerrors.NotValid)
	}
	sourceUUID := secretURIParse.ReplaceAllString(idStr, "$source")
	if sourceUUID == "" {
		sourceUUID = u.Host
	}
	idPart := secretURIParse.ReplaceAllString(idStr, "$id")
	id, err := xid.FromString(idPart)
	if err != nil {
		return nil, errors.Errorf("secret URI %q %w", str, coreerrors.NotValid)
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

// OwnerKind represents the kind of a secret owner entity.
type OwnerKind string

// These represent the kinds of secret owner.
const (
	ApplicationOwner OwnerKind = "application"
	UnitOwner        OwnerKind = "unit"
	ModelOwner       OwnerKind = "model"
)

// Owner is the owner of a secret.
type Owner struct {
	Kind OwnerKind
	ID   string
}

func (o Owner) String() string {
	return fmt.Sprintf("%s-%s", o.Kind, strings.ReplaceAll(o.ID, "/", "-"))
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

	// Owner is the entity which created the secret.
	Owner Owner

	CreateTime time.Time
	UpdateTime time.Time

	// These are denormalised here for ease of access.

	// LatestRevision is the most recent secret revision.
	LatestRevision int
	// LatestRevisionChecksum is the checksum of the most
	// recent revision content.
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

// AccessorKind represents the kind of a secret accessor entity.
type AccessorKind string

// These represent the kinds of secret accessor.
const (
	UnitAccessor  AccessorKind = "unit"
	ModelAccessor AccessorKind = "model"
)

// Accessor is the accessor of a secret.
type Accessor struct {
	Kind AccessorKind
	ID   string
}

func (a Accessor) String() string {
	return fmt.Sprintf("%s-%s", a.Kind, strings.ReplaceAll(a.ID, "/", "-"))
}

// SecretRevisionRef is a reference to a secret revision
// stored in a secret backend.
type SecretRevisionRef struct {
	URI        *URI
	RevisionID string
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

// SecretExternalRevision holds metadata about an external secret revision.
type SecretExternalRevision struct {
	Revision int
	ValueRef *ValueRef
}

// SecretMetadataForDrain holds a secret metadata and any backend references of revisions for drain.
type SecretMetadataForDrain struct {
	URI       *URI
	Revisions []SecretExternalRevision
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
}

// SecretRevisionInfo holds info used to read a secret vale.
type SecretRevisionInfo struct {
	LatestRevision int
	Label          string
}

// Filter is used when querying secrets.
type Filter struct {
	URI      *URI
	Label    *string
	Revision *int
	Owner    *Owner
}
