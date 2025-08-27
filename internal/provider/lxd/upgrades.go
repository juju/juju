// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"os"
	"path"

	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
	jujupaths "github.com/juju/juju/core/paths"
)

// ReadLegacyCloudCredentials reads cloud credentials off disk for an old
// LXD controller, and returns them as a cloud.Credential with the
// certificate auth-type.
//
// If the credential files are missing from the filesystem, an error
// satisfying errors.IsNotFound will be returned.
func ReadLegacyCloudCredentials(readFile func(string) ([]byte, error)) (cloud.Credential, error) {
	var (
		jujuConfDir    = jujupaths.ConfDir(jujupaths.OSUnixLike)
		clientCertPath = path.Join(jujuConfDir, "lxd-client.crt")
		clientKeyPath  = path.Join(jujuConfDir, "lxd-client.key")
		serverCertPath = path.Join(jujuConfDir, "lxd-server.crt")
	)
	readFileString := func(path string) (string, error) {
		data, err := readFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				err = errors.NotFoundf("%s", path)
			}
			return "", errors.Trace(err)
		}
		return string(data), nil
	}
	clientCert, err := readFileString(clientCertPath)
	if err != nil {
		return cloud.Credential{}, errors.Annotate(err, "reading client certificate")
	}
	clientKey, err := readFileString(clientKeyPath)
	if err != nil {
		return cloud.Credential{}, errors.Annotate(err, "reading client key")
	}
	serverCert, err := readFileString(serverCertPath)
	if err != nil {
		return cloud.Credential{}, errors.Annotate(err, "reading server certificate")
	}
	return cloud.NewCredential(cloud.CertificateAuthType, map[string]string{
		credAttrServerCert: serverCert,
		credAttrClientCert: clientCert,
		credAttrClientKey:  clientKey,
	}), nil
}
