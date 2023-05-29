// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"github.com/juju/charm/v11"
)

const (
	// SeriesAll defines a platform that targets all series.
	SeriesAll = "all"
	// ArchAll defines a platform that targets all architectures.
	ArchAll = "all"
)

type InfoResponse struct {
	Type        string       `json:"type" yaml:"type"`
	ID          string       `json:"id" yaml:"id"`
	Name        string       `json:"name" yaml:"name"`
	Description string       `json:"description" yaml:"description"`
	Publisher   string       `json:"publisher" yaml:"publisher"`
	Summary     string       `json:"summary" yaml:"summary"`
	Supports    []Base       `json:"supports,omitempty" yaml:"supports,omitempty"`
	StoreURL    string       `json:"store-url" yaml:"store-url"`
	Tags        []string     `json:"tags,omitempty" yaml:"tags,omitempty"`
	Charm       *Charm       `json:"charm,omitempty" yaml:"charm,omitempty"`
	Bundle      *Bundle      `json:"bundle,omitempty" yaml:"bundle,omitempty"`
	Channels    RevisionsMap `json:"channels" yaml:"channels"`
	Tracks      []string     `json:"tracks,omitempty" yaml:"tracks,omitempty"`
}

// RevisionsMap is a map of tracks to risks to list of revisions, for example
// {"latest": {"stable": [r3, r2, r1]}}.
type RevisionsMap map[string]map[string][]Revision

type FindResponse struct {
	Type      string   `json:"type" yaml:"type"`
	ID        string   `json:"id" yaml:"id"`
	Name      string   `json:"name" yaml:"name"`
	Publisher string   `json:"publisher" yaml:"publisher"`
	Summary   string   `json:"summary" yaml:"summary"`
	Version   string   `json:"version" yaml:"version"`
	Arches    []string `json:"architectures,omitempty" yaml:"architectures,omitempty"`
	OS        []string `json:"os,omitempty" yaml:"os,omitempty"`
	Supports  []Base   `json:"supports,omitempty" yaml:"supports,omitempty"`
	StoreURL  string   `json:"store-url" yaml:"store-url"`
}

type Revision struct {
	Track      string   `json:"track" yaml:"track"`
	Risk       string   `json:"risk" yaml:"risk"`
	Version    string   `json:"version" yaml:"version"`
	Revision   int      `json:"revision" yaml:"revision"`
	ReleasedAt string   `json:"released-at" yaml:"released-at"`
	Size       int      `json:"size" yaml:"size"`
	Arches     []string `json:"architectures" yaml:"architectures"`
	Bases      []Base   `json:"bases" yaml:"bases"`
}

type Base struct {
	Name    string `json:"name" yaml:"name"`
	Channel string `json:"channel" yaml:"channel"`
}

// Charm matches a params.CharmHubCharm
type Charm struct {
	Config      *charm.Config                `json:"config,omitempty" yaml:"config,omitempty"`
	Relations   map[string]map[string]string `json:"relations,omitempty" yaml:"relations,omitempty"`
	Subordinate bool                         `json:"subordinate,omitempty" yaml:"subordinate,omitempty"`
	UsedBy      []string                     `json:"used-by,omitempty" yaml:"used-by,omitempty"` // bundles which use the charm
}

type Bundle struct {
	Charms []BundleCharm `json:"charms,omitempty" yaml:"charms,omitempty"`
}

type BundleCharm struct {
	Name string `json:"name" yaml:"name"`
}
