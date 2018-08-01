// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongodoc // import "gopkg.in/juju/charmstore.v5/internal/mongodoc"

import (
	"time"

	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/charmrepo.v3/csclient/params"
	"gopkg.in/mgo.v2/bson"
)

// Entity holds the in-database representation of charm or bundle's
// document in the charms collection. It holds information
// on one specific revision and series of the charm or bundle - see
// also BaseEntity.
//
// We ensure that there is always a single BaseEntity for any
// set of entities which share the same base URL.
type Entity struct {
	// URL holds the fully specified URL of the charm or bundle.
	// e.g. cs:precise/wordpress-34, cs:~user/trusty/foo-2
	URL *charm.URL `bson:"_id"`

	// BaseURL holds the reference URL of the charm or bundle
	// (this omits the series and revision from URL)
	// e.g. cs:wordpress, cs:~user/foo
	BaseURL *charm.URL

	// User holds the user part of the entity URL (for instance, "joe").
	User string

	// Name holds the name of the entity (for instance "wordpress").
	Name string

	// Revision holds the entity revision (it cannot be -1/unset).
	Revision int

	// Series holds the entity series (for instance "trusty" or "bundle").
	// For multi-series charms, this will be empty.
	Series string

	// SupportedSeries holds the series supported by a charm.
	// For non-multi-series charms, this is a single element slice
	// containing the value in Series.
	SupportedSeries []string

	// PreV5BlobHash holds the hash checksum of the
	// blob that will be served from the v4 and legacy
	// APIs. This will be the same as BlobHash for single-series charms.
	PreV5BlobHash string

	// PreV5BlobExtraHash holds the hash of the extra
	// blob that's appended to the main blob. This is empty
	// when PreV5BlobHash is the same as BlobHash.
	PreV5BlobExtraHash string `bson:",omitempty"`

	// PreV5BlobSize holds the size of the
	// blob that will be served from the v4 and legacy
	// APIs. This will be the same as Size for single-series charms.
	PreV5BlobSize int64

	// PreV5BlobHash256 holds the SHA256 hash checksum
	// of the blob that will be served from the v4 and legacy
	// APIs. This will be the same as Hash256 for single-series charms.
	PreV5BlobHash256 string

	// BlobHash holds the hash checksum of the blob, in hexadecimal format,
	// as created by blobstore.NewHash.
	BlobHash string

	// BlobHash256 holds the SHA256 hash checksum of the blob,
	// in hexadecimal format. This is only used by the legacy
	// API, and is calculated lazily the first time it is required.
	// Note that this is calculated from the pre-V5 blob.
	BlobHash256 string

	// Size holds the size of the archive blob.
	// TODO(rog) rename this to BlobSize.
	Size int64

	UploadTime time.Time

	// ExtraInfo holds arbitrary extra metadata associated with
	// the entity. The byte slices hold JSON-encoded data.
	ExtraInfo map[string][]byte `bson:",omitempty" json:",omitempty"`

	// TODO(rog) verify that all these types marshal to the expected
	// JSON form.
	CharmMeta    *charm.Meta
	CharmMetrics *charm.Metrics
	CharmConfig  *charm.Config
	CharmActions *charm.Actions

	// CharmProvidedInterfaces holds all the relation
	// interfaces provided by the charm
	CharmProvidedInterfaces []string

	// CharmRequiredInterfaces is similar to CharmProvidedInterfaces
	// for required interfaces.
	CharmRequiredInterfaces []string

	BundleData   *charm.BundleData
	BundleReadMe string

	// BundleCharms includes all the charm URLs referenced
	// by the bundle, including base URLs where they are
	// not already included.
	BundleCharms []*charm.URL

	// BundleMachineCount counts the machines used or created
	// by the bundle. It is nil for charms.
	BundleMachineCount *int

	// BundleUnitCount counts the units created by the bundle.
	// It is nil for charms.
	BundleUnitCount *int

	// TODO Add fields denormalized for search purposes
	// and search ranking field(s).

	// Contents holds entries for frequently accessed
	// entries in the file's blob. Storing this avoids
	// the need to linearly read the zip file's manifest
	// every time we access one of these files.
	Contents map[FileId]ZipFile `json:",omitempty" bson:",omitempty"`

	// PromulgatedURL holds the promulgated URL of the entity. If the entity
	// is not promulgated this should be set to nil.
	PromulgatedURL *charm.URL `json:",omitempty" bson:"promulgated-url,omitempty"`

	// PromulgatedRevision holds the revision number from the promulgated URL.
	// If the entity is not promulgated this should be set to -1.
	PromulgatedRevision int `bson:"promulgated-revision"`

	// Published holds whether the entity has been published on a channel.
	Published map[params.Channel]bool `json:",omitempty" bson:",omitempty"`
}

// PreferredURL returns the preferred way to refer to this entity. If
// the entity has a promulgated URL and usePromulgated is true then the
// promulgated URL will be used, otherwise the standard URL is used.
//
// The returned URL may be modified freely.
func (e *Entity) PreferredURL(usePromulgated bool) *charm.URL {
	var u charm.URL
	if usePromulgated && e.PromulgatedURL != nil {
		u = *e.PromulgatedURL
	} else {
		u = *e.URL
	}
	return &u
}

// BaseEntity holds metadata for a charm or bundle
// independent of any specific uploaded revision or series.
type BaseEntity struct {
	// URL holds the reference URL of of charm on bundle
	// regardless of its revision, series or promulgation status
	// (this omits the revision and series from URL).
	// e.g., cs:~user/collection/foo
	URL *charm.URL `bson:"_id"`

	// User holds the user part of the entity URL (for instance, "joe").
	User string

	// Name holds the name of the entity (for instance "wordpress").
	Name string

	// Promulgated specifies whether the charm or bundle should be
	// promulgated.
	Promulgated IntBool

	// CommonInfo holds arbitrary common extra metadata associated with
	// the base entity. Thhose data apply to all revisions.
	// The byte slices hold JSON-encoded data.
	CommonInfo map[string][]byte `bson:",omitempty" json:",omitempty"`

	// ChannelACLs holds a map from an entity channel to the ACLs
	// that apply to entities that use this base entity that are associated
	// with the given channel.
	ChannelACLs map[params.Channel]ACL

	// ChannelEntities holds a set of channels, each containing a set
	// of series holding the currently published entity revision for
	// that channel and series.
	ChannelEntities map[params.Channel]map[string]*charm.URL

	// ChannelResources holds a set of channels, each containing a
	// set of resource names holding the currently published resource
	// version for that channel and resource name.
	ChannelResources map[params.Channel][]ResourceRevision

	// NoIngest is set to true when a charm or bundle has been uploaded
	// with a POST request. Since the ingester only uses PUT requests
	// at present, this signifies that someone has taken over control from
	// the ingester.
	NoIngest bool `bson:",omitempty"`
}

// LatestRevision holds an entry in the revisions collection.
type LatestRevision struct {
	// URL holds the id that the latest revision is associated
	// with. URL.Revision is always -1.
	URL *charm.URL `bson:"_id"`

	// BaseURL holds the reference URL of the charm or bundle
	// (this omits the series from URL)
	// e.g. cs:wordpress, cs:~user/foo
	BaseURL *charm.URL

	// Revision holds the latest known revision for the
	// URL.
	Revision int
}

// ResourceRevision specifies an association of a resource name to a
// revision.
type ResourceRevision struct {
	Name     string
	Revision int
}

// ACL holds lists of users and groups that are
// allowed to perform specific actions.
type ACL struct {
	// Read holds users and groups that are allowed to read the charm
	// or bundle.
	Read []string
	// Write holds users and groups that are allowed to upload/modify the charm
	// or bundle.
	Write []string
}

type FileId string

const (
	FileReadMe FileId = "readme"
	FileIcon   FileId = "icon"
)

// ZipFile refers to a specific file in the uploaded archive blob.
type ZipFile struct {
	// Compressed specifies whether the file is compressed or not.
	Compressed bool

	// Offset holds the offset into the zip archive of the start of
	// the file's data.
	Offset int64

	// Size holds the size of the file before decompression.
	Size int64
}

// Valid reports whether f is a valid (non-zero) reference to
// a zip file.
func (f ZipFile) IsValid() bool {
	// Note that no valid zip files can start at offset zero,
	// because that's where the zip header lives.
	return f != ZipFile{}
}

// Log holds the in-database representation of a log message sent to the charm
// store.
type Log struct {
	// Data holds the JSON-encoded log message.
	Data []byte

	// Level holds the log level: whether the log is a warning, an error, etc.
	Level LogLevel

	// Type holds the log type.
	Type LogType

	// URLs holds a slice of entity URLs associated with the log message.
	URLs []*charm.URL

	// Time holds the time of the log.
	Time time.Time
}

// LogLevel holds the level associated with a log.
type LogLevel int

// When introducing a new log level, do the following:
// 1) add the new level as a constant below;
// 2) add the new level in params as a string for HTTP requests/responses;
// 3) include the new level in the mongodocLogLevels and paramsLogLevels maps
//    in internal/v4.
const (
	_ LogLevel = iota
	InfoLevel
	WarningLevel
	ErrorLevel
)

// LogType holds the type of the log.
type LogType int

// When introducing a new log type, do the following:
// 1) add the new type as a constant below;
// 2) add the new type in params as a string for HTTP requests/responses;
// 3) include the new type in the mongodocLogTypes and paramsLogTypes maps
//    in internal/v4.
const (
	_ LogType = iota
	IngestionType
	LegacyStatisticsType
)

type MigrationName string

// Migration holds information about the database migration.
type Migration struct {
	// Executed holds the migration names for migrations already executed.
	Executed []MigrationName
}

// IntBool is a bool that will be represented internally in the database as 1 for
// true and -1 for false.
type IntBool bool

func (b IntBool) GetBSON() (interface{}, error) {
	if b {
		return 1, nil
	}
	return -1, nil
}

func (b *IntBool) SetBSON(raw bson.Raw) error {
	var x int
	if err := raw.Unmarshal(&x); err != nil {
		return errgo.Notef(err, "cannot unmarshal value")
	}
	switch x {
	case 1:
		*b = IntBool(true)
	case -1:
		*b = IntBool(false)
	default:
		return errgo.Newf("invalid value %d", x)
	}
	return nil
}

// BaseURL returns the "base" version of url. If
// url represents an entity, then the returned URL
// will represent its base entity.
func BaseURL(url *charm.URL) *charm.URL {
	newURL := *url
	newURL.Revision = -1
	newURL.Series = ""
	return &newURL
}

func copyURL(u *charm.URL) *charm.URL {
	u1 := *u
	return &u1
}
