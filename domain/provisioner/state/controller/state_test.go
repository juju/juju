// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"database/sql"
	"testing"

	"github.com/juju/tc"

	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type controllerStateSuite struct {
	schematesting.ControllerSuite

	state *State
}

func TestStateSuite(t *testing.T) {
	tc.Run(t, &controllerStateSuite{})
}

func (s *controllerStateSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)
	s.state = NewState(s.TxnRunnerFactory(), loggertesting.WrapCheckLog(c))
}

// runQuery executes an SQL statement for test setup.
func (s *controllerStateSuite) runQuery(c *tc.C, query string, args ...any) {
	err := s.TxnRunner().StdTxn(c.Context(), func(ctx context.Context, tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, query, args...)
		if err != nil {
			return errors.Errorf("%w: query: %s (args: %v)", err, query, args)
		}
		return nil
	})
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("(Arrange) failed to populate DB: %v",
		errors.ErrorStack(err)))
}

// TestGetControllerConfigEmpty verifies that when neither the controller
// table nor controller_config is populated, an empty map is returned.
func (s *controllerStateSuite) TestGetControllerConfigEmpty(c *tc.C) {
	result, err := s.state.GetControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 0)
}

// TestGetControllerConfigFromView verifies that the view includes
// controller-uuid, ca-cert, and api-port from the controller table
// as well as entries from controller_config.
func (s *controllerStateSuite) TestGetControllerConfigFromView(c *tc.C) {
	s.runQuery(c, `INSERT INTO controller (uuid, model_uuid, target_version, api_port, ca_cert) VALUES (?,?,?,?,?)`,
		"ctrl-uuid-abc", "model-uuid-123", "4.0.0", "17070", "test-ca-cert")
	s.runQuery(c, `INSERT INTO controller_config ("key", value) VALUES (?,?)`,
		"model-logfile-max-size", "10M")

	result, err := s.state.GetControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result["controller-uuid"], tc.Equals, "ctrl-uuid-abc")
	c.Check(result["ca-cert"], tc.Equals, "test-ca-cert")
	c.Check(result["api-port"], tc.Equals, "17070")
	c.Check(result["model-logfile-max-size"], tc.Equals, "10M")
}

// TestGetControllerConfigNoAPIPort verifies that when api_port is NULL
// in the controller table, the view omits the api-port key.
func (s *controllerStateSuite) TestGetControllerConfigNoAPIPort(c *tc.C) {
	s.runQuery(c, `INSERT INTO controller (uuid, model_uuid, target_version, ca_cert) VALUES (?,?,?,?)`,
		"ctrl-uuid-abc", "model-uuid-123", "4.0.0", "test-ca-cert")

	result, err := s.state.GetControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result["controller-uuid"], tc.Equals, "ctrl-uuid-abc")
	c.Check(result["ca-cert"], tc.Equals, "test-ca-cert")
	_, hasAPIPort := result["api-port"]
	c.Check(hasAPIPort, tc.IsFalse)
}

// TestGetControllerConfigSingleEntry verifies a single config entry.
func (s *controllerStateSuite) TestGetControllerConfigSingleEntry(c *tc.C) {
	s.runQuery(c, `INSERT INTO controller_config ("key", value) VALUES (?,?)`,
		"model-logfile-max-size", "10M")

	result, err := s.state.GetControllerConfig(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result["model-logfile-max-size"], tc.Equals, "10M")
}

// TestGetCloudEndpointNoCloud verifies that a missing cloud returns empty.
func (s *controllerStateSuite) TestGetCloudEndpointNoCloud(c *tc.C) {
	endpoint, err := s.state.GetCloudEndpoint(c.Context(), "nocloud", "")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(endpoint, tc.Equals, "")
}

// TestGetCloudEndpointCloudLevel verifies cloud-level endpoint is returned.
func (s *controllerStateSuite) TestGetCloudEndpointCloudLevel(c *tc.C) {
	s.setupCloud(c, "mycloud", "https://cloud.example.com:5000/v3")

	endpoint, err := s.state.GetCloudEndpoint(c.Context(), "mycloud", "")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(endpoint, tc.Equals, "https://cloud.example.com:5000/v3")
}

// TestGetCloudEndpointRegionOverride verifies that a region endpoint
// takes precedence over the cloud endpoint.
func (s *controllerStateSuite) TestGetCloudEndpointRegionOverride(c *tc.C) {
	cloudUUID := s.setupCloud(c, "mycloud", "https://cloud.example.com:5000/v3")
	s.runQuery(c, `INSERT INTO cloud_region (uuid, cloud_uuid, name, endpoint) VALUES (?,?,?,?)`,
		"region-uuid-1", cloudUUID, "us-east-1", "https://us-east-1.example.com:5000/v3")

	endpoint, err := s.state.GetCloudEndpoint(c.Context(), "mycloud", "us-east-1")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(endpoint, tc.Equals, "https://us-east-1.example.com:5000/v3")
}

// TestGetCloudEndpointRegionNoEndpoint verifies that if the region has no
// endpoint, the cloud-level endpoint is returned.
func (s *controllerStateSuite) TestGetCloudEndpointRegionNoEndpoint(c *tc.C) {
	cloudUUID := s.setupCloud(c, "mycloud", "https://cloud.example.com:5000/v3")
	s.runQuery(c, `INSERT INTO cloud_region (uuid, cloud_uuid, name) VALUES (?,?,?)`,
		"region-uuid-1", cloudUUID, "us-east-1")

	endpoint, err := s.state.GetCloudEndpoint(c.Context(), "mycloud", "us-east-1")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(endpoint, tc.Equals, "https://cloud.example.com:5000/v3")
}

// TestGetCachedImageMetadataEmpty verifies empty result for no metadata.
func (s *controllerStateSuite) TestGetCachedImageMetadataEmpty(c *tc.C) {
	result, err := s.state.GetCachedImageMetadata(c.Context(), "22.04", "amd64", "", "")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 0)
}

// TestGetCachedImageMetadataFiltered verifies filtering by version and arch.
func (s *controllerStateSuite) TestGetCachedImageMetadataFiltered(c *tc.C) {
	s.insertImageMetadata(c, "img-1", "released", "us-east-1", "22.04", 0, "hvm", "ebs", "custom", 10)
	s.insertImageMetadata(c, "img-2", "released", "us-east-1", "22.04", 1, "hvm", "ebs", "custom", 20)
	s.insertImageMetadata(c, "img-3", "released", "us-east-1", "24.04", 0, "hvm", "ebs", "custom", 30)

	// Filter for 22.04 + amd64 should return img-1 only.
	result, err := s.state.GetCachedImageMetadata(c.Context(), "22.04", "amd64", "", "")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 1)
	c.Check(result[0].ImageID, tc.Equals, "img-1")
	c.Check(result[0].Stream, tc.Equals, "released")
	c.Check(result[0].Version, tc.Equals, "22.04")
	c.Check(result[0].Arch, tc.Equals, "amd64")
	c.Check(result[0].Priority, tc.Equals, 10)
}

// TestGetCachedImageMetadataNoFilter verifies all results returned with
// empty filters.
func (s *controllerStateSuite) TestGetCachedImageMetadataNoFilter(c *tc.C) {
	s.insertImageMetadata(c, "img-1", "released", "us-east-1", "22.04", 0, "hvm", "ebs", "custom", 10)
	s.insertImageMetadata(c, "img-2", "daily", "eu-west-1", "24.04", 1, "hvm", "ssd", "custom", 20)

	result, err := s.state.GetCachedImageMetadata(c.Context(), "", "", "", "")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 2)
}

// TestGetCachedImageMetadataRootStorageSize verifies root storage size is
// returned correctly.
func (s *controllerStateSuite) TestGetCachedImageMetadataRootStorageSize(c *tc.C) {
	s.runQuery(c, `INSERT INTO cloud_image_metadata
		(uuid, created_at, source, stream, region, version, architecture_id,
		 virt_type, root_storage_type, root_storage_size, priority, image_id)
		VALUES (?,datetime('now'),?,?,?,?,?,?,?,?,?,?)`,
		"meta-uuid-1", "custom", "released", "us-east-1", "22.04", 0,
		"hvm", "ebs", 8192, 10, "img-with-size")

	result, err := s.state.GetCachedImageMetadata(c.Context(), "22.04", "amd64", "", "")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 1)
	c.Assert(result[0].RootStorageSize, tc.Not(tc.IsNil))
	c.Check(*result[0].RootStorageSize, tc.Equals, uint64(8192))
}

// TestGetCachedImageMetadataFilteredByRegionAndStream verifies filtering by
// region and stream in addition to version and arch.
func (s *controllerStateSuite) TestGetCachedImageMetadataFilteredByRegionAndStream(c *tc.C) {
	s.insertImageMetadata(c, "img-1", "released", "us-east-1", "22.04", 0, "hvm", "ebs", "custom", 10)
	s.insertImageMetadata(c, "img-2", "daily", "us-east-1", "22.04", 0, "hvm", "ebs", "custom", 20)
	s.insertImageMetadata(c, "img-3", "released", "eu-west-1", "22.04", 0, "hvm", "ebs", "custom", 30)

	// Filter for released + us-east-1 should only return img-1.
	result, err := s.state.GetCachedImageMetadata(c.Context(), "22.04", "amd64", "us-east-1", "released")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 1)
	c.Check(result[0].ImageID, tc.Equals, "img-1")

	// Filter for daily stream should only return img-2.
	result, err = s.state.GetCachedImageMetadata(c.Context(), "22.04", "amd64", "", "daily")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 1)
	c.Check(result[0].ImageID, tc.Equals, "img-2")

	// Filter for eu-west-1 should only return img-3.
	result, err = s.state.GetCachedImageMetadata(c.Context(), "", "", "eu-west-1", "")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.HasLen, 1)
	c.Check(result[0].ImageID, tc.Equals, "img-3")
}

// setupCloud inserts a cloud and returns its UUID.
func (s *controllerStateSuite) setupCloud(c *tc.C, name, endpoint string) string {
	uuid := "cloud-uuid-" + name
	s.runQuery(c, `INSERT INTO cloud (uuid, name, cloud_type_id, endpoint, skip_tls_verify) VALUES (?,?,?,?,?)`,
		uuid, name, 5, endpoint, false)
	return uuid
}

// insertImageMetadata inserts a cloud image metadata row.
func (s *controllerStateSuite) insertImageMetadata(c *tc.C, imageID, stream, region, version string, archID int, virtType, rootStorageType, source string, priority int) {
	uuid := "meta-uuid-" + imageID
	s.runQuery(c, `INSERT INTO cloud_image_metadata
		(uuid, created_at, source, stream, region, version, architecture_id,
		 virt_type, root_storage_type, priority, image_id)
		VALUES (?,datetime('now'),?,?,?,?,?,?,?,?,?)`,
		uuid, source, stream, region, version, archID,
		virtType, rootStorageType, priority, imageID)
}
