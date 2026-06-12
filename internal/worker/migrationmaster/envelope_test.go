// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"strings"
	"testing"
	"time"

	"github.com/juju/tc"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/core/machine"
	coreresource "github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application/architecture"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	charmresource "github.com/juju/juju/domain/deployment/charm/resource"
	"github.com/juju/juju/domain/modelmigration"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/rpc/params"
)

type EnvelopeSuite struct {
	testhelpers.IsolationSuite
}

func TestEnvelopeSuite(t *testing.T) {
	tc.Run(t, &EnvelopeSuite{})
}

func (*EnvelopeSuite) TestEnvelopeFromControllerModelInfo(c *tc.C) {
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	size := uint64(8)
	info := modelmigration.ControllerModelInfo{
		ModelInfo: modelmigration.ModelIdentityInfo{
			UUID:            "model-uuid",
			Name:            "model-name",
			Qualifier:       "prod",
			Type:            "iaas",
			Cloud:           "aws",
			CloudRegion:     "eu-west-1",
			CredentialName:  "cred",
			CredentialOwner: "owner",
			Life:            "alive",
		},
		Users: []modelmigration.ModelUser{{
			Name:        "fred",
			DisplayName: "Fred",
			CreatedBy:   "admin",
			CreatedAt:   created,
			LastLogin:   &created,
		}},
		ModelCredential: &modelmigration.ModelCloudCredential{
			Cloud:         "aws",
			Owner:         "owner",
			Name:          "cred",
			AuthType:      "access-key",
			Attributes:    map[string]string{"key": "value"},
			Revoked:       true,
			Invalid:       true,
			InvalidReason: "because",
		},
		Permissions: []modelmigration.ModelPermission{{
			ObjectType:  "model",
			GrantOn:     "model-uuid",
			SubjectName: "fred",
			Access:      "read",
		}, {
			ObjectType:  "offer",
			GrantOn:     "offer-uuid",
			SubjectName: "fred",
			Access:      "consume",
		}},
		AuthorizedKeys: []modelmigration.ModelAuthorizedKey{{
			Username:  "fred",
			PublicKey: "ssh-rsa AAAA",
		}},
		SecretBackend: &modelmigration.ModelSecretBackend{
			Name:        "vault",
			BackendType: "vault",
		},
		SecretBackendRefs: []modelmigration.SecretBackendReference{{
			BackendName:        "vault",
			SecretRevisionUUID: "rev-uuid",
			SecretID:           "secret-id",
		}},
		Leaders: []modelmigration.ApplicationLeadership{{
			Application: "app",
			Leader:      "app/0",
		}},
		CloudImageMetadata: []modelmigration.CloudImageMetadata{{
			Stream:          "released",
			Region:          "eu-west-1",
			Version:         "22.04",
			Arch:            "amd64",
			VirtType:        "kvm",
			RootStorageType: "ssd",
			RootStorageSize: &size,
			Source:          "custom",
			Priority:        7,
			ImageID:         "ami-1",
			CreatedAt:       created,
		}},
		ExternalControllers: []modelmigration.ExternalController{{
			UUID:           "ext-uuid",
			Alias:          "other",
			CACert:         "ca-cert",
			Addresses:      []string{"10.0.0.1:17070"},
			ConsumedModels: []string{"offerer-model-uuid"},
		}},
	}

	envelope := envelopeFromControllerModelInfo(info, "migration-uuid")
	c.Check(envelope, tc.DeepEquals, params.SerializedModelV2{
		ModelInfo: params.SerializedModelInfo{
			UUID:                "model-uuid",
			Name:                "model-name",
			Qualifier:           "prod",
			Type:                "iaas",
			Cloud:               "aws",
			CloudRegion:         "eu-west-1",
			CredentialName:      "cred",
			CredentialOwner:     "owner",
			Life:                "alive",
			SourceMigrationUUID: "migration-uuid",
		},
		Users: []params.ModelUser{{
			Name:        "fred",
			DisplayName: "Fred",
			CreatedBy:   "admin",
			CreatedAt:   created,
		}},
		ModelCredential: &params.ModelCloudCredential{
			Cloud:         "aws",
			Owner:         "owner",
			Name:          "cred",
			AuthType:      "access-key",
			Attributes:    map[string]string{"key": "value"},
			Revoked:       true,
			Invalid:       true,
			InvalidReason: "because",
		},
		Permissions: []params.ModelPermission{{
			ObjectType:  "model",
			GrantOn:     "model-uuid",
			SubjectName: "fred",
			Access:      "read",
		}, {
			ObjectType:  "offer",
			GrantOn:     "offer-uuid",
			SubjectName: "fred",
			Access:      "consume",
		}},
		AuthorizedKeys: []params.ModelAuthorizedKey{{
			Username:  "fred",
			PublicKey: "ssh-rsa AAAA",
		}},
		SecretBackend: &params.ModelSecretBackend{
			Name:        "vault",
			BackendType: "vault",
		},
		SecretBackendRefs: []params.SecretBackendReference{{
			BackendName:        "vault",
			SecretRevisionUUID: "rev-uuid",
			SecretID:           "secret-id",
		}},
		Leases: []params.Lease{{
			Type:   "application-leadership",
			Name:   "app",
			Holder: "app/0",
		}},
		LastLogins: []params.ModelLastLogin{{
			Username: "fred",
			Time:     created,
		}},
		CloudImageMetadata: []params.ModelCloudImageMetadata{{
			Stream:          "released",
			Region:          "eu-west-1",
			Version:         "22.04",
			Arch:            "amd64",
			VirtType:        "kvm",
			RootStorageType: "ssd",
			RootStorageSize: &size,
			Source:          "custom",
			Priority:        7,
			ImageId:         "ami-1",
			CreatedAt:       created,
		}},
		ExternalControllers: []params.ExternalControllerRef{{
			UUID:           "ext-uuid",
			Alias:          "other",
			CACert:         "ca-cert",
			Addresses:      []string{"10.0.0.1:17070"},
			ConsumedModels: []string{"offerer-model-uuid"},
		}},
	})
}

func (*EnvelopeSuite) TestEnvelopeFromControllerModelInfoEmpty(c *tc.C) {
	envelope := envelopeFromControllerModelInfo(modelmigration.ControllerModelInfo{
		ModelInfo: modelmigration.ModelIdentityInfo{UUID: "model-uuid"},
	}, "migration-uuid")

	c.Check(envelope.ModelCredential, tc.IsNil)
	c.Check(envelope.SecretBackend, tc.IsNil)
	c.Check(envelope.Users, tc.HasLen, 0)
	c.Check(envelope.Permissions, tc.HasLen, 0)
	c.Check(envelope.ModelInfo.SourceMigrationUUID, tc.Equals, "migration-uuid")
}

func (*EnvelopeSuite) TestCharmURLsFromLocators(c *tc.C) {
	urls, err := charmURLsFromLocators([]applicationcharm.CharmLocator{{
		Name:         "wordpress",
		Revision:     42,
		Source:       applicationcharm.CharmHubSource,
		Architecture: architecture.AMD64,
	}, {
		Name:         "local-thing",
		Revision:     1,
		Source:       applicationcharm.LocalSource,
		Architecture: architecture.Unknown,
	}})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(urls, tc.DeepEquals, []string{
		"ch:amd64/wordpress-42",
		"local:local-thing-1",
	})
}

func (*EnvelopeSuite) TestCharmURLsFromLocatorsEmpty(c *tc.C) {
	urls, err := charmURLsFromLocators(nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(urls, tc.IsNil)
}

func (*EnvelopeSuite) TestCharmURLsFromLocatorsUnsupportedSource(c *tc.C) {
	_, err := charmURLsFromLocators([]applicationcharm.CharmLocator{{
		Name:   "wat",
		Source: applicationcharm.CharmSource("wat"),
	}})
	c.Assert(err, tc.ErrorMatches, `unsupported charm source "wat"`)
}

func (*EnvelopeSuite) TestToolsForEnvelope(c *tc.C) {
	machineTools := map[machine.Name]coreagentbinary.Metadata{
		"0": {
			Version: coreagentbinary.Version{
				Number: semversion.MustParse("4.0.6"),
				Arch:   arch.AMD64,
			},
			SHA256: "sha-amd64",
		},
		"1": {
			Version: coreagentbinary.Version{
				Number: semversion.MustParse("4.0.6"),
				Arch:   arch.ARM64,
			},
			SHA256: "sha-arm64",
		},
	}
	unitTools := map[unit.Name]coreagentbinary.Metadata{
		// Same binary as machine 0; deduplicated by SHA256.
		"app/0": {
			Version: coreagentbinary.Version{
				Number: semversion.MustParse("4.0.6"),
				Arch:   arch.AMD64,
			},
			SHA256: "sha-amd64",
		},
	}

	tools, envelopeTools := toolsForEnvelope(machineTools, unitTools)
	c.Check(tools, tc.DeepEquals, map[string]semversion.Binary{
		"sha-amd64": semversion.MustParseBinary("4.0.6-ubuntu-amd64"),
		"sha-arm64": semversion.MustParseBinary("4.0.6-ubuntu-arm64"),
	})
	c.Check(envelopeTools, tc.DeepEquals, []params.SerializedModelTools{{
		Version: "4.0.6-ubuntu-amd64",
		URI:     "/tools/4.0.6-ubuntu-amd64",
		SHA256:  "sha-amd64",
	}, {
		Version: "4.0.6-ubuntu-arm64",
		URI:     "/tools/4.0.6-ubuntu-arm64",
		SHA256:  "sha-arm64",
	}})
}

func (*EnvelopeSuite) TestToolsForEnvelopeEmpty(c *tc.C) {
	tools, envelopeTools := toolsForEnvelope(nil, nil)
	c.Check(tools, tc.HasLen, 0)
	c.Check(envelopeTools, tc.IsNil)
}

func (*EnvelopeSuite) TestResourcesForEnvelope(c *tc.C) {
	fp, err := charmresource.GenerateFingerprint(strings.NewReader("a resource body"))
	c.Assert(err, tc.ErrorIsNil)
	uploaded := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	out := resourcesForEnvelope([]coreresource.Resource{{
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name: "blob",
				Type: charmresource.TypeFile,
			},
			Origin:      charmresource.OriginUpload,
			Revision:    3,
			Fingerprint: fp,
			Size:        15,
		},
		ApplicationName: "app",
		RetrievedBy:     "fred",
		Timestamp:       uploaded,
	}, {
		// A placeholder resource with no stored blob.
		Resource: charmresource.Resource{
			Meta: charmresource.Meta{
				Name: "pending",
				Type: charmresource.TypeContainerImage,
			},
			Origin: charmresource.OriginStore,
		},
		ApplicationName: "app",
	}})
	c.Check(out, tc.DeepEquals, []params.SerializedModelResource{{
		Application:    "app",
		Name:           "blob",
		Revision:       3,
		Type:           "file",
		Origin:         "upload",
		FingerprintHex: fp.Hex(),
		Size:           15,
		Timestamp:      uploaded,
		Username:       "fred",
	}, {
		Application: "app",
		Name:        "pending",
		Type:        "oci-image",
		Origin:      "store",
	}})
}

func (*EnvelopeSuite) TestResourcesForEnvelopeEmpty(c *tc.C) {
	c.Check(resourcesForEnvelope(nil), tc.IsNil)
}
