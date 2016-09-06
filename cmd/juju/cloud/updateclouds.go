// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"sort"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/utils"
	"github.com/juju/utils/set"
	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/clearsign"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/juju/keys"
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

See also: clouds
`

// NewUpdateCloudsCommand returns a command to update cloud information.
var NewUpdateCloudsCommand = func() cmd.Command {
	return newUpdateCloudsCommand()
}

func newUpdateCloudsCommand() cmd.Command {
	return &updateCloudsCommand{
		publicSigningKey: keys.JujuPublicKey,
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
	fmt.Fprint(ctxt.Stderr, "Fetching latest public cloud list...\n")
	client := utils.GetHTTPClient(utils.VerifySSLHostnames)
	resp, err := client.Get(c.publicCloudURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusNotFound:
			fmt.Fprintln(ctxt.Stderr, "Public cloud list is unavailable right now.")
			return nil
		case http.StatusUnauthorized:
			return errors.Unauthorizedf("unauthorised access to URL %q", c.publicCloudURL)
		}
		return errors.Errorf("cannot read public cloud information at URL %q, %q", c.publicCloudURL, resp.Status)
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
		fmt.Fprintln(ctxt.Stderr, "Your list of public clouds is up to date, see `juju clouds`.")
		return nil
	}
	if err := jujucloud.WritePublicCloudMetadata(newPublicClouds); err != nil {
		return errors.Annotate(err, "error writing new local public cloud data")
	}
	updateDetails := diffClouds(newPublicClouds, currentPublicClouds)
	fmt.Fprintln(ctxt.Stderr, fmt.Sprintf("Updated your list of public clouds with %s", updateDetails))
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
	for cloudName, _ := range oldClouds {
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
	for oldName, _ := range oldRegions {
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
	for one, _ := range c.all {
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
		for one, _ := range typeGroup {
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
