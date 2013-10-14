// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"text/template"
)

var configHeader = `
# This is the Juju config file, which you can use to specify multiple environments in which to deploy.
# By default Juju ships AWS (default), HP Cloud, OpenStack.
# See https://juju.ubuntu.com/docs for more information

# An environment configuration must always specify at least the following information:
#
# - name (to identify the environment)
# - type (to specify the provider)

# Values in <brackets> below need to be filled in by the user.
# Optional attributes are shown commented out, with
# a sample value or a value in <brackets>.

# The default environment is chosen when an environment is not
# specified using any of the following, in descending order of precedence:
#   -e or --environment command line parameter.
#   JUJU_ENV environment variable.
#   the juju switch command.
default: amazon

environments:
`[1:]

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
	var config bytes.Buffer

	config.WriteString(configHeader)
	for name, p := range providers {
		t, err := parseTemplate(p.BoilerplateConfig())
		if err != nil {
			panic(fmt.Errorf("cannot parse boilerplate from %s: %v", name, err))
		}
		var ecfg bytes.Buffer
		if err := t.Execute(&ecfg, nil); err != nil {
			panic(fmt.Errorf("cannot generate boilerplate from %s: %v", name, err))
		}
		indent(&config, ecfg.Bytes(), "    ")
	}

	// Sanity check to ensure the boilerplate parses.
	_, err := ReadEnvironsBytes(config.Bytes())
	if err != nil {
		panic(fmt.Errorf("cannot parse %s:\n%v", config.String(), err))
	}
	return config.String()
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
