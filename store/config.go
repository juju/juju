// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package store

import (
	"fmt"
	"io/ioutil"
	"os"

	"launchpad.net/goyaml"
)

type Config struct {
	MongoURL string `yaml:"mongo-url"`
	APIAddr  string `yaml:"api-addr"`
}

func ReadConfig(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening config file: %v", err)
	}
	defer f.Close()
	data, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %v", err)
	}
	conf := new(Config)
	err = goyaml.Unmarshal(data, conf)
	if err != nil {
		return nil, fmt.Errorf("processing config file: %v", err)
	}
	return conf, nil
}
