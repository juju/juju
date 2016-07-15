// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main // import "gopkg.in/juju/charmstore.v5-unstable/cmd/essync"

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/juju/loggo"
	"gopkg.in/errgo.v1"
	"gopkg.in/mgo.v2"

	"gopkg.in/juju/charmstore.v5-unstable/config"
	"gopkg.in/juju/charmstore.v5-unstable/elasticsearch"
	"gopkg.in/juju/charmstore.v5-unstable/internal/charmstore"
)

var logger = loggo.GetLogger("essync")

var (
	index         = flag.String("index", "cs", "Name of index to populate.")
	loggingConfig = flag.String("logging-config", "", "specify log levels for modules e.g. <root>=TRACE")
	mapping       = flag.String("mapping", "", "No longer used.")
	settings      = flag.String("settings", "", "No longer used.")
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
	if err := populate(flag.Arg(0)); err != nil {
		logger.Errorf("cannot populate elasticsearch: %v", err)
		os.Exit(1)
	}
}

func populate(confPath string) error {
	logger.Debugf("reading config file %q", confPath)
	conf, err := config.Read(confPath)
	if err != nil {
		return errgo.Notef(err, "cannot read config file %q", confPath)
	}
	if conf.ESAddr == "" {
		return errgo.Newf("no elasticsearch-addr specified in config file %q", confPath)
	}
	si := &charmstore.SearchIndex{
		Database: &elasticsearch.Database{
			conf.ESAddr,
		},
		Index: *index,
	}
	session, err := mgo.Dial(conf.MongoURL)
	if err != nil {
		return errgo.Notef(err, "cannot dial mongo at %q", conf.MongoURL)
	}
	defer session.Close()
	db := session.DB("juju")

	pool, err := charmstore.NewPool(db, si, nil, charmstore.ServerParams{})
	if err != nil {
		return errgo.Notef(err, "cannot create a new store")
	}
	store := pool.Store()
	defer store.Close()
	if err := store.SynchroniseElasticsearch(); err != nil {
		return errgo.Notef(err, "cannot synchronise elasticsearch")
	}
	return nil
}
