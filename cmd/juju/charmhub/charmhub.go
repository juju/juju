// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"

	"github.com/juju/juju/v2/charmhub"
	"github.com/juju/juju/v2/cmd/modelcmd"
	"github.com/juju/juju/v2/core/arch"
)

var logger = loggo.GetLogger("juju.cmd.juju.charmhub")

func newCharmHubCommand() *charmHubCommand {
	return &charmHubCommand{
		arches: arch.AllArches(),
		CharmHubClientFunc: func(config charmhub.Config, fs charmhub.FileSystem) (CharmHubClient, error) {
			return charmhub.NewClientWithFileSystem(config, fs)
		},
	}
}

type charmHubCommand struct {
	cmd.CommandBase
	modelcmd.FilesystemCommand

	arch        string
	arches      arch.Arches
	series      string
	charmHubURL string

	CharmHubClientFunc func(charmhub.Config, charmhub.FileSystem) (CharmHubClient, error)
}

func (c *charmHubCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.charmHubURL, "charmhub-url", charmhub.CharmHubServerURL, "specify the Charmhub URL for querying the store")
}

// Init initializes the info command, including validating the provided
// flags. It implements part of the cmd.Command interface.
func (c *charmHubCommand) Init(args []string) error {
	// If the architecture is empty, ensure we normalize it to all to prevent
	// complicated comparison checking.
	if c.arch == "" {
		c.arch = ArchAll
	}

	if c.arch != ArchAll && !c.arches.Contains(c.arch) {
		return errors.Errorf("unexpected architecture flag value %q, expected <%s>", c.arch, c.archArgumentList())
	}

	if urlFromEnv := os.Getenv("CHARMHUB_URL"); urlFromEnv != "" {
		c.charmHubURL = urlFromEnv
	}
	if c.charmHubURL == "" {
		c.charmHubURL = charmhub.CharmHubServerURL
	}
	_, err := url.ParseRequestURI(c.charmHubURL)
	if err != nil {
		return errors.Annotatef(err, "invalid charmhub-url")
	}

	return nil
}

func (c *charmHubCommand) archArgumentList() string {
	archList := strings.Join(c.arches.StringList(), "|")
	return fmt.Sprintf("%s|%s", ArchAll, archList)
}
