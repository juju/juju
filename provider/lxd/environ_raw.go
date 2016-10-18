// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxd

import (
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils"
	lxdshared "github.com/lxc/lxd/shared"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/juju/paths"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/common"
	"github.com/juju/juju/tools/lxdclient"
)

var (
	clientCertPath = path.Join(paths.Conf, "lxd-client.crt")
	clientKeyPath  = path.Join(paths.Conf, "lxd-client.key")
	serverCertPath = path.Join(paths.Conf, "lxd-server.crt")
)

type rawProvider struct {
	lxdCerts
	lxdConfig
	lxdInstances
	lxdProfiles
	lxdImages
	common.Firewaller
}

type lxdCerts interface {
	AddCert(lxdclient.Cert) error
	RemoveCertByFingerprint(string) error
}

type lxdConfig interface {
	ServerStatus() (*lxdshared.ServerState, error)
	SetConfig(k, v string) error
}

type lxdInstances interface {
	Instances(string, ...string) ([]lxdclient.Instance, error)
	AddInstance(lxdclient.InstanceSpec) (*lxdclient.Instance, error)
	RemoveInstances(string, ...string) error
	Addresses(string) ([]network.Address, error)
}

type lxdProfiles interface {
	CreateProfile(string, map[string]string) error
	HasProfile(string) (bool, error)
}

type lxdImages interface {
	EnsureImageExists(series string, sources []lxdclient.Remote, copyProgressHandler func(string)) error
}

func newRawProvider(spec environs.CloudSpec) (*rawProvider, error) {
	client, err := newClient(spec, ioutil.ReadFile, utils.RunCommand)
	if err != nil {
		return nil, errors.Annotate(err, "creating LXD client")
	}

	raw := &rawProvider{
		lxdCerts:     client,
		lxdConfig:    client,
		lxdInstances: client,
		lxdProfiles:  client,
		lxdImages:    client,
		Firewaller:   common.NewFirewaller(),
	}
	return raw, nil
}

type readFileFunc func(string) ([]byte, error)
type runCommandFunc func(string, ...string) (string, error)

func newClient(
	spec environs.CloudSpec,
	readFile readFileFunc,
	runCommand runCommandFunc,
) (*lxdclient.Client, error) {
	if spec.Endpoint != "" {
		// We don't handle connecting to non-local lxd at present.
		return nil, errors.NotValidf("endpoint %q", spec.Endpoint)
	}

	config, err := getRemoteConfig(readFile, runCommand)
	if errors.IsNotFound(err) {
		config = &lxdclient.Config{Remote: lxdclient.Local}
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	client, err := lxdclient.Connect(*config, true)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return client, nil
}

// getRemoteConfig returns a lxdclient.Config using a TCP-based remote
// if called from within an instance started by the LXD provider. Otherwise,
// it returns an errors satisfying errors.IsNotFound.
func getRemoteConfig(readFile readFileFunc, runCommand runCommandFunc) (*lxdclient.Config, error) {
	readFileOrig := readFile
	readFile = func(path string) ([]byte, error) {
		data, err := readFileOrig(path)
		if err != nil {
			if os.IsNotExist(err) {
				err = errors.NotFoundf("%s", path)
			}
			return nil, err
		}
		return data, nil
	}
	clientCert, err := readFile(clientCertPath)
	if err != nil {
		return nil, errors.Annotate(err, "reading client certificate")
	}
	clientKey, err := readFile(clientKeyPath)
	if err != nil {
		return nil, errors.Annotate(err, "reading client key")
	}
	serverCert, err := readFile(serverCertPath)
	if err != nil {
		return nil, errors.Annotate(err, "reading server certificate")
	}
	cert := lxdclient.NewCert(clientCert, clientKey)
	hostAddress, err := getDefaultGateway(runCommand)
	if err != nil {
		return nil, errors.Annotate(err, "getting gateway address")
	}
	return &lxdclient.Config{
		lxdclient.Remote{
			Name:          "remote",
			Host:          hostAddress,
			Protocol:      lxdclient.LXDProtocol,
			Cert:          &cert,
			ServerPEMCert: string(serverCert),
		},
	}, nil
}

func getDefaultGateway(runCommand runCommandFunc) (string, error) {
	out, err := runCommand("ip", "route", "list", "match", "0/0")
	if err != nil {
		return "", errors.Trace(err)
	}
	if !strings.HasPrefix(string(out), "default via") {
		return "", errors.Errorf(`unexpected output from "ip route": %s`, out)
	}
	fields := strings.Fields(string(out))
	return fields[2], nil
}
