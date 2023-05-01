package state

type ExternalController struct {
	// Id holds external controller document key.
	// ID is the controller UUID.
	ID string `db:"uuid"`

	// Alias holds an alias (human friendly) name for the controller.
	Alias string `db:"alias"`

	// Addrs holds the host:port values for the external
	// controller's API server.
	Addrs []string `db:"addresses"`

	// CACert holds the certificate to validate the external
	// controller's target API server's TLS certificate.
	CACert string `db:"cacert"`

	// Models holds model UUIDs hosted on this controller.
	Models []string `db:"models"`
}
