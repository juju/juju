// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package schema

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/juju/collections/set"
	jc "github.com/juju/testing/checkers"
	_ "github.com/mattn/go-sqlite3"
	gc "gopkg.in/check.v1"

	databasetesting "github.com/juju/juju/internal/database/testing"
)

type schemaSuite struct {
	databasetesting.DqliteSuite
}

var _ = gc.Suite(&schemaSuite{})

func (s *schemaSuite) TestControllerDDLApply(c *gc.C) {
	c.Logf("Committing schema DDL")

	schema := ControllerDDL()
	changeSet, err := schema.Ensure(context.Background(), s.TxnRunner())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(changeSet.Current, gc.Equals, 0)
	c.Check(changeSet.Post > 0, jc.IsTrue)

	// Ensure that each table is present.
	expected := set.NewStrings(
		// Autocert cache
		"autocert_cache",
		"autocert_cache_encoding",

		// Leases
		"lease",
		"lease_type",
		"lease_pin",

		// Change log
		"change_log",
		"change_log_edit_type",
		"change_log_namespace",
		"change_log_witness",

		// Cloud
		"cloud",
		"auth_type",
		"cloud_auth_type",
		"cloud_ca_cert",
		"cloud_credential",
		"cloud_credential_attributes",
		"cloud_defaults",
		"cloud_region",
		"cloud_region_defaults",
		"cloud_type",

		// External controller
		"external_controller",
		"external_controller_address",
		"external_model",

		// Model
		"model_list",
		"model_metadata",
		"model_type",

		// Controller config
		"controller_config",

		// Controller nodes
		"controller_node",

		// Model migration
		"model_migration",
		"model_agent",
		"model_migration_status",
		"model_migration_user",
		"model_migration_minion_sync",

		// Upgrade info
		"upgrade_info",
		"upgrade_info_controller_node",
		"upgrade_state_type",

		// Object store metadata
		"object_store_metadata",
		"object_store_metadata_path",
		"object_store_metadata_hash_type",

		// Users
		"user",
		"user_authentication",
		"user_password",
		"user_activation_key",
		"v_user_auth",

		// Flags
		"flag",

		// Permissions
		"permission_access_type",
		"permission_object_access",
		"permission_object_type",
		"permission",
	)
	c.Assert(readTableNames(c, s.DB()), jc.SameContents, expected.Union(internalTableNames).SortedValues())
}

func (s *schemaSuite) TestModelDDLApply(c *gc.C) {
	schema := ModelDDL()
	changeSet, err := schema.Ensure(context.Background(), s.TxnRunner())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(changeSet.Current, gc.Equals, 0)
	c.Check(changeSet.Post > 0, jc.IsTrue)

	// Ensure that each table is present.
	expected := set.NewStrings(
		// Annotations
		"annotation_application",
		"annotation_charm",
		"annotation_machine",
		"annotation_unit",
		"annotation_model",
		"annotation_storage_volume",
		"annotation_storage_filesystem",

		"life",

		// Change log
		"change_log",
		"change_log_edit_type",
		"change_log_namespace",
		"change_log_witness",

		// Model config
		"model_config",

		// Object store metadata
		"object_store_metadata",
		"object_store_metadata_path",
		"object_store_metadata_hash_type",

		"application",
		"charm",
		"machine",
		"net_node",
		"cloud_service",
		"cloud_container",
		"unit",

		// Spaces
		"space",
		"provider_space",

		// Subnets
		"subnet",
		"subnet_association_type",
		"subnet_type",
		"subnet_type_association_type",
		"subnet_association",
		"provider_subnet",
		"provider_network",
		"provider_network_subnet",
		"availability_zone",
		"availability_zone_subnet",

		// Block device
		"block_device",
		"filesystem_type",
		"block_device_link_device",

		// Storage
		"storage_pool",
		"storage_pool_attribute",
		"storage_kind",
		"storage_instance",
		"storage_instance_pool",
		"storage_unit_owner",
		"storage_attachment",
		"storage_constraint_type",
		"storage_instance_constraint",
		"storage_volume",
		"storage_instance_volume",
		"storage_volume_attachment",
		"storage_filesystem",
		"storage_instance_filesystem",
		"storage_filesystem_attachment",
		"storage_volume_attachment_plan",
		"storage_volume_attachment_plan_attr",
		"storage_provisioning_status",
		"storage_volume_device_type",
	)
	c.Assert(readTableNames(c, s.DB()), jc.SameContents, expected.Union(internalTableNames).SortedValues())
}

// NewCleanDB returns a new sql.DB reference.
func (s *schemaSuite) NewCleanDB(c *gc.C) *sql.DB {
	dir := c.MkDir()

	url := fmt.Sprintf("file:%s/db.sqlite3?_foreign_keys=1", dir)
	c.Logf("Opening sqlite3 db with: %v", url)

	db, err := sql.Open("sqlite3", url)
	c.Assert(err, jc.ErrorIsNil)

	return db
}

var (
	internalTableNames = set.NewStrings(
		"schema",
		"sqlite_sequence",
	)
)

func readTableNames(c *gc.C, db *sql.DB) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := db.BeginTx(ctx, nil)
	c.Assert(err, jc.ErrorIsNil)

	rows, err := tx.QueryContext(ctx, "SELECT tbl_name FROM sqlite_master")
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = rows.Close() }()

	var tables []string
	for rows.Next() {
		var table string
		err = rows.Scan(&table)
		c.Assert(err, jc.ErrorIsNil)
		tables = append(tables, table)
	}

	err = tx.Commit()
	c.Assert(err, jc.ErrorIsNil)

	return set.NewStrings(tables...).SortedValues()
}
