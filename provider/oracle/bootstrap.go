package oracle

import (
	"fmt"
	"os"
	"strings"

	oci "github.com/hoenirvili/go-oracle-cloud/api"
	"github.com/hoenirvili/go-oracle-cloud/response"
	"github.com/juju/errors"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/instance"
)

// launchBootstrapController creates a new vm inside
// the oracle infrastracuture and parses the response into a instance.Instance
func launchBootstrapConstroller(c *oci.Client, params []oci.InstanceParams) (instance.Instance, error) {
	if c == nil {
		return nil, errors.NotFoundf("client")
	}

	resp, err := c.CreateInstance(params)
	if err != nil {
		return nil, errors.Trace(err)
	}

	fmt.Printf("%v\n", resp)
	os.Exit(1)
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
	if c == nil {
		return "", errors.Errorf("cannot use nil error client to upload ssh keys")
	}

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

	if !resp.Enabled || resp.Key != key {
		if !resp.Enabled {
			logger.Debugf("The Enabled flag is set to false on the key, setting it to true")
		}
		if resp.Key != key {
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

	return resp.Name, nil
}

// isComplaint takes a ImageList object and finds
// if it contains the given shapName, imageVersion and imageArch
// if not it will return false
func isComplaint(
	list response.ImageList,
	shapeName, imageVersion, imageArch string,
) bool {

	i := 0
	for _, val := range list.Entries {
		if strings.Contains(
			val.Attributes.SupportedShapes,
			shapeName,
		) {
			i++
			break
		}
	}
	if strings.Contains(list.Name, imageVersion) {
		i++
	}
	if strings.Contains(list.Name, imageArch) {
		i++
	}
	if i == 3 {
		return true
	}

	return false
}

// checkImageList checks if there is a image specified in the
// oracle cloud infrstracture like one in the toolds metadata
// if there is return one that matches the tools one, return it
func checkImageList(
	c *oci.Client,
	tools []*imagemetadata.ImageMetadata,
	shape *shape,
) (string, error) {

	var (
		imageVersion string
		imageArch    string
	)

	if c == nil {
		return "", errors.NotFoundf("Cannot use nil client")
	}

	if tools == nil {
		return "", errors.NotFoundf("No tools imagemedatada provided")
	}

	// take a list of all images that are in the oracle cloud account
	resp, err := c.AllImageList()
	if err != nil {
		return "", errors.Trace(err)
	}

	// filter through all the image list and if we found one complaint with
	// the tools just return it
	for _, val := range tools {
		if len(val.Version) > 0 && len(val.Arch) > 0 {
			imageVersion = val.Version
			imageVersion = "16.10" // TODO REMOVE THIS
			imageArch = val.Arch
		}
		if imageVersion == "" || imageArch == "" {
			continue
		}
		for _, val := range resp.Result {
			if isComplaint(val, shape.name, imageVersion, imageArch) {
				// extract the name of the imagelist
				// the imagelist name contains the idenity, the username appended
				// to the actual image list name
				names := strings(name, "/")
				// so we need the last element separated with "/"
				return names[len(names)-1], nil
			}
		}
	}

	return "", errors.Errorf(
		"Cannot find any image in the oracle cloud that is complaint with the image tools",
	)

}

// extracts the name of a imagelist name
// the imagelist names contains the indenity, the username of the client and after
// the actual name of the imagelist all are separted with "/"
func theName(name string) string {
	names := strings.Split(name, "/")
	return names[len(names)-1]
}
