# Juju Schema Rules for Coding Agents

These rules apply to `domain/schema/` and its sub-packages and supplement the
[domain guidance](../AGENTS.md).

## Schema Evolution

- Before editing an existing schema patch, check its release history and the
  supported upgrade path for the target release. Distinguish beta-only changes
  from patches shipped in release candidates or final releases.
- Preserve existing patch contents and ordering when supported installations
  must upgrade in place. Patch hashes form a chain; changing an applied patch
  or inserting one before it breaks that chain.
- Base schema files may change when no supported in-place upgrade depends on
  their existing contents. Do not require an incremental patch solely because
  a file existed in a beta. Check the applicable upgrade policy and migration
  path before deciding.
- For incremental changes within the same major.minor version, add a
  `.PATCH.sql` file under `controller/sql/` or `model/sql/`. Register it in
  `controllerPostPatchFilesByVersion` in [controller.go](controller.go) or
  `modelPostPatchFilesByVersion` in [model.go](model.go), at the version where
  it is first applied. File creation alone does not register a post-patch.
- Verify fresh database creation and supported cumulative in-place upgrades,
  including preservation or intentional transformation of existing data. Use
  the schema suites in [controller_schema_test.go](controller_schema_test.go)
  and [model_schema_test.go](model_schema_test.go).
