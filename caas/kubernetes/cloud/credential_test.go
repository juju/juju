// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"io/ioutil"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	k8scloud "github.com/juju/juju/caas/kubernetes/cloud"
	"github.com/juju/juju/cloud"
)

type credentialSuite struct {
}

var _ = gc.Suite(&credentialSuite{})

func (s *credentialSuite) TestValidCredentials(c *gc.C) {
	tests := []struct {
		AuthInfo   *clientcmdapi.AuthInfo
		AuthType   cloud.AuthType
		Attributes map[string]string
		Name       string
		PreSetup   func(*clientcmdapi.AuthInfo) error
	}{
		{
			AuthInfo: &clientcmdapi.AuthInfo{
				ClientCertificateData: []byte("cert-data"),
				ClientKeyData:         []byte("cert-key-data"),
			},
			AuthType: cloud.CertificateAuthType,
			Attributes: map[string]string{
				"ClientCertificateData": "cert-data",
				"ClientKeyData":         "cert-key-data",
			},
			Name: "client-cert-data",
		},
		{
			AuthInfo: &clientcmdapi.AuthInfo{},
			AuthType: cloud.CertificateAuthType,
			Attributes: map[string]string{
				"ClientCertificateData": "cert-data",
				"ClientKeyData":         "cert-key-data",
			},
			Name: "client-cert-data-from-file",
			PreSetup: func(a *clientcmdapi.AuthInfo) error {
				certFile, err := ioutil.TempFile("", "")
				if err != nil {
					return err
				}
				_, err = certFile.WriteString("cert-data")
				if err != nil {
					return err
				}
				certKeyFile, err := ioutil.TempFile("", "")
				if err != nil {
					return err
				}
				_, err = certKeyFile.WriteString("cert-key-data")
				if err != nil {
					return err
				}

				a.ClientCertificate = certFile.Name()
				a.ClientKey = certKeyFile.Name()
				certFile.Close()
				certKeyFile.Close()
				return nil
			},
		},
		{
			AuthInfo: &clientcmdapi.AuthInfo{
				Token: "wef44t34f23",
			},
			AuthType: cloud.OAuth2AuthType,
			Attributes: map[string]string{
				"Token": "wef44t34f23",
			},
			Name: "token",
		},
		{
			AuthInfo: &clientcmdapi.AuthInfo{},
			AuthType: cloud.OAuth2AuthType,
			Attributes: map[string]string{
				"Token": "wef44t34f23",
			},
			Name: "token-from-file",
			PreSetup: func(a *clientcmdapi.AuthInfo) error {
				tokenFile, err := ioutil.TempFile("", "")
				if err != nil {
					return err
				}
				_, err = tokenFile.WriteString("wef44t34f23")
				if err != nil {
					return err
				}

				a.TokenFile = tokenFile.Name()
				tokenFile.Close()
				return nil
			},
		},
		{
			AuthInfo: &clientcmdapi.AuthInfo{
				Username: "tlm",
				Password: "top-secret",
			},
			AuthType: cloud.UserPassAuthType,
			Attributes: map[string]string{
				"username": "tlm",
				"password": "top-secret",
			},
			Name: "username-password",
		},
	}

	for _, test := range tests {
		if test.PreSetup != nil {
			err := test.PreSetup(test.AuthInfo)
			c.Assert(err, jc.ErrorIsNil)
		}
		cred, err := k8scloud.CredentialFromAuthInfo(test.Name, test.AuthInfo)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(cred.AuthType(), gc.Equals, test.AuthType)
		c.Assert(cred.Attributes(), jc.DeepEquals, test.Attributes)
	}
}

func (s *credentialSuite) TestUnsupportedCredentials(c *gc.C) {
	authInfo := &clientcmdapi.AuthInfo{
		ClientKeyData: []byte("test"),
	}

	_, err := k8scloud.CredentialFromAuthInfo("unsupported", authInfo)
	c.Assert(err.Error(), gc.Equals, "configuration for \"unsupported\" not supported")
}
