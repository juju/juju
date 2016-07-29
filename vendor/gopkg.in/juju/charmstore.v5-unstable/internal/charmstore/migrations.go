// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmstore // import "gopkg.in/juju/charmstore.v5-unstable/internal/charmstore"

import (
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/charmrepo.v2-unstable/csclient/params"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"gopkg.in/juju/charmstore.v5-unstable/internal/blobstore"
	"gopkg.in/juju/charmstore.v5-unstable/internal/mongodoc"
)

const (
	migrationAddSupportedSeries      mongodoc.MigrationName = "add supported series"
	migrationAddDevelopment          mongodoc.MigrationName = "add development"
	migrationAddDevelopmentACLs      mongodoc.MigrationName = "add development acls"
	migrationFixBogusPromulgatedURL  mongodoc.MigrationName = "fix promulgate url"
	migrationAddPreV5CompatBlobBogus mongodoc.MigrationName = "add pre-v5 compatibility blobs"
	migrationAddPreV5CompatBlob      mongodoc.MigrationName = "add pre-v5 compatibility blobs; second try"
	migrationNewChannelsModel        mongodoc.MigrationName = "new channels model"
)

// migrations holds all the migration functions that are executed in the order
// they are defined when the charm store server is started. Each migration is
// associated with a name that is used to check whether the migration has been
// already run. To introduce a new database migration, add the corresponding
// migration name and function to this list, and update the
// TestMigrateMigrationList test in migration_test.go adding the new name(s).
// Note that migration names must be unique across the list.
//
// A migration entry may have a nil migration function if the migration
// is obsolete. Obsolete migrations should never be removed entirely,
// otherwise the charmstore will see the old migrations in the table
// and refuse to start up because it thinks that it's running an old
// version of the charm store on a newer version of the database.
var migrations = []migration{{
	name: "entity ids denormalization",
}, {
	name: "base entities creation",
}, {
	name: "read acl creation",
}, {
	name: "write acl creation",
}, {
	name: migrationAddSupportedSeries,
}, {
	name: migrationAddDevelopment,
}, {
	name: migrationAddDevelopmentACLs,
}, {
	name: migrationFixBogusPromulgatedURL,
}, {
	// The original migration that attempted to do this actually did
	// nothing, so leave it here but use a new name for the
	// fixed version.
	name: migrationAddPreV5CompatBlobBogus,
}, {
	name:    migrationAddPreV5CompatBlob,
	migrate: addPreV5CompatBlob,
}, {
	name:    migrationNewChannelsModel,
	migrate: migrateToNewChannelsModel,
}}

// migration holds a migration function with its corresponding name.
type migration struct {
	name    mongodoc.MigrationName
	migrate func(StoreDatabase) error
}

// Migrate starts the migration process using the given database.
func migrate(db StoreDatabase) error {
	// Retrieve already executed migrations.
	executed, err := getExecuted(db)
	if err != nil {
		return errgo.Mask(err)
	}

	// Explicitly create the collection in case there are no migrations
	// so that the tests that expect the migrations collection to exist
	// will pass. We ignore the error because we'll get one if the
	// collection already exists and there's no special type or value
	// for that (and if it's a genuine error, we'll catch the problem later
	// anyway).
	db.Migrations().Create(&mgo.CollectionInfo{})
	// Execute required migrations.
	for _, m := range migrations {
		if executed[m.name] || m.migrate == nil {
			logger.Debugf("skipping already executed migration: %s", m.name)
			continue
		}
		logger.Infof("starting migration: %s", m.name)
		if err := m.migrate(db); err != nil {
			return errgo.Notef(err, "error executing migration: %s", m.name)
		}
		if err := setExecuted(db, m.name); err != nil {
			return errgo.Mask(err)
		}
		logger.Infof("migration completed: %s", m.name)
	}
	return nil
}

func getExecuted(db StoreDatabase) (map[mongodoc.MigrationName]bool, error) {
	// Retrieve the already executed migration names.
	executed := make(map[mongodoc.MigrationName]bool)
	var doc mongodoc.Migration
	if err := db.Migrations().Find(nil).Select(bson.D{{"executed", 1}}).One(&doc); err != nil {
		if err == mgo.ErrNotFound {
			return executed, nil
		}
		return nil, errgo.Notef(err, "cannot retrieve executed migrations")
	}

	names := make(map[mongodoc.MigrationName]bool, len(migrations))
	for _, m := range migrations {
		names[m.name] = true
	}
	for _, name := range doc.Executed {
		name := mongodoc.MigrationName(name)
		// Check that the already executed migrations are known.
		if !names[name] {
			return nil, errgo.Newf("found unknown migration %q; running old charm store code on newer charm store database?", name)
		}
		// Collect the name of the executed migration.
		executed[name] = true
	}
	return executed, nil
}

func addPreV5CompatBlob(db StoreDatabase) error {
	blobStore := blobstore.New(db.Database, "entitystore")
	entities := db.Entities()
	iter := entities.Find(nil).Select(map[string]int{
		"size":             1,
		"blobhash":         1,
		"blobname":         1,
		"blobhash256":      1,
		"charmmeta.series": 1,
	}).Iter()
	var entity mongodoc.Entity
	for iter.Next(&entity) {
		var info *preV5CompatibilityHackBlobInfo

		if entity.CharmMeta == nil || len(entity.CharmMeta.Series) == 0 {
			info = &preV5CompatibilityHackBlobInfo{
				hash:    entity.BlobHash,
				hash256: entity.BlobHash256,
				size:    entity.Size,
			}
		} else {
			r, _, err := blobStore.Open(entity.BlobName)
			if err != nil {
				return errgo.Notef(err, "cannot open original blob")
			}
			info, err = addPreV5CompatibilityHackBlob(blobStore, r, entity.BlobName, entity.Size)
			r.Close()
			if err != nil {
				return errgo.Mask(err)
			}
		}
		err := entities.UpdateId(entity.URL, bson.D{{
			"$set", bson.D{{
				"prev5blobhash", info.hash,
			}, {
				"prev5blobhash256", info.hash256,
			}, {
				"prev5blobsize", info.size,
			}},
		}})
		if err != nil {
			return errgo.Notef(err, "cannot update pre-v5 info")
		}
	}
	if err := iter.Err(); err != nil {
		return errgo.Notef(err, "cannot iterate through entities")
	}
	return nil
}

func migrateToNewChannelsModel(db StoreDatabase) error {
	if err := ncmUpdateDevelopmentAndStable(db); err != nil {
		return errgo.Mask(err)
	}
	if err := ncmUpdateBaseEntities(db); err != nil {
		return errgo.Mask(err)
	}
	return nil
}

// ncmUpdateDevelopmentAndStable updates the Development and Stable
// entity fields to conform to the new channels model.
// All entities are treated as if they're in development; entities
// without the development field set are treated as stable.
func ncmUpdateDevelopmentAndStable(db StoreDatabase) error {
	entities := db.Entities()
	iter := entities.Find(bson.D{{
		"stable", bson.D{{"$exists", false}},
	}}).Select(map[string]int{
		"_id":         1,
		"development": 1,
	}).Iter()

	// For every entity without a stable field, update
	// its development and stable fields appropriately.
	var entity mongodoc.Entity
	for iter.Next(&entity) {
		err := entities.UpdateId(entity.URL, bson.D{{
			"$set", bson.D{
				{"development", true},
				{"stable", !entity.Development},
			},
		}})
		if err != nil {
			return errgo.Notef(err, "cannot update entity")
		}
	}
	if err := iter.Err(); err != nil {
		return errgo.Notef(err, "cannot iterate through entities")
	}
	return nil
}

// preNCMBaseEntity holds the type of a base entity just before
// the new channels model migration.
type preNCMBaseEntity struct {
	// URL holds the reference URL of of charm on bundle
	// regardless of its revision, series or promulgation status
	// (this omits the revision and series from URL).
	// e.g., cs:~user/collection/foo
	URL *charm.URL `bson:"_id"`

	// User holds the user part of the entity URL (for instance, "joe").
	User string

	// Name holds the name of the entity (for instance "wordpress").
	Name string

	// Public specifies whether the charm or bundle
	// is available to all users. If this is true, the ACLs will
	// be ignored when reading a charm.
	Public bool

	// ACLs holds permission information relevant to the base entity.
	// The permissions apply to all revisions.
	ACLs mongodoc.ACL

	// DevelopmentACLs is similar to ACLs but applies to all development
	// revisions.
	DevelopmentACLs mongodoc.ACL

	// Promulgated specifies whether the charm or bundle should be
	// promulgated.
	Promulgated mongodoc.IntBool

	// CommonInfo holds arbitrary common extra metadata associated with
	// the base entity. Thhose data apply to all revisions.
	// The byte slices hold JSON-encoded data.
	CommonInfo map[string][]byte `bson:",omitempty" json:",omitempty"`
}

// ncmUpdateBaseEntities updates all the base entities to conform to
// the new channels model. It assumes that ncmUpdateDevelopmentAndStable
// has been run already.
func ncmUpdateBaseEntities(db StoreDatabase) error {
	baseEntities := db.BaseEntities()
	iter := baseEntities.Find(bson.D{{
		"channelentities", bson.D{{"$exists", false}},
	}}).Iter()
	// For every base entity without a ChannelEntities field, update
	// its ChannelEntities and and ChannelACLs field appropriately.
	var baseEntity preNCMBaseEntity
	for iter.Next(&baseEntity) {
		if err := ncmUpdateBaseEntity(db, &baseEntity); err != nil {
			return errgo.Mask(err)
		}
	}
	if err := iter.Err(); err != nil {
		return errgo.Notef(err, "cannot iterate through base entities")
	}
	return nil
}

// ncmUpdateBaseEntity updates a single base entity to conform to
// the new channels model.
func ncmUpdateBaseEntity(db StoreDatabase, baseEntity *preNCMBaseEntity) error {
	channelEntities := make(map[params.Channel]map[string]*charm.URL)

	updateChannelURL := func(url *charm.URL, ch params.Channel, series string) {
		if channelEntities[ch] == nil {
			channelEntities[ch] = make(map[string]*charm.URL)
		}
		if oldURL := channelEntities[ch][series]; oldURL == nil || oldURL.Revision < url.Revision {
			channelEntities[ch][series] = url
		}
	}
	// updateChannelEntity updates the series entries in channelEntities
	// for the given entity, setting the entity URL entry if the revision
	// is greater than any already found.
	updateChannelEntity := func(entity *mongodoc.Entity, ch params.Channel) {
		if entity.URL.Series == "" {
			for _, series := range entity.SupportedSeries {
				updateChannelURL(entity.URL, ch, series)
			}
		} else {
			updateChannelURL(entity.URL, ch, entity.URL.Series)
		}
	}

	// Iterate through all the entities associated with the base entity
	// to find the most recent "published" entities so that we can
	// populate the ChannelEntities field.
	var entity mongodoc.Entity
	iter := db.Entities().Find(bson.D{{"baseurl", baseEntity.URL}}).Iter()
	for iter.Next(&entity) {
		if entity.Development {
			updateChannelEntity(&entity, params.DevelopmentChannel)
		}
		if entity.Stable {
			updateChannelEntity(&entity, params.StableChannel)
		}
	}
	if err := iter.Err(); err != nil {
		return errgo.Notef(err, "cannot iterate through entities")
	}
	err := db.BaseEntities().UpdateId(baseEntity.URL, bson.D{{
		"$set", bson.D{{
			"channelentities", channelEntities,
		}, {
			"channelacls", map[params.Channel]mongodoc.ACL{
				params.UnpublishedChannel: baseEntity.DevelopmentACLs,
				params.DevelopmentChannel: baseEntity.DevelopmentACLs,
				params.StableChannel:      baseEntity.ACLs,
			},
		}},
	}, {
		"$unset", bson.D{{
			"developmentacls", nil,
		}, {
			"acls", nil,
		}},
	}})
	if err != nil {
		return errgo.Notef(err, "cannot update base entity")
	}
	return nil
}

func setExecuted(db StoreDatabase, name mongodoc.MigrationName) error {
	if _, err := db.Migrations().Upsert(nil, bson.D{{
		"$addToSet", bson.D{{"executed", name}},
	}}); err != nil {
		return errgo.Notef(err, "cannot add %s to executed migrations", name)
	}
	return nil
}
