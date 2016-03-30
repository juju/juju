// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils"
	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/clearsign"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/juju"
)

type updateCloudsCommand struct {
	cmd.CommandBase

	publicSigningKey string
	publicCloudURL   string
}

var updateCloudsDoc = `
If any new information for public clouds (such as regions and connection
endpoints) are available this command will update Juju accordingly. It is
suggested to run this command periodically.

Examples:

    juju update-clouds

See also: list-clouds
`

// NewUpdateCloudsCommand returns a command to update cloud information.
func NewUpdateCloudsCommand() cmd.Command {
	return &updateCloudsCommand{
		publicSigningKey: juju.JujuPublicKey,
		publicCloudURL:   "https://streams.canonical.com/juju/public-clouds.syaml",
	}
}

func (c *updateCloudsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "update-clouds",
		Purpose: "Updates public cloud information available to Juju.",
		Doc:     updateCloudsDoc,
	}
}

func (c *updateCloudsCommand) Run(ctxt *cmd.Context) error {
	fmt.Fprint(ctxt.Stdout, "Fetching latest public cloud list... ")
	client := utils.GetHTTPClient(utils.VerifySSLHostnames)
	resp, err := client.Get(c.publicCloudURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	noNewClouds := "\nYour cloud database is up to date, see juju list-clouds."
	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusNotFound:
			fmt.Fprintln(ctxt.Stdout, noNewClouds)
			return nil
		case http.StatusUnauthorized:
			return errors.Unauthorizedf("unauthorised access to URL %q", c.publicCloudURL)
		}
		return fmt.Errorf("cannot read public cloud information at URL %q, %q", c.publicCloudURL, resp.Status)
	}

	cloudData, err := decodeCheckSignature(resp.Body, c.publicSigningKey)
	if err != nil {
		return errors.Annotate(err, "error receiving updated cloud data")
	}
	newPublicClouds, err := jujucloud.ParseCloudMetadata(cloudData)
	if err != nil {
		return errors.Annotate(err, "invalid cloud data received when updating clouds")
	}
	currentPublicClouds, _, err := jujucloud.PublicCloudMetadata(jujucloud.JujuPublicCloudsPath())
	if err != nil {
		return errors.Annotate(err, "invalid local public cloud data")
	}
	sameCloudInfo, err := jujucloud.IsSameCloudMetadata(newPublicClouds, currentPublicClouds)
	if err != nil {
		// Should never happen.
		return err
	}
	if sameCloudInfo {
		fmt.Fprintln(ctxt.Stdout, noNewClouds)
		return nil
	}
	if err := jujucloud.WritePublicCloudMetadata(newPublicClouds); err != nil {
		return errors.Annotate(err, "error writing new local public cloud data")
	}
	fmt.Fprintln(ctxt.Stdout, "done.")
	return nil
}

func decodeCheckSignature(r io.Reader, publicKey string) ([]byte, error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}
	b, _ := clearsign.Decode(data)
	if b == nil {
		return nil, errors.New("no PGP signature embedded in plain text data")
	}
	keyring, err := openpgp.ReadArmoredKeyRing(bytes.NewBufferString(publicKey))
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %v", err)
	}

	_, err = openpgp.CheckDetachedSignature(keyring, bytes.NewBuffer(b.Bytes), b.ArmoredSignature.Body)
	if err != nil {
		return nil, err
	}
	return b.Plaintext, nil
}
