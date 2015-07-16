// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"text/template"

	"github.com/juju/juju/environs/config"
)

var configHeader = `
# This is the Juju config file, which you can use to specify multiple
# environments in which to deploy. By default Juju ships with AWS
# (default), HP Cloud, OpenStack, Azure, MaaS, Local and Manual
# providers. See https://juju.ubuntu.com/docs for more information

# An environment configuration must always specify at least the
# following information:
# - name (to identify the environment)
# - type (to specify the provider)
# In the following example the name is "myenv" and type is "ec2".
# myenv:
#    type: ec2

# Values in <brackets> below need to be filled in by the user.
# Optional attributes are shown commented out, with
# a sample value or a value in <brackets>.

# There are several settings supported by all environments, all of which
# are optional and have specified default values. For more info, see the
# Juju documentation.

# The default environment is chosen when an environment is not
# specified using any of the following, in descending order of precedence:
#  1. -e or --environment command line parameter, passed after the command, e.g.
#     $ juju add-unit -e myenv myservice
#  2. By setting JUJU_ENV environment variable.
#  3. Using the juju switch command like this:
#     $ juju switch myenv
#

# You can control how Juju harvests machines by using the
# {{.ProvisionerHarvestKey}} setting. Harvesting is a process wherein
# Juju attempts to reclaim unused machines.
#
# Options are:
#
# Don't harvest any machines.
# {{.ProvisionerHarvestKey}}: none
#
# Only harvest machines that Juju knows about and are dead.
# {{.ProvisionerHarvestKey}}: destroyed
#
# Only harvest machines that Juju doesn't know about.
# {{.ProvisionerHarvestKey}}: unknown
#
# Harvest both dead and unknown machines.
# {{.ProvisionerHarvestKey}}: all

default: amazon

environments:
`[1:]

func init() {

	type headerVars struct {
		ProvisionerHarvestKey string
	}

	configBuff := new(bytes.Buffer)
	configHeaderTmpl := template.Must(template.New("config header").Parse(configHeader))
	if err := configHeaderTmpl.Execute(configBuff, headerVars{
		ProvisionerHarvestKey: config.ProvisionerHarvestModeKey,
	}); err != nil {
		panic(fmt.Sprintf("error building config header: %v", err))
	}
	configHeader = configBuff.String()
}

func randomKey() string {
	buf := make([]byte, 16)
	_, err := io.ReadFull(rand.Reader, buf)
	if err != nil {
		panic(fmt.Errorf("error from crypto rand: %v", err))
	}
	return fmt.Sprintf("%x", buf)
}

// BoilerplateConfig returns a sample juju configuration.
func BoilerplateConfig() string {
	configBuff := new(bytes.Buffer)
	configBuff.WriteString(configHeader)
	for name, p := range providers {
		t, err := parseTemplate(p.BoilerplateConfig())
		if err != nil {
			panic(fmt.Errorf("cannot parse boilerplate from %s: %v", name, err))
		}
		var ecfg bytes.Buffer
		if err := t.Execute(&ecfg, nil); err != nil {
			panic(fmt.Errorf("cannot generate boilerplate from %s: %v", name, err))
		}
		indent(configBuff, ecfg.Bytes(), "    ")
	}

	// Sanity check to ensure the boilerplate parses.
	_, err := ReadEnvironsBytes(configBuff.Bytes())
	if err != nil {
		panic(fmt.Errorf("cannot parse %s:\n%v", configBuff.String(), err))
	}
	return configBuff.String()
}

func parseTemplate(s string) (*template.Template, error) {
	t := template.New("")
	t.Funcs(template.FuncMap{"rand": randomKey})
	return t.Parse(s)
}

// indent appends the given text to the given buffer indented by the given indent string.
func indent(b *bytes.Buffer, text []byte, indentStr string) {
	for {
		if len(text) == 0 {
			return
		}
		b.WriteString(indentStr)
		i := bytes.IndexByte(text, '\n')
		if i == -1 {
			b.Write(text)
			b.WriteRune('\n')
			return
		}
		i++
		b.Write(text[0:i])
		text = text[i:]
	}
}
