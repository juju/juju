// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrations

import (
	"github.com/juju/charm/v12"
	"github.com/juju/description/v9"
	"github.com/juju/errors"
	"github.com/juju/names/v5"
)

// MigrationRemoteApplication is an in-place representation of the
// state.RemoteApplication
type MigrationRemoteApplication interface {
	Tag() names.Tag
	OfferUUID() string
	URL() (string, bool)
	SourceModel() names.ModelTag
	IsConsumerProxy() bool
	Endpoints() ([]MigrationRemoteEndpoint, error)
	Bindings() map[string]string
	Spaces() []MigrationRemoteSpace
	GlobalKey() string
	Macaroon() string
	ConsumeVersion() int
}

// MigrationRemoteEndpoint is an in-place representation of the state.Endpoint
type MigrationRemoteEndpoint struct {
	Name      string
	Role      charm.RelationRole
	Interface string
}

// MigrationRemoteSpace is an in-place representation of the state.RemoteSpace
type MigrationRemoteSpace struct {
	CloudType          string
	Name               string
	ProviderId         string
	ProviderAttributes map[string]interface{}
	Subnets            []MigrationRemoteSubnet
}

// MigrationRemoteSubnet is an in-place representation of the state.RemoteSubnet
type MigrationRemoteSubnet struct {
	CIDR              string
	ProviderId        string
	VLANTag           int
	AvailabilityZones []string
	ProviderSpaceId   string
	ProviderNetworkId string
}

// AllRemoteApplicationSource defines an in-place usage for reading all the
// remote application.
type AllRemoteApplicationSource interface {
	AllRemoteApplications() ([]MigrationRemoteApplication, error)
}

// StatusSource defines an in-place usage for reading in the status for a given
// entity.
type StatusSource interface {
	StatusArgs(string) (description.StatusArgs, error)
}

// RemoteApplicationSource composes all the interfaces to create a remote
// application.
type RemoteApplicationSource interface {
	AllRemoteApplicationSource
	StatusSource
}

// RemoteApplicationModel defines an in-place usage for adding a remote entity
// to a model.
type RemoteApplicationModel interface {
	AddRemoteApplication(description.RemoteApplicationArgs) description.RemoteApplication
}

// ExportRemoteApplications describes a way to execute a migration for exporting
// remote entities.
type ExportRemoteApplications struct{}

// Execute the migration of the remote entities using typed interfaces, to
// ensure we don't loose any type safety.
// This doesn't conform to an interface because go doesn't have generics, but
// when this does arrive this would be an excellent place to use them.
func (m ExportRemoteApplications) Execute(src RemoteApplicationSource, dst RemoteApplicationModel) error {
	remoteApps, err := src.AllRemoteApplications()
	if err != nil {
		return errors.Trace(err)
	}

	for _, remoteApp := range remoteApps {
		if err := m.addRemoteApplication(src, dst, remoteApp); err != nil {
			return errors.Trace(err)
		}
	}
	return nil
}

func (m ExportRemoteApplications) addRemoteApplication(src RemoteApplicationSource, dst RemoteApplicationModel, app MigrationRemoteApplication) error {
	// Note the ignore case is not an error, but a bool indicating if it's valid
	// or not. For this scenario, we're happy to ignore that situation.
	url, _ := app.URL()

	args := description.RemoteApplicationArgs{
		Tag:             app.Tag().(names.ApplicationTag),
		OfferUUID:       app.OfferUUID(),
		URL:             url,
		SourceModel:     app.SourceModel(),
		IsConsumerProxy: app.IsConsumerProxy(),
		Bindings:        app.Bindings(),
		Macaroon:        app.Macaroon(),
		ConsumeVersion:  app.ConsumeVersion(),
	}
	descApp := dst.AddRemoteApplication(args)

	status, err := src.StatusArgs(app.GlobalKey())
	if err != nil && !errors.IsNotFound(err) {
		return errors.Trace(err)
	}
	// Not all remote applications have status.
	if err == nil {
		descApp.SetStatus(status)
	}

	endpoints, err := app.Endpoints()
	if err != nil {
		return errors.Trace(err)
	}
	for _, ep := range endpoints {
		descApp.AddEndpoint(description.RemoteEndpointArgs{
			Name:      ep.Name,
			Role:      string(ep.Role),
			Interface: ep.Interface,
		})
	}
	for _, space := range app.Spaces() {
		m.addRemoteSpace(descApp, space)
	}
	return nil
}

func (m ExportRemoteApplications) addRemoteSpace(descApp description.RemoteApplication, space MigrationRemoteSpace) {
	descSpace := descApp.AddSpace(description.RemoteSpaceArgs{
		CloudType:          space.CloudType,
		Name:               space.Name,
		ProviderId:         space.ProviderId,
		ProviderAttributes: space.ProviderAttributes,
	})
	for _, subnet := range space.Subnets {
		descSpace.AddSubnet(description.SubnetArgs{
			CIDR:              subnet.CIDR,
			ProviderId:        subnet.ProviderId,
			VLANTag:           subnet.VLANTag,
			AvailabilityZones: subnet.AvailabilityZones,
			ProviderSpaceId:   subnet.ProviderSpaceId,
			ProviderNetworkId: subnet.ProviderNetworkId,
		})
	}
}
