// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"sort"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jujuhttp "github.com/juju/http/v2"
	"github.com/juju/names/v4"
	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/clearsign"

	cloudapi "github.com/juju/juju/api/client/cloud"
	jujucloud "github.com/juju/juju/cloud"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/juju/keys"
	"github.com/juju/juju/jujuclient"
)

type updatePublicCloudsCommand struct {
	modelcmd.OptionalControllerCommand

	publicSigningKey string
	publicCloudURL   string

	addCloudAPIFunc func() (updatePublicCloudAPI, error)
}

var updatePublicCloudsDoc = `
If any new information for public clouds (such as regions and connection
endpoints) are available this command will update Juju accordingly. It is
suggested to run this command periodically.

Use --controller option to update public cloud(s) on a controller. The command
will only update the clouds that a controller knows about. 

Use --client to update a definition of public cloud(s) on this client.

Examples:

    juju update-public-clouds
    juju update-public-clouds --client
    juju update-public-clouds --controller mycontroller

See also:
    clouds
`

// NewUpdatePublicCloudsCommand returns a command to update cloud information.
var NewUpdatePublicCloudsCommand = func() cmd.Command {
	return newUpdatePublicCloudsCommand()
}

func newUpdatePublicCloudsCommand() cmd.Command {
	store := jujuclient.NewFileClientStore()
	access := PublicCloudsAccess()
	c := &updatePublicCloudsCommand{
		OptionalControllerCommand: modelcmd.OptionalControllerCommand{
			Store: store,
		},
		publicSigningKey: access.publicSigningKey,
		publicCloudURL:   access.publicCloudURL,
	}
	c.addCloudAPIFunc = c.cloudAPI
	return modelcmd.WrapBase(c)
}

func (c *updatePublicCloudsCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "update-public-clouds",
		Purpose: "Updates public cloud information available to Juju.",
		Doc:     updatePublicCloudsDoc,
	})
}

type PublicCloudsAccessDetails struct {
	publicSigningKey string
	publicCloudURL   string
}

// PublicCloudsAccess contains information about
// where to find published public clouds details.
func PublicCloudsAccess() PublicCloudsAccessDetails {
	return PublicCloudsAccessDetails{
		publicSigningKey: keys.JujuPublicKey,
		publicCloudURL:   "https://streams.canonical.com/juju/public-clouds.syaml",
	}
}

// Init populates the command with the args from the command line.
func (c *updatePublicCloudsCommand) Init(args []string) error {
	if err := c.OptionalControllerCommand.Init(args); err != nil {
		return err
	}
	return cmd.CheckEmpty(args)
}

func PublishedPublicClouds(url, key string) (map[string]jujucloud.Cloud, error) {
	client := jujuhttp.NewClient(
		jujuhttp.WithLogger(logger.ChildWithLabels("http", corelogger.HTTP)),
	)
	resp, err := client.Get(context.TODO(), url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusNotFound:
			return nil, errors.Errorf("public cloud list is unavailable right now")
		case http.StatusUnauthorized:
			return nil, errors.Unauthorizedf("unauthorised access to URL %q", url)
		}
		return nil, errors.Errorf("cannot read public cloud information at URL %q, %q", url, resp.Status)
	}

	cloudData, err := decodeCheckSignature(resp.Body, key)
	if err != nil {
		return nil, errors.Annotate(err, "receiving updated cloud data")
	}
	clouds, err := jujucloud.ParseCloudMetadata(cloudData)
	if err != nil {
		return nil, errors.Annotate(err, "invalid cloud data received when updating clouds")
	}
	return clouds, nil
}

func (c *updatePublicCloudsCommand) Run(ctxt *cmd.Context) error {
	if err := c.MaybePrompt(ctxt, "update public clouds on"); err != nil {
		return errors.Trace(err)
	}
	fmt.Fprint(ctxt.Stderr, "Fetching latest public cloud list...\n")
	var returnedErr error
	publishedClouds, msg, err := FetchAndMaybeUpdatePublicClouds(
		PublicCloudsAccessDetails{
			publicSigningKey: c.publicSigningKey,
			publicCloudURL:   c.publicCloudURL,
		},
		c.Client)
	if err != nil {
		// Since FetchAndMaybeUpdatePublicClouds retrieves public clouds
		// as well as updates client copy of the clouds, it is
		// possible that the returned error is related to clouds retrieval.
		// If there are no public clouds returned, we can assume that the
		// retrieval itself was unsuccessful and abort further processing.
		if len(publishedClouds) == 0 {
			return errors.Trace(err)
		}
		ctxt.Infof("ERROR %v", err)
		returnedErr = cmd.ErrSilent
	}
	if msg != "" {
		ctxt.Infof("%s", msg)
	}
	if c.ControllerName != "" {
		if err := c.updateControllerCopy(ctxt, publishedClouds); err != nil {
			ctxt.Infof("ERROR %v", err)
			returnedErr = cmd.ErrSilent
		}
	}
	return returnedErr
}

// FetchAndMaybeUpdatePublicClouds gets published public clouds information
// and updates client copy of public clouds if desired.
// This call returns discovered public clouds and a user-facing message
// whether they are different with what was known prior to the call.
// Since this call can also update a client copy of clouds, it is possible that the public
// clouds have been retrieved but the client update fail. In this case, we still
// return public clouds as well as the client error.
var FetchAndMaybeUpdatePublicClouds = func(access PublicCloudsAccessDetails, updateClient bool) (map[string]jujucloud.Cloud, string, error) {
	var msg string
	publishedClouds, err := PublishedPublicClouds(access.publicCloudURL, access.publicSigningKey)
	if err != nil {
		return nil, msg, errors.Trace(err)
	}
	if updateClient {
		if msg, err = updateClientCopy(publishedClouds); err != nil {
			return publishedClouds, msg, err
		}
	}
	return publishedClouds, msg, nil
}

func updateClientCopy(publishedClouds map[string]jujucloud.Cloud) (string, error) {
	currentPublicClouds, _, err := jujucloud.PublicCloudMetadata(jujucloud.JujuPublicCloudsPath())
	if err != nil {
		return "", errors.Annotate(err, "invalid local public cloud data")
	}
	sameCloudInfo, err := jujucloud.IsSameCloudMetadata(publishedClouds, currentPublicClouds)
	if err != nil {
		// Should never happen.
		return "", err
	}
	if sameCloudInfo {
		return "List of public clouds on this client is up to date, see `juju clouds --client`.\n", nil
	}
	if err := jujucloud.WritePublicCloudMetadata(publishedClouds); err != nil {
		return "", errors.Annotate(err, "error writing new local public cloud data")
	}
	updateDetails := diffClouds(publishedClouds, currentPublicClouds)
	return fmt.Sprintf("Updated list of public clouds on this client, %s\n", updateDetails), nil
}

func (c *updatePublicCloudsCommand) updateControllerCopy(ctxt *cmd.Context, publishedClouds map[string]jujucloud.Cloud) error {
	api, err := c.addCloudAPIFunc()
	if err != nil {
		return errors.Trace(err)
	}
	defer api.Close()

	allClouds, err := api.Clouds()
	if err != nil {
		return errors.Trace(err)
	}
	oldCopies, newCopies := map[string]jujucloud.Cloud{}, map[string]jujucloud.Cloud{}
	var updatedAny bool
	for cloudTag, currentCopy := range allClouds {
		cloudName := cloudTag.Id()
		updatedCopy, ok := publishedClouds[cloudName]
		if !ok {
			continue
		}
		if !cloudChanged(cloudName, updatedCopy, currentCopy) {
			continue
		}
		oldCopies[cloudName] = currentCopy
		newCopies[cloudName] = updatedCopy
		if err := api.UpdateCloud(updatedCopy); err != nil {
			fmt.Fprintln(ctxt.Stderr, fmt.Sprintf("ERROR updating public cloud data on controller %q: %v", c.ControllerName, err))
			continue
		}
		updatedAny = true
	}
	if !updatedAny {
		fmt.Fprintln(ctxt.Stderr, fmt.Sprintf("List of public clouds on controller %q is up to date, see `juju clouds --controller %v`.", c.ControllerName, c.ControllerName))
		return nil
	}
	updateDetails := diffClouds(newCopies, oldCopies)
	fmt.Fprintln(ctxt.Stderr, fmt.Sprintf("Updated list of public clouds on controller %q, %s", c.ControllerName, updateDetails))
	return nil
}

type updatePublicCloudAPI interface {
	updateCloudAPI
	Clouds() (map[names.CloudTag]jujucloud.Cloud, error)
}

func (c *updatePublicCloudsCommand) cloudAPI() (updatePublicCloudAPI, error) {
	root, err := c.NewAPIRoot(c.Store, c.ControllerName, "")
	if err != nil {
		return nil, errors.Trace(err)
	}
	return cloudapi.NewClient(root), nil
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

func diffClouds(newClouds, oldClouds map[string]jujucloud.Cloud) string {
	diff := newChanges()
	// added and updated clouds
	for cloudName, cloud := range newClouds {
		oldCloud, ok := oldClouds[cloudName]
		if !ok {
			diff.addChange(addChange, cloudScope, cloudName)
			continue
		}

		if cloudChanged(cloudName, cloud, oldCloud) {
			diffCloudDetails(cloudName, cloud, oldCloud, diff)
		}
	}

	// deleted clouds
	for cloudName := range oldClouds {
		if _, ok := newClouds[cloudName]; !ok {
			diff.addChange(deleteChange, cloudScope, cloudName)
		}
	}
	return diff.summary()
}

func cloudChanged(cloudName string, new, old jujucloud.Cloud) bool {
	same, _ := jujucloud.IsSameCloudMetadata(
		map[string]jujucloud.Cloud{cloudName: new},
		map[string]jujucloud.Cloud{cloudName: old},
	)
	// If both old and new version are the same the cloud is not changed.
	return !same
}

func diffCloudDetails(cloudName string, new, old jujucloud.Cloud, diff *changes) {
	sameAuthTypes := func() bool {
		if len(old.AuthTypes) != len(new.AuthTypes) {
			return false
		}
		newAuthTypes := set.NewStrings()
		for _, one := range new.AuthTypes {
			newAuthTypes.Add(string(one))
		}

		for _, anOldOne := range old.AuthTypes {
			if !newAuthTypes.Contains(string(anOldOne)) {
				return false
			}
		}
		return true
	}

	endpointChanged := new.Endpoint != old.Endpoint
	identityEndpointChanged := new.IdentityEndpoint != old.IdentityEndpoint
	storageEndpointChanged := new.StorageEndpoint != old.StorageEndpoint

	if endpointChanged || identityEndpointChanged || storageEndpointChanged || new.Type != old.Type || !sameAuthTypes() {
		diff.addChange(updateChange, attributeScope, cloudName)
	}

	formatCloudRegion := func(rName string) string {
		return fmt.Sprintf("%v/%v", cloudName, rName)
	}

	oldRegions := mapRegions(old.Regions)
	newRegions := mapRegions(new.Regions)
	// added & modified regions
	for newName, newRegion := range newRegions {
		oldRegion, ok := oldRegions[newName]
		if !ok {
			diff.addChange(addChange, regionScope, formatCloudRegion(newName))
			continue

		}
		if (oldRegion.Endpoint != newRegion.Endpoint) || (oldRegion.IdentityEndpoint != newRegion.IdentityEndpoint) || (oldRegion.StorageEndpoint != newRegion.StorageEndpoint) {
			diff.addChange(updateChange, regionScope, formatCloudRegion(newName))
		}
	}

	// deleted regions
	for oldName := range oldRegions {
		if _, ok := newRegions[oldName]; !ok {
			diff.addChange(deleteChange, regionScope, formatCloudRegion(oldName))
		}
	}
}

func mapRegions(regions []jujucloud.Region) map[string]jujucloud.Region {
	result := make(map[string]jujucloud.Region)
	for _, region := range regions {
		result[region.Name] = region
	}
	return result
}

type changeType string

const (
	addChange    changeType = "added"
	deleteChange changeType = "deleted"
	updateChange changeType = "changed"
)

type scope string

const (
	cloudScope     scope = "cloud"
	regionScope    scope = "cloud region"
	attributeScope scope = "cloud attribute"
)

type changes struct {
	all map[changeType]map[scope][]string
}

func newChanges() *changes {
	return &changes{make(map[changeType]map[scope][]string)}
}

func (c *changes) addChange(aType changeType, entity scope, details string) {
	byType, ok := c.all[aType]
	if !ok {
		byType = make(map[scope][]string)
		c.all[aType] = byType
	}
	byType[entity] = append(byType[entity], details)
}

func (c *changes) summary() string {
	if len(c.all) == 0 {
		return ""
	}

	// Sort by change types
	types := []string{}
	for one := range c.all {
		types = append(types, string(one))
	}
	sort.Strings(types)

	msgs := []string{}
	details := ""
	tabSpace := "    "
	detailsSeparator := fmt.Sprintf("\n%v%v- ", tabSpace, tabSpace)
	for _, aType := range types {
		typeGroup := c.all[changeType(aType)]
		entityMsgs := []string{}

		// Sort by change scopes
		scopes := []string{}
		for one := range typeGroup {
			scopes = append(scopes, string(one))
		}
		sort.Strings(scopes)

		for _, aScope := range scopes {
			scopeGroup := typeGroup[scope(aScope)]
			sort.Strings(scopeGroup)
			entityMsgs = append(entityMsgs, adjustPlurality(aScope, len(scopeGroup)))
			details += fmt.Sprintf("\n%v%v %v:%v%v",
				tabSpace,
				aType,
				aScope,
				detailsSeparator,
				strings.Join(scopeGroup, detailsSeparator))
		}
		typeMsg := formatSlice(entityMsgs, ", ", " and ")
		msgs = append(msgs, fmt.Sprintf("%v %v", typeMsg, aType))
	}

	result := formatSlice(msgs, "; ", " as well as ")
	return fmt.Sprintf("%v:\n%v", result, details)
}

// TODO(anastasiamac 2014-04-13) Move this to
// juju/utils (eg. Pluralize). Added tech debt card.
func adjustPlurality(entity string, count int) string {
	switch count {
	case 0:
		return ""
	case 1:
		return fmt.Sprintf("%d %v", count, entity)
	default:
		return fmt.Sprintf("%d %vs", count, entity)
	}
}

func formatSlice(slice []string, itemSeparator, lastSeparator string) string {
	switch len(slice) {
	case 0:
		return ""
	case 1:
		return slice[0]
	default:
		return fmt.Sprintf("%v%v%v",
			strings.Join(slice[:len(slice)-1], itemSeparator),
			lastSeparator,
			slice[len(slice)-1])
	}
}
