// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package transport

// The following contains all the common DTOs for a gathering information from
// a given store.

// Type represents the type of payload is expected from the API
type Type string

// Matches attempts to match a string to a given source.
func (t Type) Matches(o string) bool {
	return string(t) == o
}

func (t Type) String() string {
	return string(t)
}

const (
	// CharmType represents the charm payload.
	CharmType Type = "charm"
	// BundleType represents the bundle payload.
	BundleType Type = "bundle"
)

// Channel defines a unique permutation that corresponds to the track, risk
// and base. There can be multiple channels of the same track and risk, but
// with different bases.
type Channel struct {
	Name       string `json:"name"`
	Base       Base   `json:"base"`
	ReleasedAt string `json:"released-at"`
	Risk       string `json:"risk"`
	Track      string `json:"track"`
}

// Base is a typed tuple for identifying charms or bundles with a matching
// architecture, os and channel.
type Base struct {
	Architecture string `json:"architecture"`
	Name         string `json:"name"`
	Channel      string `json:"channel"`
}

// Download represents the download structure from CharmHub.
// Elements not used by juju but not used are: "hash-sha3-384"
// and "hash-sha-512"
type Download struct {
	HashSHA256 string `json:"hash-sha-256"`
	HashSHA384 string `json:"hash-sha-384"`
	Size       int    `json:"size"`
	URL        string `json:"url"`
}

// Entity holds the information about the charm or bundle, either contains the
// information about the charm or bundle or whom owns it.
type Entity struct {
	Categories  []Category        `json:"categories"`
	Charms      []Charm           `json:"contains-charms"`
	Description string            `json:"description"`
	License     string            `json:"license"`
	Publisher   map[string]string `json:"publisher"`
	Summary     string            `json:"summary"`
	UsedBy      []string          `json:"used-by"`
	StoreURL    string            `json:"store-url"`
}

// Category defines the category of a given charm or bundle. Akin to a tag.
type Category struct {
	Featured bool   `json:"featured"`
	Name     string `json:"name"`
}

// Charm is used to identify charms within a bundle.
type Charm struct {
	Name      string `json:"name"`
	PackageID string `json:"package-id"`
	StoreURL  string `json:"store-url"`
}
