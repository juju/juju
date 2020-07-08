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

	"github.com/juju/charm/v7"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/api/charmhub"
	"github.com/juju/juju/cmd/output"
)

// Note:
// Using yaml formatting for most of the juju info output,
// to keep it similar to snap info, is easiest done in yaml.
// There are exceptions, slices of strings and tables.  These
// are transformed into strings.

func makeInfoWriter(ctx *cmd.Context, in *charmhub.InfoResponse) Printer {
	iw := infoWriter{
		w:        ctx.Stdout,
		warningf: ctx.Warningf,
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
	in       *charmhub.InfoResponse
}

func (iw infoWriter) print(info interface{}) error {
	encoder := yaml.NewEncoder(iw.w)
	defer func() { _ = encoder.Close() }()
	return encoder.Encode(info)
}

func (iw infoWriter) publisher() string {
	publisher, _ := iw.in.Entity.Publisher["display-name"]
	return publisher
}

func (iw infoWriter) storeURL() string {
	// TODO (hml) 2020-06-24
	// Implement once the data is available, not clear
	// where it will be at this time.
	return ""
}

func (iw infoWriter) channels() string {
	// TODO (hml) 2020-06-24
	// Implement up arrow for closed channels
	if len(iw.in.ChannelMap) == 0 {
		return ""
	}
	buffer := bytes.NewBufferString("")

	tw := output.TabWriter(buffer)
	w := output.Wrapper{TabWriter: tw}

	for _, ch := range iw.in.ChannelMap {
		w.Printf("%s:", ch.Channel.Name)
		w.Print(ch.Revision.Version)
		releasedAt, err := time.Parse(time.RFC3339, ch.Channel.ReleasedAt)
		if err != nil {
			// This should not fail, if it does, warn on the error
			// rather than ignoring.
			iw.warningf("%v", errors.Annotate(err, "could not parse released at time"))
			w.Print(" ")
		} else {
			w.Print(releasedAt.Format("2006-01-02"))
		}
		w.Printf("(%s)", strconv.Itoa(ch.Revision.Revision))
		w.Println(sizeToStr(ch.Revision.Download.Size))
	}
	if err := w.Flush(); err != nil {
		iw.warningf("%v", errors.Annotate(err, "could not flush channel data to buffer"))
	}
	return buffer.String()
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
		Summary:     b.in.Entity.Summary,
		Publisher:   b.publisher(),
		Description: b.in.Entity.Description,
		Channels:    b.channels(),
	}
	return b.print(out)
}

type charmInfoOutput struct {
	Name        string                       `yaml:"name,omitempty"`
	ID          string                       `yaml:"charm-id,omitempty"`
	Summary     string                       `yaml:"summary,omitempty"`
	Publisher   string                       `yaml:"publisher,omitempty"`
	Supports    string                       `yaml:"supports,omitempty"`
	Tags        string                       `yaml:"tags,omitempty"`
	Subordinate bool                         `yaml:"subordinate"`
	StoreURL    string                       `yaml:"store-url,omitempty"`
	Description string                       `yaml:"description,omitempty"`
	Relations   map[string]map[string]string `yaml:"relations,omitempty"`
	Channels    string                       `yaml:"channels,omitempty"`
	Installed   string                       `yaml:"installed,omitempty"`
}

type charmInfoWriter struct {
	infoWriter
	charmMeta   *charm.Meta
	charmConfig *charm.Config
}

func (c charmInfoWriter) Print() error {
	c.unmarshalCharmConfig()
	c.unmarshalCharmMetadata()

	out := &charmInfoOutput{
		Name:        c.in.Name,
		ID:          c.in.ID,
		Summary:     c.in.Entity.Summary,
		Publisher:   c.publisher(),
		Supports:    c.supports(),
		Tags:        c.tags(),
		Subordinate: c.subordinate(),
		Description: c.in.Entity.Description,
		Channels:    c.channels(),
	}
	if rels, err := c.relations(); err == nil {
		out.Relations = rels
	}
	return c.print(out)
}

func (c *charmInfoWriter) unmarshalCharmMetadata() {
	if c.in.DefaultRelease.Revision.MetadataYAML == "" {
		return
	}
	m := c.in.DefaultRelease.Revision.MetadataYAML
	meta, err := charm.ReadMeta(bytes.NewBufferString(m))
	if err != nil {
		// Do not fail on unmarshalling metadata, log instead.
		// This should not happen, however at implementation
		// we were dealing with handwritten data for test, not
		// the real deal.  Usually charms are validated before
		// being uploaded to the store.
		c.warningf(errors.Annotate(err, "cannot unmarshal charm metadata").Error())
		return
	}
	c.charmMeta = meta
	return
}

func (c *charmInfoWriter) unmarshalCharmConfig() {
	if c.in.DefaultRelease.Revision.ConfigYAML == "" {
		return
	}
	cfgYaml := c.in.DefaultRelease.Revision.ConfigYAML
	cfg, err := charm.ReadConfig(bytes.NewBufferString(cfgYaml))
	if err != nil {
		// Do not fail on unmarshalling metadata, log instead.
		// This should not happen, however at implementation
		// we were dealing with handwritten data for test, not
		// the real deal.  Usually charms are validated before
		// being uploaded to the store.
		c.warningf(errors.Annotate(err, "cannot unmarshal charm config").Error())
		return
	}
	c.charmConfig = cfg
	return
}

func (c charmInfoWriter) relations() (map[string]map[string]string, error) {
	if c.charmMeta == nil {
		return nil, errors.NotFoundf("charm meta data")
	}
	if len(c.charmMeta.Requires) == 0 && len(c.charmMeta.Provides) == 0 {
		return nil, errors.NotFoundf("charm meta data")
	}
	relations := make(map[string]map[string]string)
	if provides, ok := formatRelationPart(c.charmMeta.Provides); ok {
		relations["provides"] = provides
	}
	if requires, ok := formatRelationPart(c.charmMeta.Requires); ok {
		relations["requires"] = requires
	}
	return relations, nil
}

func formatRelationPart(rels map[string]charm.Relation) (map[string]string, bool) {
	if len(rels) <= 0 {
		return nil, false
	}
	relations := make(map[string]string, len(rels))
	for k, v := range rels {
		relations[k] = v.Name
	}
	return relations, true
}

func (c charmInfoWriter) subordinate() bool {
	if c.charmMeta == nil {
		return false
	}
	return c.charmMeta.Subordinate
}

func (c charmInfoWriter) supports() string {
	if c.charmMeta == nil || len(c.charmMeta.Series) == 0 {
		return ""
	}
	return strings.Join(c.charmMeta.Series, ", ")
}

func (c charmInfoWriter) tags() string {
	if c.charmMeta == nil || len(c.charmMeta.Tags) == 0 {
		return ""
	}
	return strings.Join(c.charmMeta.Tags, ", ")
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
