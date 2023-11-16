// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package remoterelations defines workers which manage the operation of cross model relations.
//   - `Worker`: top level worker, watches saas applications/proxies and creates a worker for each.
//   - `remoteApplicationWorker`: manages operations for a consumer or offer proxy;
//     consume and publish relation data and status changes.
//   - `remoteRelationsWorker`: runs on the consuming model to manages relations to the offer.
//   - `relationUnitsWorker`: runs on the consuming model to receive and publish on changes to
//     each relation unit data bag.
//
// The consuming side pushes relation updates from the consumer application to the model containing
// the offer. It also polls the offered application to record relation changes from the offer into
// the consuming model.
package remoterelations
