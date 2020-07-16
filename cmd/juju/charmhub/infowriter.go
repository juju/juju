// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/juju/errors"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/cmd/output"
)

// Note:
// Using yaml formatting for most of the juju info output,
// to keep it similar to snap info, is easiest done in yaml.
// There are exceptions, slices of strings and tables.  These
// are transformed into strings.

func makeInfoWriter(w io.Writer, warningLog Log, in *InfoResponse) Printer {
	iw := infoWriter{
		w:        w,
		warningf: warningLog,
		in:       in,
	}
	if iw.in.Type == "charm" {
		return charmInfoWriter{infoWriter: iw}
	}
	return bundleInfoWriter{infoWriter: iw}
}

type infoWriter struct {
	warningf Log
	w        io.Writer
	in       *InfoResponse
}

func (iw infoWriter) print(info interface{}) error {
	encoder := yaml.NewEncoder(iw.w)
	defer func() { _ = encoder.Close() }()
	return encoder.Encode(info)
}

var channelRisks = []string{"stable", "candidate", "beta", "edge"}

func (iw infoWriter) channels() string {
	if len(iw.in.Channels) == 0 {
		return ""
	}
	buffer := bytes.NewBufferString("")

	tw := output.TabWriter(buffer)
	w := output.Wrapper{TabWriter: tw}
	for _, track := range iw.in.Tracks {
		trackHasOpenChannel := false
		for _, risk := range channelRisks {
			chName := fmt.Sprintf("%s/%s", track, risk)
			ch, ok := iw.in.Channels[chName]
			if ok {
				iw.writeOpenChanneltoBuffer(w, ch)
				trackHasOpenChannel = true
			} else {
				iw.writeClosedChannelToBuffer(w, chName, trackHasOpenChannel)
			}
		}
	}
	if err := w.Flush(); err != nil {
		iw.warningf("%v", errors.Annotate(err, "could not flush channel data to buffer"))
	}
	return buffer.String()
}

func (iw infoWriter) writeOpenChanneltoBuffer(w output.Wrapper, channel Channel) {
	w.Printf("%s/%s:", channel.Track, channel.Risk)
	w.Print(channel.Version)
	releasedAt, err := time.Parse(time.RFC3339, channel.ReleasedAt)
	if err != nil {
		// This should not fail, if it does, warn on the error
		// rather than ignoring.
		iw.warningf("%s", errors.Annotate(err, "could not parse released at time").Error())
		w.Print(" ")
	} else {
		w.Print(releasedAt.Format("2006-01-02"))
	}
	w.Printf("(%s)", strconv.Itoa(channel.Revision))
	w.Println(sizeToStr(channel.Size))
}

func (iw infoWriter) writeClosedChannelToBuffer(w output.Wrapper, name string, hasOpenChannel bool) {
	// TODO (hml) 2020-07-07
	// Add the ability to print unicode or ascii characters here
	// for the closed channel symbols.
	w.Printf("%s:", name)
	if hasOpenChannel {
		w.Println("^")
		return
	}
	w.Println("--")
}

type bundleInfoOutput struct {
	Name        string `yaml:"name,omitempty"`
	ID          string `yaml:"bundle-id,omitempty"`
	Summary     string `yaml:"summary,omitempty"`
	Publisher   string `yaml:"publisher,omitempty"`
	Supports    string `yaml:"supports,omitempty"`
	Tags        string `yaml:"tags,omitempty"`
	StoreURL    string `yaml:"store-url,omitempty"`
	Description string `yaml:"description,omitempty"`
	Charms      string `yaml:"charms,omitempty"`
	Channels    string `yaml:"channels,omitempty"`
	Installed   string `yaml:"installed,omitempty"`
}

type bundleInfoWriter struct {
	infoWriter
}

func (b bundleInfoWriter) Print() error {
	out := &bundleInfoOutput{
		Name:        b.in.Name,
		ID:          b.in.ID,
		Summary:     b.in.Summary,
		Publisher:   b.in.Publisher,
		Description: b.in.Description,
		Tags:        strings.Join(b.in.Tags, ", "),
		Channels:    b.channels(),
	}
	return b.print(out)
}

type charmInfoOutput struct {
	Name        string         `yaml:"name,omitempty"`
	ID          string         `yaml:"charm-id,omitempty"`
	Summary     string         `yaml:"summary,omitempty"`
	Publisher   string         `yaml:"publisher,omitempty"`
	Supports    string         `yaml:"supports,omitempty"`
	Tags        string         `yaml:"tags,omitempty"`
	Subordinate bool           `yaml:"subordinate"`
	StoreURL    string         `yaml:"store-url,omitempty"`
	Description string         `yaml:"description,omitempty"`
	Relations   relationOutput `yaml:"relations,omitempty"`
	Channels    string         `yaml:"channels,omitempty"`
	Installed   string         `yaml:"installed,omitempty"`
}

type relationOutput struct {
	Provides map[string]string `json:"provides,omitempty"`
	Requires map[string]string `json:"requires,omitempty"`
}

type charmInfoWriter struct {
	infoWriter
}

func (c charmInfoWriter) Print() error {
	out := &charmInfoOutput{
		Name:        c.in.Name,
		ID:          c.in.ID,
		Summary:     c.in.Summary,
		Publisher:   c.in.Publisher,
		Supports:    strings.Join(c.in.Series, ", "),
		Description: c.in.Description,
		Channels:    c.channels(),
		Tags:        strings.Join(c.in.Tags, ", "),
	}
	if c.in.Charm != nil {
		out.Subordinate = c.in.Charm.Subordinate
	}
	if rels, err := c.relations(); err == nil {
		out.Relations = rels
	}
	return c.print(out)
}

func (c charmInfoWriter) relations() (relationOutput, error) {
	if c.in.Charm == nil {
		return relationOutput{}, errors.NotFoundf("charm")
	}
	requires, foundRequires := c.in.Charm.Relations["requires"]
	provides, foundProvides := c.in.Charm.Relations["provides"]
	if !foundProvides && !foundRequires {
		return relationOutput{}, errors.NotFoundf("charm relations")
	}
	var relations relationOutput
	if foundProvides {
		relations.Provides = provides
	}
	if foundRequires {
		relations.Requires = requires
	}
	return relations, nil
}

func sizeToStr(size int) string {
	suffixes := []string{"B", "kB", "MB", "GB", "TB", "PB", "EB"}
	for _, suf := range suffixes {
		if size < 1000 {
			return fmt.Sprintf("%d%s", size, suf)
		}
		size /= 1000
	}
	return ""
}
