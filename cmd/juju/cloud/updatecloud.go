// Copyright 2019 Canonical Ltd.
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
	"github.com/juju/gnuflag"
	"github.com/juju/utils"
	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/clearsign"
	"gopkg.in/juju/names.v3"

	cloudapi "github.com/juju/juju/api/cloud"
	jujucloud "github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/juju/keys"
	"github.com/juju/juju/jujuclient"
)

type updateCloudCommand struct {
	modelcmd.OptionalControllerCommand

	cloudMetadataStore CloudMetadataStore

	// Cloud is the name of the cloud to update
	Cloud string

	// CloudFile is the name of the cloud YAML file
	CloudFile string

	// Used when updating controllers' cloud details
	updateCloudAPIFunc func() (updateCloudAPI, error)

	publicCloudFetchConfig publicCloudsConfig
}

var updateCloudDoc = `
Update cloud information on this client and/or on a controller.

A cloud can be updated from a file. This requires a <cloud name> and a yaml file
containing the cloud details. 
This method can be used for cloud updates on the client side and on a controller. 

A cloud on the controller can also be updated just by using a name of a cloud
from this client.

Use --controller option to update a cloud on a controller. 

Use --client to update cloud definition on this client.

Definitions of 'public' clouds such as aws, azure, etc are stored in a 
central, Juju accessible location on https://streams.canonical.com/.

However, for convenience, each Juju client has a copy of these definitions.
Thus, when public cloud definitions change, for example a new region gets added,
a client copy might become outdated. If this client copy was used on 
a controller, than a controller copy will also become outdated.

Both client and controller copies of public clouds can be updated using this command.
However, how the command runs in each case is slightly different:

* when updating 'public' clouds on a client (--client), Juju will use central location from above;

* when updating 'public' clouds on a controller (-c/--controller), Juju will use local client copy.

You can combine these options together to update controller copy
from central location 'juju update-cloud aws --client -c mycontroller' since in this combination
client copy will get updated first.

Examples:

    juju update-cloud mymaas -f path/to/maas.yaml
    juju update-cloud mymaas -f path/to/maas.yaml --controller mycontroller
    juju update-cloud mymaas --controller mycontroller
    juju update-cloud mymaas --client --controller mycontroller
    juju update-cloud mymaas --client -f path/to/maas.yaml

See also:
    add-cloud
    remove-cloud
    list-clouds
`

type updateCloudAPI interface {
	UpdateCloud(jujucloud.Cloud) error
	Close() error
}

// NewUpdateCloudCommand returns a command to update cloud information.
var NewUpdateCloudCommand = func(cloudMetadataStore CloudMetadataStore) cmd.Command {
	return newUpdateCloudCommand(cloudMetadataStore)
}

func newUpdateCloudCommand(cloudMetadataStore CloudMetadataStore) cmd.Command {
	store := jujuclient.NewFileClientStore()
	c := &updateCloudCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{
			Store: store,
		},
		cloudMetadataStore:     cloudMetadataStore,
		publicCloudFetchConfig: newPublicCloudsConfig(),
	}
	c.updateCloudAPIFunc = c.updateCloudAPI

	return modelcmd.WrapBase(c)
}

type publicCloudsConfig struct {
	publicSigningKey string
	publicCloudURL   string
}

func newPublicCloudsConfig() publicCloudsConfig {
	return publicCloudsConfig{
		publicSigningKey: keys.JujuPublicKey,
		publicCloudURL:   "https://streams.canonical.com/juju/public-clouds.syaml",
	}
}

func (c *updateCloudCommand) updateCloudAPI() (updateCloudAPI, error) {
	root, err := c.NewAPIRoot(c.Store, c.ControllerName, "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	return cloudapi.NewClient(root), nil
}

// Init populates the command with the args from the command line.
func (c *updateCloudCommand) Init(args []string) error {
	if err := c.OptionalControllerCommand.Init(args); err != nil {
		return err
	}
	if len(args) < 1 {
		return errors.BadRequestf("cloud name required")
	}

	c.Cloud = args[0]
	if ok := names.IsValidCloud(c.Cloud); !ok {
		return errors.NotValidf("cloud name %q", c.Cloud)
	}
	if err := cmd.CheckEmpty(args[1:]); err != nil {
		return err
	}
	return nil
}

func (c *updateCloudCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "update-cloud",
		Args:    "<cloud name>",
		Purpose: "Updates cloud information available to Juju.",
		Doc:     updateCloudDoc,
	})
}

func (c *updateCloudCommand) SetFlags(f *gnuflag.FlagSet) {
	c.OptionalControllerCommand.SetFlags(f)
	f.StringVar(&c.CloudFile, "f", "", "The path to a cloud definition file")
}

func (c *updateCloudCommand) Run(ctxt *cmd.Context) error {
	var newCloud *jujucloud.Cloud
	if c.CloudFile != "" {
		r := &cloudFileReader{
			cloudMetadataStore: c.cloudMetadataStore,
			cloudName:          c.Cloud,
		}
		var err error
		if newCloud, err = r.readCloudFromFile(c.CloudFile, ctxt); err != nil {
			return errors.Annotatef(err, "could not read cloud definition from provided file")
		}
		c.Cloud = r.cloudName
	}
	if err := c.MaybePrompt(ctxt, fmt.Sprintf("update cloud %q on", c.Cloud)); err != nil {
		return errors.Trace(err)
	}
	var returnErr error
	processErr := func(err error, successMsg string) {
		if err != nil {
			ctxt.Infof("ERROR %v", err)
			returnErr = cmd.ErrSilent
			return
		}
		ctxt.Infof(successMsg)
	}
	if c.Client {
		if c.CloudFile != "" {
			err := addLocalCloud(c.cloudMetadataStore, *newCloud)
			processErr(err, fmt.Sprintf("Cloud %q updated on this client using provided file.", c.Cloud))
		} else {
			if err := c.maybeUpdatePublicCloud(ctxt); err != nil {
				ctxt.Infof("ERROR %v", err)
				returnErr = cmd.ErrSilent
			}
		}
	}
	if c.ControllerName != "" {
		if c.CloudFile != "" {
			err := c.updateController(newCloud)
			processErr(err, fmt.Sprintf("Cloud %q updated on controller %q using provided file.", c.Cloud, c.ControllerName))
		} else {
			err := c.updateControllerCacheFromLocalCache()
			processErr(err, fmt.Sprintf("Cloud %q updated on controller %q using client cloud definition.", c.Cloud, c.ControllerName))
		}
	}
	return returnErr
}

func getPublishedPublicClouds(ctxt *cmd.Context, cfg publicCloudsConfig) (map[string]jujucloud.Cloud, error) {
	fmt.Fprint(ctxt.Stderr, "Fetching latest public cloud list...\n")
	client := utils.GetHTTPClient(utils.VerifySSLHostnames)
	resp, err := client.Get(cfg.publicCloudURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusNotFound:
			fmt.Fprintln(ctxt.Stderr, "Public cloud list is unavailable right now.")
			return nil, cmd.ErrSilent
		case http.StatusUnauthorized:
			return nil, errors.Unauthorizedf("unauthorised access to URL %q", cfg.publicCloudURL)
		}
		return nil, errors.Errorf("cannot read public cloud information at URL %q, %q", cfg.publicCloudURL, resp.Status)
	}

	cloudData, err := decodeCheckSignature(resp.Body, cfg.publicSigningKey)
	if err != nil {
		return nil, errors.Annotate(err, "error receiving updated cloud data")
	}
	newPublicClouds, err := jujucloud.ParseCloudMetadata(cloudData)
	if err != nil {
		return nil, errors.Annotate(err, "invalid cloud data received when updating clouds")
	}
	return newPublicClouds, nil
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
		return nil, errors.Errorf("failed to parse public key: %v", err)
	}

	_, err = openpgp.CheckDetachedSignature(keyring, bytes.NewBuffer(b.Bytes), b.ArmoredSignature.Body)
	if err != nil {
		return nil, err
	}
	return b.Plaintext, nil
}

func (c *updateCloudCommand) maybeUpdatePublicCloud(ctxt *cmd.Context) error {
	// Public clouds get special treatment - they need to be fetched from streams.
	currentPublicClouds, _, err := jujucloud.PublicCloudMetadata(jujucloud.JujuPublicCloudsPath())
	if err != nil {
		return errors.Trace(err)
	}
	currentCloud, ok := currentPublicClouds[c.Cloud]
	if !ok {
		ctxt.Infof("To update cloud %q on this client, a cloud definition file is required.", c.Cloud)
		return nil
	}
	publishedPublic, err := getPublishedPublicClouds(ctxt, c.publicCloudFetchConfig)
	if err != nil {
		return errors.Annotate(err, "invalid cloud data received when updating clouds")
	}
	publishedCloud, ok := publishedPublic[c.Cloud]
	if !ok {
		// Highly unlikely that 'it will ever happen', but clouds may become decommissioned...
		return errors.Errorf("published clouds no longer have a definition for cloud %q", c.Cloud)
	}
	if !cloudChanged(c.Cloud, publishedCloud, currentCloud) {
		ctxt.Infof("No new details for cloud %q have been published.", c.Cloud)
		return nil
	}
	currentPublicClouds[c.Cloud] = publishedCloud
	if err := jujucloud.WritePublicCloudMetadata(currentPublicClouds); err != nil {
		return errors.Annotatef(err, "error writing new published defintion for cloud %q", c.Cloud)
	}
	diff := newChanges()
	diffCloudDetails(c.Cloud, publishedCloud, currentCloud, diff)
	fmt.Fprintln(ctxt.Stderr, fmt.Sprintf("Updated public cloud %q on this client, %s", c.Cloud, diff.summary()))
	return nil
}

func cloudChanged(cloudName string, new, old jujucloud.Cloud) bool {
	same, _ := jujucloud.IsSameCloudMetadata(
		map[string]jujucloud.Cloud{cloudName: new},
		map[string]jujucloud.Cloud{cloudName: old},
	)
	// If both old and new version are the same the cloud has not changed.
	return !same
}

func (c *updateCloudCommand) updateLocalCache(ctxt *cmd.Context, newCloud *jujucloud.Cloud) error {
	if err := addLocalCloud(c.cloudMetadataStore, *newCloud); err != nil {
		return err
	}
	return nil
}

func (c *updateCloudCommand) updateControllerCacheFromLocalCache() error {
	newCloud, err := cloudFromLocal(c.Store, c.Cloud)
	if err != nil {
		return errors.Trace(err)
	}
	return c.updateController(newCloud)
}

func (c updateCloudCommand) updateController(cloud *jujucloud.Cloud) error {
	api, err := c.updateCloudAPIFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer api.Close()
	err = api.UpdateCloud(*cloud)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
