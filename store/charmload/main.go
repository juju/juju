package main

import (
	"fmt"
	"io/ioutil"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/juju/log"
	"launchpad.net/juju-core/juju/store"
	"launchpad.net/lpad"
	stdlog "log"
	"os"
	"path/filepath"
)

func main() {
	log.Target = stdlog.New(os.Stdout, "", stdlog.LstdFlags)
	err := load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

type config struct {
	MongoURL string `yaml:"mongo-url"`
}

func readConfig(path string, conf interface{}) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("opening config file: %v", err)
	}
	data, err := ioutil.ReadAll(f)
	f.Close()
	if err != nil {
		return fmt.Errorf("reading config file: %v", err)
	}
	err = goyaml.Unmarshal(data, conf)
	if err != nil {
		return fmt.Errorf("processing config file: %v", err)
	}
	return nil
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
	var conf config
	err := readConfig(confPath, &conf)
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
