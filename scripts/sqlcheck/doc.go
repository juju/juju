// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Command sqlcheck extracts SQL embedded in Juju's Go source, renders it
// into SQLite-compatible SQL for the repository's pinned sqlfluff, and
// reports any linting diagnostics against the original Go file, line,
// and column.
//
// SQL that cannot be statically reconstructed, such as imperative string
// composition, is reported in a coverage summary instead of being
// silently skipped. Use `make lint-sql` from the repository root.
//
// # Suppressing diagnostics
//
// Add a `// sqlcheck-ignore` comment on the line of the SQL statement,
// or on the line immediately before it, to suppress its diagnostics:
//
//	// sqlcheck-ignore
//	st := s.db.Prepare("SELECT  id FROM u")
//
// Only specific rules may be suppressed by listing them:
//
//	st := s.db.Prepare("SELECT id FROM u") // sqlcheck-ignore LT01,CP01
//
// # Dummy tokens
//
// Values that cannot be known statically, such as fmt verbs, are
// rendered as context-safe placeholder tokens so the SQL stays
// parseable. Reference rules may therefore report on the placeholder
// rather than the original expression. A sqlair &Type.* outcome renders
// as a representative column; rules that apply to the star itself, such
// as column expansion, are not applicable to the rendered SQL.
package main
