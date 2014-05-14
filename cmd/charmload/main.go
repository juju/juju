// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"launchpad.net/lpad"

	"launchpad.net/juju-core/store"
)

func main() {
	err := load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func load() error {
	var confPath string
	if len(os.Args) == 2 {
		if _, err := os.Stat(os.Args[1]); err == nil {
			confPath = os.Args[1]
		}
	}
	if confPath == "" {
		return fmt.Errorf("usage: %s <config path>", filepath.Base(os.Args[0]))
	}
	conf, err := store.ReadConfig(confPath)
	if err != nil {
		return err
	}
	if conf.MongoURL == "" {
		return fmt.Errorf("missing mongo-url in config file")
	}
	s, err := store.Open(conf.MongoURL)
	if err != nil {
		return err
	}
	defer s.Close()
	err = store.PublishCharmsDistro(s, lpad.Production)
	if _, ok := err.(store.PublishBranchErrors); ok {
		// Ignore branch errors since they're commonplace here.
		// They're logged, though.
		return nil
	}
	return err
}
