// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package remoterelationconsumer defines workers which manage the operation of
// cross model relations from the consuming model's perspective.
//
// Worker hierarchy:
//
//		Worker (top-level, watches remote application offerers in the consumer model)
//		 └─ localConsumerWorker (one per remote application offerer)
//		     ├─ consumerunitrelations.localWorker   (one per relation)
//		     ├─ offererunitrelations.remoteWorker   (one per relation)
//		     └─ offererrelations.offererRelationsWorker (one per relation)
//
//	 - `Worker`: Top-level worker that watches for remote application offerer
//	   changes and manages one `localConsumerWorker` per offerer using a runner.
//	   Also watches for model-dying events to propagate cleanup notifications.
//
//	 - `localConsumerWorker`: Per-offerer worker that coordinates cross model
//	   relation lifecycle. Watches local relation life/suspended status and
//	   remote offer status. Registers relations with the offering model,
//	   starts sub-workers per relation, and handles degraded mode when offer
//	   permissions are revoked (retries establishing the offer status watcher
//	   periodically while suspending local relations).
//
//	 - `consumerunitrelations.localWorker`: Watches local relation unit changes
//	   (units entering/leaving scope, settings) and forwards them to the parent
//	   `localConsumerWorker`, which publishes them to the offering model.
//
//	 - `offererunitrelations.remoteWorker`: Watches relation unit changes in the
//	   offering model via the remote API and forwards them to the parent
//	   `localConsumerWorker`, which applies them locally.
//
//	 - `offererrelations.offererRelationsWorker`: Watches the life and suspended
//	   status of a specific relation in the offering model and forwards changes
//	   to the parent `localConsumerWorker`, which mirrors them locally or removes
//	   the relation.
//
// The consuming side pushes relation unit updates from the consumer
// application to the model containing the offer. It also observes relation
// unit changes and status changes from the offer and applies them to the
// consuming model.
package remoterelationconsumer
