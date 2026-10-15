// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

// recoveryExclusions is the replay exclusion contract for both databases.
// The decoder accepts these archive tables without selecting them, and the
// loader excludes them even if a caller constructs typed bindings directly.
// Logical controller/model identities and object metadata are restored: unlike
// migration, recovery preserves identity and restores the archived blobs.
// Source change history is not replayed; target triggers record replay writes.
// Replay must finish before watchers start and obtain their initial snapshots.
// The replacement has new node membership and may have different endpoints,
// even when its externally advertised address is preserved.
var recoveryExclusions = map[string]string{
	"schema": "Keep the target's applied migration bookkeeping.",

	"change_log":           "Do not replay source change notifications.",
	"change_log_witness":   "Keep target watcher progress.",
	"change_log_edit_type": "Identical schema seeds need no replay.",
	"change_log_namespace": "Identical schema seeds need no replay.",

	"controller_node":               "Keep replacement controller nodes.",
	"controller_node_agent_version": "Keep replacement node agent versions.",
	"controller_node_password":      "Keep replacement node credentials.",
	"controller_node_nonce":         "Keep replacement node introduction nonces.",
	"controller_api_address":        "Keep replacement API endpoints.",
	"controller_agent_address":      "Keep replacement agent endpoints.",
	"controller_client_address":     "Keep replacement client endpoints.",
	"controller_peer_address":       "Keep replacement peer endpoints.",

	// A backup can include its own lease and pins, as well as ownership by
	// source controllers that no longer exist. Pinned leases cannot expire.
	"lease":                        "Reacquire ownership on the replacement.",
	"lease_pin":                    "Do not restore non-expiring source ownership.",
	"upgrade_info":                 "Keep target upgrade progress.",
	"upgrade_info_controller_node": "Keep target node upgrade progress.",

	"object_store_placement": "Rebuild placements using the supplied target node.",
	"log_transfer_progress":  "Controller-local transfer progress is not exported.",
}
