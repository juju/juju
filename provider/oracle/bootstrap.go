// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package oracle

import (
	"strings"

	oci "github.com/hoenirvili/go-oracle-cloud/api"

	"github.com/juju/errors"
	"github.com/juju/juju/instance"
)

// createInstance creates a new vm inside
// the oracle infrastracuture and parses  the response into a instance.Instance
func createInstance(c *oci.Client, params oci.InstanceParams) (instance.Instance, error) {
	if len(params.Instances) > 1 {
		return nil, errors.NotSupportedf("launching multiple controllers")
	}

	logger.Infof("Launching tbe bootstrap creation method")

	// make the api request to create the instance
	resp, err := c.CreateInstance(params)
	if err != nil {
		return nil, errors.Trace(err)
	}

	instance, err := newInstance(resp)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return instance, nil
}

// uploadSSHControllerKeys checks if the oracle api has the present key if not
// then it will upload it, if there's arealdy the key we should check if the key
// there maches the key we want to add, if not then we should modify the content
// in order to be sure we are using the correct key
func uploadSSHControllerKeys(c *oci.Client, key string) (string, error) {
	if len(key) == 0 {
		return "", errors.Errorf("cannot use empty key in order to upload it")
	}

	resp, err := c.SSHKeyDetails("juju-client-key")
	if err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			// the keys does not exist, we should upload them
			resp, err = c.AddSHHKey("juju-client-key", key, true)
			if err != nil {
				return "", errors.Errorf("cannot upload the juju client keys")
			}
		} else {
			return "", errors.Errorf("cannot get details of the juju-client-key")
		}
	}

	keyrsponse := key[:strings.Index(key, "juju-client-key")-1]
	if !resp.Enabled || resp.Key != keyrsponse {
		if !resp.Enabled {
			logger.Debugf("The Enabled flag is set to false on the key, setting it to true")
		}
		if resp.Key != keyrsponse {
			logger.Debugf("The key provided with the oracle one does not match, update the key content")
		}
		resp, err = c.UpdateSSHKey("juju-client-key", key, true)
		if err != nil {
			return "", errors.Errorf("cannot enable of update the juju-client-key content")
		}
		if !resp.Enabled {
			return "", errors.Errorf("juju client key is not enabled after previously tried to enable")
		}
	}

	list := strings.Split(resp.Name, "/")
	name := list[len(list)-1]
	return name, nil
}
