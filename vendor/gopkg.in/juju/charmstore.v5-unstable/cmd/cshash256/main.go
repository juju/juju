// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// This command populates the blobhash256 field of all entities.
// This command is intended to be run on the production db and then discarded.
// The first time this command is executed, all the entities are updated.
// Subsequent runs have no effect.

package main // import "gopkg.in/juju/charmstore.v5-unstable/cmd/cshash256"

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/juju/loggo"
	"gopkg.in/errgo.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"gopkg.in/juju/charmstore.v5-unstable/config"
	"gopkg.in/juju/charmstore.v5-unstable/internal/charmstore"
	"gopkg.in/juju/charmstore.v5-unstable/internal/mongodoc"
)

var (
	logger        = loggo.GetLogger("cshash256")
	loggingConfig = flag.String("logging-config", "INFO", "specify log levels for modules e.g. <root>=TRACE")
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s [options] <config path>\n", filepath.Base(os.Args[0]))
		flag.PrintDefaults()
		os.Exit(2)
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
	}
	if *loggingConfig != "" {
		if err := loggo.ConfigureLoggers(*loggingConfig); err != nil {
			fmt.Fprintf(os.Stderr, "cannot configure loggers: %v", err)
			os.Exit(1)
		}
	}
	if err := run(flag.Arg(0)); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func run(confPath string) error {
	logger.Infof("reading configuration")
	conf, err := config.Read(confPath)
	if err != nil {
		return errgo.Notef(err, "cannot read config file %q", confPath)
	}

	logger.Infof("connecting to mongo")
	session, err := mgo.Dial(conf.MongoURL)
	if err != nil {
		return errgo.Notef(err, "cannot dial mongo at %q", conf.MongoURL)
	}
	defer session.Close()
	db := session.DB("juju")

	logger.Infof("instantiating the store")
	pool, err := charmstore.NewPool(db, nil, nil, charmstore.ServerParams{})
	if err != nil {
		return errgo.Notef(err, "cannot create a new store")
	}
	store := pool.Store()
	defer store.Close()

	logger.Infof("updating entities")
	if err := update(store); err != nil {
		return errgo.Notef(err, "cannot update entities")
	}

	logger.Infof("done")
	return nil
}

func update(store *charmstore.Store) error {
	entities := store.DB.Entities()
	var entity mongodoc.Entity
	iter := entities.Find(bson.D{{"blobhash256", ""}}).Select(bson.D{{"blobname", 1}}).Iter()
	defer iter.Close()

	counter := 0
	for iter.Next(&entity) {
		// Retrieve the archive contents.
		r, _, err := store.BlobStore.Open(entity.BlobName)
		if err != nil {
			return errgo.Notef(err, "cannot open archive data for %s", entity.URL)
		}

		// Calculate the contents hash.
		hash := sha256.New()
		if _, err = io.Copy(hash, r); err != nil {
			r.Close()
			return errgo.Notef(err, "cannot calculate archive sha256 for %s", entity.URL)
		}
		r.Close()

		// Update the entity document.
		if err := entities.UpdateId(entity.URL, bson.D{{
			"$set", bson.D{{"blobhash256", fmt.Sprintf("%x", hash.Sum(nil))}},
		}}); err != nil {
			return errgo.Notef(err, "cannot update entity id %s", entity.URL)
		}
		counter++
		if counter%100 == 0 {
			logger.Infof("%d entities updated", counter)
		}

	}

	if err := iter.Close(); err != nil {
		return errgo.Notef(err, "cannot iterate entities")
	}
	logger.Infof("%d entities updated", counter)
	return nil
}
