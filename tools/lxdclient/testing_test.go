// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"crypto/x509"
	"os"

	"github.com/juju/errors"
	"github.com/juju/testing"
	"github.com/lxc/lxd"
	"github.com/lxc/lxd/shared"
	gc "gopkg.in/check.v1"
)

type BaseSuite struct {
	testing.IsolationSuite

	Stub   *testing.Stub
	Client *stubClient
	Cert   *Cert
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.Stub = &testing.Stub{}
	s.Client = &stubClient{stub: s.Stub}
	s.Cert = &Cert{
		Name:    "some cert",
		CertPEM: []byte("<a valid PEM-encoded x.509 cert>"),
		KeyPEM:  []byte("<a valid PEM-encoded x.509 key>"),
	}
}

type stubClient struct {
	stub *testing.Stub

	Instance   *shared.ContainerState
	Instances  []shared.ContainerInfo
	ReturnCode int
	Response   *lxd.Response
}

func (s *stubClient) WaitForSuccess(waitURL string) error {
	s.stub.AddCall("WaitForSuccess", waitURL)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *stubClient) SetServerConfig(key string, value string) (*lxd.Response, error) {
	s.stub.AddCall("SetServerConfig", key, value)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.Response, nil
}

func (s *stubClient) CertificateAdd(cert *x509.Certificate, name string) error {
	s.stub.AddCall("CertificateAdd", cert, name)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

func (s *stubClient) ContainerState(name string) (*shared.ContainerState, error) {
	s.stub.AddCall("ContainerState", name)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.Instance, nil
}

func (s *stubClient) ListContainers() ([]shared.ContainerInfo, error) {
	s.stub.AddCall("ListContainers")
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.Instances, nil
}

func (s *stubClient) Init(name, remote, image string, profiles *[]string, ephem bool) (*lxd.Response, error) {
	s.stub.AddCall("AddInstance", name, remote, image, profiles, ephem)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.Response, nil
}

func (s *stubClient) Delete(name string) (*lxd.Response, error) {
	s.stub.AddCall("Delete", name)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.Response, nil
}

func (s *stubClient) Action(name string, action shared.ContainerAction, timeout int, force bool) (*lxd.Response, error) {
	s.stub.AddCall("Action", name, action, timeout, force)
	if err := s.stub.NextErr(); err != nil {
		return nil, errors.Trace(err)
	}

	return s.Response, nil
}

func (s *stubClient) Exec(name string, cmd []string, env map[string]string, stdin *os.File, stdout *os.File, stderr *os.File) (int, error) {
	s.stub.AddCall("Exec", name, cmd, env, stdin, stdout, stderr)
	if err := s.stub.NextErr(); err != nil {
		return -1, errors.Trace(err)
	}

	return s.ReturnCode, nil
}

func (s *stubClient) SetContainerConfig(name, key, value string) error {
	s.stub.AddCall("SetContainerConfig", name, key, value)
	if err := s.stub.NextErr(); err != nil {
		return errors.Trace(err)
	}

	return nil
}
