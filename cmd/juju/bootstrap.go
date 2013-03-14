package main

import (
	"fmt"
	"io/ioutil"
	"launchpad.net/gnuflag"
	"launchpad.net/juju-core/cert"
	"launchpad.net/juju-core/cmd"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"os"
	"path/filepath"
	"time"
)

// BootstrapCommand is responsible for launching the first machine in a juju
// environment, and setting up everything necessary to continue working.
type BootstrapCommand struct {
	EnvCommandBase
	UploadTools bool
}

func (c *BootstrapCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "bootstrap",
		Purpose: "start up an environment from scratch",
	}
}

func (c *BootstrapCommand) SetFlags(f *gnuflag.FlagSet) {
	c.EnvCommandBase.SetFlags(f)
	f.BoolVar(&c.UploadTools, "upload-tools", false, "upload local version of tools before bootstrapping")
}

func writeCertAndKeyToHome(name string, cert, key []byte) error {
	path := filepath.Join(os.Getenv("HOME"), ".juju", name)
	if err := ioutil.WriteFile(path+"-cert.pem", cert, 0644); err != nil {
		return err
	}
	if err := ioutil.WriteFile(path+"-private-key.pem", key, 0600); err != nil {
		return err
	}
	return nil
}

func generateCertificate(environ environs.Environ) error {
	cfg := environ.Config()
	caCert, caKey, err := cert.NewCA(environ.Name(), time.Now().UTC().AddDate(10, 0, 0))
	if err != nil {
		return err
	}
	m := cfg.AllAttrs()
	m["ca-cert"] = string(caCert)
	m["ca-private-key"] = string(caKey)
	cfg, err = config.New(m)
	if err != nil {
		return fmt.Errorf("cannot create environment configuration with new CA: %v", err)
	}
	if err := environ.SetConfig(cfg); err != nil {
		return fmt.Errorf("cannot set environment configuration with CA: %v", err)
	}
	if err := writeCertAndKeyToHome(environ.Name(), caCert, caKey); err != nil {
		return fmt.Errorf("cannot write CA certificate and key: %v", err)
	}
	return nil
}

func checkCertificate(environ environs.Environ) error {
	cfg := environ.Config()
	_, hasCACert := cfg.CACert()
	_, hasCAKey := cfg.CAPrivateKey()

	if hasCACert && hasCAKey {
		// All is good in the world.
		return nil
	}
	// It is not possible to create an environment that has a private key, but no certificate.
	if hasCACert && !hasCAKey {
		return fmt.Errorf("environment configuration with a certificate but no CA private key")
	}

	return generateCertificate(environ)
}

// Run connects to the environment specified on the command line and bootstraps
// a juju in that environment if none already exists. If there is as yet no environments.yaml file,
// the user is informed how to create one.
func (c *BootstrapCommand) Run(context *cmd.Context) error {
	environ, err := environs.NewFromName(c.EnvName)
	if err != nil {
		if os.IsNotExist(err) {
			out := context.Stderr
			fmt.Fprintln(out, "No juju environment configuration file exists.")
			fmt.Fprintln(out, "Please create a configuration by running:")
			fmt.Fprintln(out, "    juju init -w")
			fmt.Fprintln(out, "then edit the file to configure your juju environment.")
			fmt.Fprintln(out, "You can then re-run bootstrap.")
		}
		return err
	}

	err = checkCertificate(environ)
	if err != nil {
		return err
	}
	return environs.Bootstrap(environ, c.UploadTools)
}
