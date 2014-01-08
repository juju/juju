// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"launchpad.net/juju-core/store"
)

func main() {
	err := serve()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func serve() error {
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
	if conf.MongoURL == "" || conf.APIAddr == "" {
		return fmt.Errorf("missing mongo-url or api-addr in config file")
	}
	s, err := store.Open(conf.MongoURL)
	if err != nil {
		return err
	}
	defer s.Close()
	server, err := store.NewServer(s)
	if err != nil {
		return err
	}
	return http.ListenAndServe(conf.APIAddr, server)
}
