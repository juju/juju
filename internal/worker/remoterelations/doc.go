// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package remoterelations defines workers which manage the operation of cross model relations.
//   - `Worker`: Top level worker. Watches SaaS applications/proxies and creates a worker for each.
//   - `remoteApplicationWorker`: Manages operations for a consumer or offer proxy. Consumes and publishes relation data and status changes.
//   - `remoteRelationsWorker`: Runs on the consuming model to manage relations to the offer.
//   - `relationUnitsWorker`: Runs on the consuming model to receive and publish changes to each relation unit data bag.
//
// The consuming side pushes relation updates from the consumer application to the model containing
// the offer. It also polls the offered application to record relation changes from the offer into
// the consuming model.
package remoterelations
