// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/arch"
)

type charmhubCommand struct {
	modelcmd.ModelCommandBase

	arch   string
	arches arch.Arches
	series string
}

func (c *charmhubCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)

	f.StringVar(&c.arch, "arch", ArchAll, fmt.Sprintf("specify an arch <%s>", c.archArgumentList()))
	f.StringVar(&c.series, "series", SeriesAll, "specify a series")
}

// Init initializes the info command, including validating the provided
// flags. It implements part of the cmd.Command interface.
func (c *charmhubCommand) Init(args []string) error {
	// If the architecture is empty, ensure we normalize it to all to prevent
	// complicated comparison checking.
	if c.arch == "" {
		c.arch = ArchAll
	}

	if c.arch != ArchAll && !c.arches.Contains(c.arch) {
		return errors.Errorf("unexpected architecture flag value %q, expected <%s>", c.arch, c.archArgumentList())
	}

	// It's much harder to specify the series we support in a list fashion.
	if c.series == "" {
		c.series = SeriesAll
	}

	return nil
}

func (c *charmhubCommand) archArgumentList() string {
	archList := strings.Join(c.arches.StringList(), "|")
	return fmt.Sprintf("%s|%s", ArchAll, archList)
}
