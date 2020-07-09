package charm

// Source represents the source of the charm.
type Source string

func (c Source) String() string {
	return string(c)
}

const (
	// Local represents a local charm.
	Local Source = "local"
	// CharmStore represents a charm from the now old charmstore.
	CharmStore Source = "charm-store"
	// Charmhub represents a charm from the new charmhub.
	Charmhub Source = "charmhub"
	// Unknown represents that we don't know where this charm came from. Either
	// the charm was migrated up from an older version of Juju or we didn't
	// have enough information when we set the charm.
	Unknown Source = "unknown"
)

// Origin holds the original source of a charm. Information about where the
// charm was installed from (charmhub, charmstore, local) and any additional
// information we can utilise when making modelling decisions for upgrading or
// changing.
type Origin struct {
	Source Source
}
