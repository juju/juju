// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/jujuclient"
)

type cloudList struct {
	public   []string
	builtin  []string
	personal []string
}

func formatCloudDetailsTabular(ctx *cmd.Context, clouds cloudList, credStore jujuclient.CredentialStore) ([]byte, error) {
	var out bytes.Buffer
	const (
		// To format things into columns.
		minwidth = 0
		tabwidth = 1
		padding  = 2
		padchar  = ' '
		flags    = 0
	)
	tw := tabwriter.NewWriter(&out, minwidth, tabwidth, padding, padchar, flags)
	p := func(values ...string) {
		text := strings.Join(values, "\t")
		fmt.Fprintln(tw, text)
	}
	p("Cloud\tCredentials\tDefault Region")
	printClouds := func(cloudNames []string) error {
		sort.Strings(cloudNames)
		for _, name := range cloudNames {
			cred, err := credStore.CredentialForCloud(name)
			if err != nil && !errors.IsNotFound(err) {
				ctx.Warningf("error loading credential for cloud %v: %v", name, err)
				continue
			}
			if err != nil || len(cred.AuthCredentials) == 0 {
				p(name, "", "")
				continue
			}
			var sortedCredNames []string
			for credName := range cred.AuthCredentials {
				sortedCredNames = append(sortedCredNames, credName)
			}
			sort.Strings(sortedCredNames)
			for i, credName := range sortedCredNames {
				if i == 0 {
					p(name, credName, cred.DefaultRegion)
				} else {
					p("", credName, "")
				}
			}
		}
		return nil
	}
	if err := printClouds(clouds.public); err != nil {
		return nil, err
	}
	if err := printClouds(clouds.builtin); err != nil {
		return nil, err
	}
	if err := printClouds(clouds.personal); err != nil {
		return nil, err
	}

	tw.Flush()
	return out.Bytes(), nil
}

func printClouds(ctx *cmd.Context, credStore jujuclient.CredentialStore) error {
	publicClouds, _, err := jujucloud.PublicCloudMetadata(jujucloud.JujuPublicCloudsPath())
	if err != nil {
		return err
	}

	personalClouds, err := jujucloud.PersonalCloudMetadata()
	if err != nil {
		return err
	}

	fmt.Fprintln(ctx.Stdout, "You can bootstrap on these clouds. See ‘--regions <cloud>’ for all regions.")
	clouds := cloudList{}
	for name := range publicClouds {
		clouds.public = append(clouds.public, name)
	}
	// Add in built in clouds like localhost (lxd).
	builtin, err := common.BuiltInClouds()
	if err != nil {
		return errors.Trace(err)
	}
	for name := range builtin {
		clouds.builtin = append(clouds.builtin, name)
	}
	for name := range personalClouds {
		clouds.personal = append(clouds.personal, name)
	}
	out, err := formatCloudDetailsTabular(ctx, clouds, credStore)
	if err != nil {
		return err
	}
	fmt.Fprintln(ctx.Stdout, string(out))
	credHelpText := `
You will need to have a credential if you want to bootstrap on a cloud, see
‘juju autoload-credentials’ and ‘juju add-credential’. The first credential
listed is the default. Add more clouds with ‘juju add-cloud’.
`
	fmt.Fprint(ctx.Stdout, credHelpText[1:])
	return nil
}

func printCloudRegions(ctx *cmd.Context, cloudName string) error {
	cloud, err := common.CloudByName(cloudName)
	if err != nil {
		return errors.Trace(err)
	}
	fmt.Fprintf(ctx.Stdout, "Showing regions for %s:\n", cloudName)
	for _, region := range cloud.Regions {
		fmt.Fprintln(ctx.Stdout, region.Name)
	}
	return nil
}
