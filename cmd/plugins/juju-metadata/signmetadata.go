// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"launchpad.net/gnuflag"
	"launchpad.net/loggo"

	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/utils"
)

var signMetadataDoc = `
sign searches for json files in the specified directory tree and inline signs
them using the private key in the specified keyring file. For each .json file, a
corresponding .sjson file is procduced.

The specified keyring file is expected to contain an amored private key. If the key
is encrypted, then the specified passphrase is used to decrypt the key.
`

// SignMetadataCommand is used to sign simplestreams metadata json files.
type SignMetadataCommand struct {
	cmd.CommandBase
	dir        string
	keyFile    string
	passphrase string
}

func (c *SignMetadataCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "sign",
		Purpose: "sign simplestreams metadata",
		Doc:     signMetadataDoc,
	}
}

func (c *SignMetadataCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	f.StringVar(&c.dir, "d", "", "directory in which to look for metadata")
	f.StringVar(&c.keyFile, "k", "", "file containing the amored private signing key")
	f.StringVar(&c.passphrase, "p", "", "passphrase used to decrypt the private key")
}

func (c *SignMetadataCommand) Init(args []string) error {
	if c.dir == "" {
		return fmt.Errorf("directory must be specified")
	}
	if c.keyFile == "" {
		return fmt.Errorf("keyfile must be specified")
	}
	return cmd.CheckEmpty(args)
}

func (c *SignMetadataCommand) Run(context *cmd.Context) error {
	loggo.RegisterWriter("signmetadata", cmd.NewCommandLogWriter("juju.plugins.metadata", context.Stdout, context.Stderr), loggo.INFO)
	defer loggo.RemoveWriter("signmetadata")
	var err error
	c.dir, err = utils.NormalizePath(c.dir)
	if err != nil {
		return err
	}
	keyData, err := ioutil.ReadFile(c.keyFile)
	if err != nil {
		return err
	}
	return process(c.dir, string(keyData), c.passphrase)
}

func process(dir, key, passphrase string) error {
	logger.Debugf("processing directory %q", dir)
	// Do any json files in dir
	filenames, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if len(filenames) > 0 {
		logger.Infof("signing %d file(s) in %q", len(filenames), dir)
	}
	for _, filename := range filenames {
		logger.Infof("signing file %q", filename)
		f, err := os.Open(filename)
		if err != nil {
			return fmt.Errorf("opening file %q: %v", filename, err)
		}
		encoded, err := simplestreams.Encode(f, key, passphrase)
		if err != nil {
			return fmt.Errorf("encoding file %q: %v", filename, err)
		}
		signedFilename := strings.Replace(filename, ".json", ".sjson", -1)
		if err = ioutil.WriteFile(signedFilename, encoded, 0644); err != nil {
			return fmt.Errorf("writing signed file %q: %v", signedFilename, err)
		}
	}
	// Now process any directories in dir.
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, f := range files {
		if f.IsDir() {
			if err = process(filepath.Join(dir, f.Name()), key, passphrase); err != nil {
				return err
			}
		}
	}
	return nil
}
