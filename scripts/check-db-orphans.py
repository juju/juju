#!/usr/bin/env python3

# Copyright 2026 Canonical Ltd.
# Licensed under the AGPLv3, see LICENCE file for details.
#
# Usage:
#   juju dump-db -m <model> > dump.yaml
#   python3 scripts/check-db-orphans.py dump.yaml
#   python3 scripts/check-db-orphans.py dump.yaml --cleanup
#   python3 scripts/check-db-orphans.py dump.yaml --manual-cleanup
#
# Reads a `juju dump-db` YAML dump and reports documents that reference
# entities which no longer exist (orphans).  It also validates offers: stale
# applicationOffers (no backing application) and applicationOfferConnections
# with a missing local relation or offer, a mismatched relation-key, a
# missing offered application endpoint, or a missing consumer proxy.
# Relations referencing a remote application whose document is gone are
# flagged too: the goal-state hook tool fails on them, breaking charm
# hooks that call planned_units().
# Run with --cleanup to also emit
# a JSON ops file suitable for scripts/mgo-run-txn, which executes the
# removals through Juju's transaction machinery so that txn-revnos
# and the txns recovery log stay consistent.
#
# Findings are classified:
#   ORPHAN  - the referenced entity must exist; it does not. Safe to remove
#             (cleanup ops are generated for these).  Cross-model offer
#             findings are report-only except stale
#             applicationOfferConnections (missing local relation or
#             offer), which are removed.
#   MISMATCH - a recorded counter or reference disagrees with model state
#             (e.g. remote application relationcount). Fixed by correcting
#             the value; cleanup ops are generated.
#   CRASH    - a relation scope exists without its settings document; the
#             relationUnitsWatcher fails on this and crashes the unit
#             agents. Manual repair required; never auto-cleaned.
#
# The report groups findings by repair action: AUTO-REPAIRABLE (the
# --cleanup ops remove or correct them) and MANUAL REPAIR (never touched
# by cleanup ops; printed with a suggested juju CLI fix and the checks to
# make before applying it).  For relation findings - an endpoint
# application missing from the model, or a broken cross-model half -
# --manual-cleanup additionally writes a ops file tearing the
# relation down at the DB level; the consuming model's half must be cleaned
# separately.  No ops can be generated for stale offers: their permissions
# live in the controller model, outside this dump.
#
# Exit codes: 0 no findings, 1 any ORPHAN/MISMATCH/CRASH found,
# 2 usage/input error.

from __future__ import print_function

import argparse
import collections
import json
import os
import re
import sys
import textwrap
import uuid

try:
    import yaml
except ImportError:
    sys.stderr.write("error: PyYAML is required (pip install pyyaml)\n")
    sys.exit(2)

MODEL_UUID_RE = re.compile(
    r"^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$")
REMOTE_UNIT_RE = re.compile(r"^remote-(.+)\/\d+$")
REMOTE_APP_RE = re.compile(r"^remote-(.+)$")


class Finding(object):
    __slots__ = ("severity", "kind", "collection", "doc_id", "ref", "detail")

    def __init__(self, severity, kind, collection, doc_id, ref, detail=""):
        self.severity = severity
        self.kind = kind
        self.collection = collection
        self.doc_id = doc_id
        self.ref = ref
        self.detail = detail

    def as_dict(self):
        return {
            "severity": self.severity,
            "kind": self.kind,
            "collection": self.collection,
            "doc_id": self.doc_id,
            "ref": self.ref,
            "detail": self.detail,
        }


def strip_prefix(raw_id, uuid):
    prefix = uuid + ":"
    if raw_id.startswith(prefix):
        return raw_id[len(prefix):]
    return None


def first_token(local_id, prefix_len):
    rest = local_id[prefix_len:]
    return rest.split("#", 1)[0]


def parse_relation_id_token(local_id, prefix_len):
    token = first_token(local_id, prefix_len)
    try:
        return int(token)
    except ValueError:
        return None


def is_remote_unit(name):
    return bool(REMOTE_UNIT_RE.match(name))


def is_remote_app(name):
    return bool(REMOTE_APP_RE.match(name))


def unit_app(unit):
    """App name of a unit - 'app/0' -> 'app'."""
    return unit.rsplit("/", 1)[0] if "/" in unit else unit


def is_remote_unit_name(model, unit):
    """True if unit belongs to a remote application.

    Remote units never appear in the units collection; they exist only in
    relationscopes/settings/unitstates as '<remote-app>/<n>'.  Legacy
    cross-model units are named 'remote-<source-model-uuid>/<n>', modern
    ones use the consumed offer's application name directly, so the
    authoritative check is membership in the remoteApplications collection.
    """
    if is_remote_unit(unit):
        return True
    return unit_app(unit) in model.remote_apps


def is_remote_app_name(model, app):
    return is_remote_app(app) or app in model.remote_apps


class ModelState(object):
    """Reference sets and findings for a single model."""

    def __init__(self, uuid):
        self.uuid = uuid
        self.units = set()
        self.applications = set()
        self.remote_apps = set()
        self.machines = set()
        self.relations = set()
        self.charms = set()
        # Relation-scope prefixes r#<id># covered by a pending settings
        # cleanup document; their settings should not be touched.
        self.pending_settings_cleanups = set()
        # Relation ids covered by a pending forceDestroyRelation cleanup;
        # the relation and its offer connections are being torn down.
        self.pending_force_relation_cleanups = set()
        # Relation keys covered by a pending forceDestroyRelation
        # cleanup whose prefix is the relation key rather than the id
        # (state/cleanup.go handles both forms); same as above.
        self.pending_force_relation_keys = set()
        # collection -> {raw_id: doc}
        self.docs = {}
        self.findings = []
        self.repair_plan = None

    def add_finding(self, severity, kind, collection, raw_id, ref, detail=""):
        self.findings.append(
            Finding(severity, kind, collection, raw_id, ref, detail))

    def relation_missing(self, rid):
        return rid not in self.relations


def build_models(dump, findings):
    """Split the dumped docs by model and build reference sets.

    Returns {model_uuid: ModelState}. Doc ids for model-scoped collections
    are `<uuid>:<globalkey>`; ids not prefixed with a known model uuid are
    reported as foreign.
    """
    uuids = set()
    model_docs = dump.get("models")
    if isinstance(model_docs, dict):
        model_docs = [model_docs]
    for doc in model_docs or []:
        uid = doc.get("_id")
        if isinstance(uid, str):
            uuids.add(uid)

    models = {uid: ModelState(uid) for uid in uuids}
    for collection, docs in dump.items():
        if not isinstance(docs, list):
            continue
        for doc in docs:
            if not isinstance(doc, dict):
                continue
            raw_id = doc.get("_id")
            if collection == "models":
                continue
            uid = None
            if isinstance(raw_id, str):
                for candidate in uuids:
                    if raw_id.startswith(candidate + ":"):
                        uid = candidate
                        break
            if uid is None:
                # Collections with raw ObjectId keys (e.g. statuseshistory)
                # carry the model in the model-uuid field instead.
                mu = doc.get("model-uuid")
                if isinstance(mu, str) and mu in models:
                    uid = mu
            if uid is None:
                findings.append(Finding(
                    "ORPHAN", "foreign-id", collection, raw_id,
                    "<unknown-model>",
                    "doc _id is not prefixed with any model uuid"))
                continue
            if not isinstance(raw_id, str):
                # A doc whose _id is not a string (missing, or a raw
                # ObjectId/int) cannot be looked up or referenced by the
                # other checks; report it and skip.
                findings.append(Finding(
                    "ORPHAN", "foreign-id", collection, raw_id,
                    "<no-string-id>",
                    "doc has no string _id (%r); attributed to model %s "
                    "only via model-uuid" % (raw_id, uid)))
                continue
            model = models[uid]
            model.docs.setdefault(collection, {})[raw_id] = doc
            local = strip_prefix(raw_id, uid)
            if collection == "units":
                model.units.add(local)
            elif collection == "applications":
                model.applications.add(local)
            elif collection == "machines":
                model.machines.add(local)
            elif collection == "relations":
                rid = doc.get("id")
                if isinstance(rid, int):
                    model.relations.add(rid)
            elif collection == "charms":
                model.charms.add(local)
            elif collection == "remoteApplications":
                model.remote_apps.add(doc.get("name", local))
            elif collection == "cleanups" and \
                    doc.get("kind") == "settings":
                # A queued settings cleanup (state/cleanup.go
                # cleanupRelationSettings, prefix "r#<id>#") will remove the
                # relation's settings docs; treat those as covered.
                pfx = doc.get("prefix")
                if isinstance(pfx, str) and pfx.startswith("r#"):
                    model.pending_settings_cleanups.add(pfx)
            elif collection == "cleanups" and \
                    doc.get("kind") == "forceDestroyRelation":
                # A queued force-destroy cleanup (state/relation.go,
                # prefix is the relation id) is tearing the relation and
                # its offer connections down; treat those as in flight.
                # Legacy cleanups use the relation key as the prefix
                # (state/cleanup.go still handles both).
                pfx = doc.get("prefix")
                if isinstance(pfx, str) and pfx:
                    if pfx.isdigit():
                        model.pending_force_relation_cleanups.add(
                            int(pfx))
                    else:
                        model.pending_force_relation_keys.add(pfx)
    return models


def check_foreign_model_uuids(model):
    for collection, docs in model.docs.items():
        for raw_id, doc in docs.items():
            mu = doc.get("model-uuid")
            if isinstance(mu, str) and mu != model.uuid:
                model.add_finding(
                    "ORPHAN", "model-uuid", collection, raw_id, mu,
                    "doc model-uuid does not match the _id prefix model")


def check_unitstates(model):
    docs = model.docs.get("unitstates", {})
    for raw_id, doc in docs.items():
        local = strip_prefix(raw_id, model.uuid) or raw_id
        if local.startswith("u#") and local.endswith("#charm"):
            unit = local[2:-len("#charm")]
            if unit not in model.units:
                model.add_finding(
                    "ORPHAN", "unitstates->unit", "unitstates", raw_id, unit)
        relation_state = doc.get("relation-state")
        if isinstance(relation_state, dict):
            unset_rids = []
            for key, value in relation_state.items():
                try:
                    rid = int(key)
                except (TypeError, ValueError):
                    continue
                if model.relation_missing(rid):
                    model.add_finding(
                        "ORPHAN", "unitstates->relation", "unitstates",
                        raw_id, "relation id %d" % rid,
                                "relation-state key %r" % key)
                    unset_rids.append(rid)
            if unset_rids:
                model.docs.setdefault("_cleanup", {}).setdefault(
                    "unitstates", {})[raw_id] = unset_rids


def check_relationscopes(model):
    docs = model.docs.get("relationscopes", {})
    for raw_id, doc in docs.items():
        local = strip_prefix(raw_id, model.uuid) or raw_id
        match = re.match(r"^r#(\d+)#", local)
        if not match:
            continue
        rid = int(match.group(1))
        if model.relation_missing(rid):
            # No pending cleanup will remove a scope whose relation is
            # already gone: the "settings" cleanup only removes settings
            # docs, and a forceDestroyRelation cleanup skips missing
            # relations (state/cleanup.go).  The scope is garbage.
            model.add_finding(
                "ORPHAN", "relationscopes->relation", "relationscopes",
                raw_id, "relation id %d" % rid)
            continue
        unit = local.rsplit("#", 1)[-1]
        if is_remote_unit_name(model, unit):
            continue
        if unit not in model.units:
            # A relation-scope doc for a unit that does not exist is garbage;
            # scopes are removed when units leave scope.
            model.add_finding(
                "ORPHAN", "relationscopes->unit", "relationscopes",
                raw_id, unit)


def check_relationscope_settings(model):
    """Flag relation scopes whose settings document is missing.

    A scope present without its settings document kills the controller-side
    relationUnitsWatcher (state/watcher.go mergeSettings fails and the
    watcher tears down), crashing every unit agent watching the relation.
    Scope and settings docs are created and removed in the same
    transactions, so a missing settings doc for a present scope is always
    anomalous.  Reported for manual repair - never auto-cleaned, since
    removing a scope on a live relation can corrupt it.
    """
    scopes = model.docs.get("relationscopes", {})
    if not scopes:
        return
    settings = model.docs.get("settings", {})
    for raw_id in scopes:
        local = strip_prefix(raw_id, model.uuid) or raw_id
        match = re.match(r"^r#(\d+)#", local)
        if not match:
            continue
        # Remote-unit scopes live in the offering model only; their
        # settings live in the consuming model, so no local settings doc is
        # expected here.
        unit = local.rsplit("#", 1)[-1]
        if is_remote_unit_name(model, unit):
            continue
        if model.relation_missing(int(match.group(1))):
            # Already reported as relationscopes->relation.
            continue
        if (model.uuid + ":" + local) not in settings:
            model.add_finding(
                "CRASH", "relationscopes->missing-settings",
                "relationscopes", raw_id,
                "settings %s" % local,
                "scope present without settings; relationUnitsWatcher "
                "fails and unit agents crash")


def check_settings(model):
    docs = model.docs.get("settings", {})
    for raw_id, doc in docs.items():
        local = strip_prefix(raw_id, model.uuid) or raw_id
        if local.startswith("r#"):
            # r#<id>#... settings docs are retained for the lifetime of the
            # relation regardless of the lifetime of the units/applications
            # they reference, so only check the relation itself.
            match = re.match(r"^r#(\d+)#", local)
            if not match:
                continue
            rid = int(match.group(1))
            if model.relation_missing(rid) and \
                    ("r#%d#" % rid) not in model.pending_settings_cleanups:
                model.add_finding(
                    "ORPHAN", "settings->relation", "settings", raw_id,
                    "relation id %d" % rid)
        elif local.startswith("a#"):
            app = first_token(local, 2)
            if app not in model.applications:
                model.add_finding(
                    "ORPHAN", "settings->application", "settings", raw_id, app)


def check_statuses(model):
    docs = model.docs.get("statuses", {})
    for raw_id, doc in docs.items():
        local = strip_prefix(raw_id, model.uuid) or raw_id
        if local.startswith("u#"):
            unit = first_token(local, 2)
            if unit not in model.units:
                model.add_finding(
                    "ORPHAN", "statuses->unit", "statuses", raw_id, unit)
        elif local.startswith("a#"):
            app = first_token(local, 2)
            if app not in model.applications:
                model.add_finding(
                    "ORPHAN", "statuses->application", "statuses", raw_id, app)
        elif local.startswith("m#"):
            machine = first_token(local, 2)
            if machine not in model.machines:
                model.add_finding(
                    "ORPHAN", "statuses->machine", "statuses", raw_id, machine)
        elif local.startswith("r#"):
            rid = parse_relation_id_token(local, 2)
            if rid is None:
                continue
            if model.relation_missing(rid):
                model.add_finding(
                    "ORPHAN", "statuses->relation", "statuses", raw_id,
                    "relation id %d" % rid)
        elif local.startswith("c#"):
            app = first_token(local, 2)
            if app not in model.remote_apps:
                model.add_finding(
                    "ORPHAN", "statuses->remote-app", "statuses", raw_id, app)
        elif local == "e":
            # "e" is the model's own global key; its doc is always valid.
            pass
        elif local.startswith("e#"):
            mu = first_token(local, 2)
            if mu != model.uuid:
                model.add_finding(
                    "ORPHAN", "statuses->model", "statuses", raw_id, mu)


def check_minunits(model):
    docs = model.docs.get("minunits", {})
    for raw_id, doc in docs.items():
        local = strip_prefix(raw_id, model.uuid) or raw_id
        if local not in model.applications:
            model.add_finding(
                "ORPHAN", "minunits->application", "minunits", raw_id, local)


def check_annotations(model):
    docs = model.docs.get("annotations", {})
    for raw_id, doc in docs.items():
        local = strip_prefix(raw_id, model.uuid) or raw_id
        if local.startswith("u#"):
            unit = first_token(local, 2)
            if unit not in model.units:
                model.add_finding(
                    "ORPHAN", "annotations->unit", "annotations", raw_id, unit)
        elif local.startswith("a#"):
            app = first_token(local, 2)
            if app not in model.applications:
                model.add_finding(
                    "ORPHAN", "annotations->application", "annotations",
                    raw_id, app)
        elif local.startswith("m#"):
            machine = first_token(local, 2)
            if machine not in model.machines:
                model.add_finding(
                    "ORPHAN", "annotations->machine", "annotations",
                    raw_id, machine)
        elif local.startswith("r#"):
            rid = parse_relation_id_token(local, 2)
            if rid is None:
                continue
            if model.relation_missing(rid):
                model.add_finding(
                    "ORPHAN", "annotations->relation", "annotations", raw_id,
                    "relation id %d" % rid)
        elif local.startswith("c#"):
            app = first_token(local, 2)
            if app not in model.remote_apps:
                model.add_finding(
                    "ORPHAN", "annotations->remote-app", "annotations",
                    raw_id, app)


def check_constraints(model):
    docs = model.docs.get("constraints", {})
    for raw_id, doc in docs.items():
        local = strip_prefix(raw_id, model.uuid) or raw_id
        if local.startswith("u#"):
            unit = first_token(local, 2)
            if unit not in model.units:
                model.add_finding(
                    "ORPHAN", "constraints->unit", "constraints", raw_id, unit)
        elif local.startswith("a#"):
            app = first_token(local, 2)
            if app not in model.applications:
                model.add_finding(
                    "ORPHAN", "constraints->application", "constraints",
                    raw_id, app)
        elif local.startswith("m#"):
            machine = first_token(local, 2)
            if machine not in model.machines:
                model.add_finding(
                    "ORPHAN", "constraints->machine", "constraints",
                    raw_id, machine)
        elif local == "e":
            # "e" is the model's own global key; its doc is always valid.
            pass
        elif local.startswith("e#"):
            mu = first_token(local, 2)
            if mu != model.uuid:
                model.add_finding(
                    "ORPHAN", "constraints->model", "constraints", raw_id, mu)


def check_endpointbindings(model):
    docs = model.docs.get("endpointbindings", {})
    for raw_id, doc in docs.items():
        local = strip_prefix(raw_id, model.uuid) or raw_id
        if local.startswith("a#"):
            app = first_token(local, 2)
            if app not in model.applications:
                model.add_finding(
                    "ORPHAN", "endpointbindings->application",
                    "endpointbindings", raw_id, app)


def check_units(model):
    docs = model.docs.get("units", {})
    for raw_id, doc in docs.items():
        local = strip_prefix(raw_id, model.uuid) or raw_id
        app = doc.get("application")
        if isinstance(app, str) and app not in model.applications:
            model.add_finding(
                "ORPHAN", "units->application", "units", raw_id, app)
        machine = doc.get("machineid")
        if isinstance(machine, str) and machine and \
                machine not in model.machines:
            model.add_finding(
                "ORPHAN", "units->machine", "units", raw_id, machine)


def check_relations(model):
    docs = model.docs.get("relations", {})
    for raw_id, doc in docs.items():
        local = strip_prefix(raw_id, model.uuid) or raw_id
        rid = doc.get("id")
        endpoints = doc.get("endpoints")
        if isinstance(endpoints, list):
            for endpoint in endpoints:
                if not isinstance(endpoint, dict):
                    continue
                app = endpoint.get("ApplicationName",
                                   endpoint.get("applicationname"))
                if not isinstance(app, str):
                    continue
                if app in model.applications or app in model.remote_apps:
                    continue
                if (isinstance(doc.get("life"), int) and
                        doc["life"] != 0) or \
                        (isinstance(rid, int) and
                         rid in model.pending_force_relation_cleanups) or \
                        doc.get("key") in model.pending_force_relation_keys:
                    # Relation teardown in flight; endpoint application
                    # documents may be removed with it.
                    continue
                if is_remote_app(app):
                    # The endpoint names a remote application whose
                    # remoteApplications document is gone while the
                    # relation stayed Alive (a force destroy of the
                    # saas application can leave this behind: force
                    # proceeds with the proxy removal even when a
                    # relation's removal ops fail to build).  The
                    # goal-state hook tool fails on the missing
                    # document (UniterAPI.goalStateRelations cannot
                    # resolve the app), breaking every charm hook that
                    # calls planned_units(), so the relation is dead
                    # weight: report it for teardown by id.
                    detail = (
                        "relation endpoint references remote application "
                        "'%s' with no remoteApplications document; the "
                        "goal-state hook tool fails on this and breaks "
                        "hooks" % app)
                    for cdoc in model.docs.get(
                            "applicationOfferConnections", {}).values():
                        if cdoc.get("relation-id") == rid and \
                                isinstance(
                                    cdoc.get("source-model-uuid"), str):
                            detail += (
                                "; an offer connection for consuming "
                                "model %s still records the relation" %
                                cdoc["source-model-uuid"])
                            break
                    model.add_finding(
                        "ORPHAN", "relations->remote-application",
                        "relations", raw_id, app, detail)
                else:
                    # A local-looking application name with no document:
                    # this can also be a consumed offer under its alias
                    # (modern cross-model names are not remote-*
                    # prefixed), and removing just the relation doc
                    # would strand its satellites - so it is report-only
                    # with a teardown, like the remote-named case.
                    model.add_finding(
                        "ORPHAN", "relations->application", "relations",
                        raw_id, app,
                        "relation %s endpoint" % local)


def check_applications(model):
    docs = model.docs.get("applications", {})
    for raw_id, doc in docs.items():
        # The bson field is "charmurl"; accept "charm-url" for dumps
        # from schema variants.
        charm_url = doc.get("charmurl") or doc.get("charm-url")
        if isinstance(charm_url, str) and charm_url not in model.charms:
            model.add_finding(
                "ORPHAN", "applications->charm", "applications", raw_id,
                charm_url)


def relation_endpoint_app(endpoint):
    if not isinstance(endpoint, dict):
        return None
    return endpoint.get("ApplicationName", endpoint.get("applicationname"))


def check_cross_model_offered_applications(models):
    """Verify that local application offers still have backing applications.

    An applicationOffer records the application-name it exposes; if that
    application is no longer in the applications collection the offer is
    stale.  Findings are report-only: a consuming model that merely
    consumed the offer (no relation yet) leaves no trace in this dump, so
    a locally-unreferenced offer may still be in use elsewhere, and
    removing the offer document directly would strand its permissions,
    refcount and remoteEntities export (state/applicationoffers.go).
    """
    referenced_offer_uuids = set()
    for model in models.values():
        for raw_id, doc in model.docs.get(
                "applicationOfferConnections", {}).items():
            offer_uuid = doc.get("offer-uuid")
            if isinstance(offer_uuid, str) and offer_uuid:
                referenced_offer_uuids.add(offer_uuid)
        for raw_id, doc in model.docs.get("remoteEntities", {}).items():
            local = strip_prefix(raw_id, model.uuid)
            if isinstance(local, str) and \
                    local.startswith("applicationoffer-"):
                referenced_offer_uuids.add(
                    local[len("applicationoffer-"):])

    for offer_model in models.values():
        for raw_id, doc in offer_model.docs.get(
                "applicationOffers", {}).items():
            app = doc.get("application-name")
            if not isinstance(app, str) or not app:
                continue
            if app in offer_model.applications:
                continue
            offer_uuid = doc.get("offer-uuid")
            if isinstance(offer_uuid, str) and \
                    offer_uuid in referenced_offer_uuids:
                offer_model.add_finding(
                    "ORPHAN", "applicationoffers->application-referenced",
                    "applicationOffers", raw_id, app,
                    "offer references missing backing application '%s' "
                    "and is still referenced by connections or entity "
                    "exports" % app)
            else:
                offer_model.add_finding(
                    "ORPHAN", "applicationoffers->application",
                    "applicationOffers", raw_id, app,
                    "offer has no backing application in model %s; no "
                    "local references, but cross-model consumers may "
                    "still hold the offer" % offer_model.uuid)


def check_cross_model_relations(models):
    """Validate offer-side applicationOfferConnections.

    Each applicationOfferConnection records a relation-id, offer-uuid and
    source-model-uuid (the consuming model).  Verify that the local
    relation still exists, the offer still exists, the connection's
    relation-key agrees with the relation document's key, the offered
    application is an endpoint of the relation, and a consumer-proxy
    remoteApplication for the consuming model is present.

    Connections whose relation or offer is missing are stale and removed
    by cleanup ops; relation-key mismatches are corrected by cleanup.  A
    connection whose relation is no longer Alive, or for which a
    forceDestroyRelation cleanup is queued, is mid-teardown - its removal
    accompanies the relation's (state/relation.go removeOps ->
    removeOfferConnectionsForRelationOps) - so it is skipped entirely.
    The remaining findings are report-only.
    """
    for source_model in models.values():
        model_offers = {}
        for offer_doc in source_model.docs.get(
                "applicationOffers", {}).values():
            offer_uuid = offer_doc.get("offer-uuid")
            if isinstance(offer_uuid, str) and offer_uuid:
                model_offers.setdefault(offer_uuid, []).append(offer_doc)

        relations_by_id = {}
        for relation_doc in source_model.docs.get("relations", {}).values():
            rid = relation_doc.get("id")
            if isinstance(rid, int):
                relations_by_id[rid] = relation_doc
        remote_docs = source_model.docs.get("remoteApplications", {})

        for raw_id, connection in source_model.docs.get(
                "applicationOfferConnections", {}).items():
            rid = connection.get("relation-id")
            offer_uuid = connection.get("offer-uuid")
            consumer_uuid = connection.get("source-model-uuid")
            relation_doc = relations_by_id.get(rid)
            offer_docs = model_offers.get(offer_uuid, [])

            if relation_doc is not None:
                life = relation_doc.get("life")
                if (isinstance(life, int) and life != 0) or \
                        (isinstance(rid, int) and rid in
                         source_model.pending_force_relation_cleanups):
                    # Relation teardown in flight; the connection goes
                    # with it.
                    continue

            if relation_doc is None:
                source_model.add_finding(
                    "ORPHAN", "applicationofferconnections->relation",
                    "applicationOfferConnections", raw_id,
                    "relation id %s" % rid,
                    "offer connection references missing local relation "
                    "id %s" % rid)
            if not offer_docs:
                source_model.add_finding(
                    "ORPHAN", "applicationofferconnections->offer",
                    "applicationOfferConnections", raw_id, str(offer_uuid),
                    "offer connection references missing application "
                    "offer %s" % offer_uuid)

            if relation_doc is not None:
                relation_key = relation_doc.get("key")
                connection_key = connection.get("relation-key")
                if isinstance(connection_key, str) and \
                        isinstance(relation_key, str) and \
                        connection_key != relation_key:
                    source_model.add_finding(
                        "MISMATCH", "applicationofferconnections-relation-key",
                        "applicationOfferConnections", raw_id, connection_key,
                        "connection key does not match relation key %s" %
                        relation_key)
                    source_model.docs.setdefault("_cleanup", {}).setdefault(
                        "applicationOfferConnections", {})[raw_id] = {
                        "set_relation_key": relation_key}

                endpoint_apps = {
                    relation_endpoint_app(ep)
                    for ep in relation_doc.get("endpoints", [])
                }
                for offer_doc in offer_docs:
                    offered_app = offer_doc.get("application-name")
                    if isinstance(offered_app, str) and \
                            offered_app not in endpoint_apps:
                        source_model.add_finding(
                            "ORPHAN",
                            "applicationofferconnections->offered-application",
                            "applicationOfferConnections", raw_id, offered_app,
                            "relation does not contain the application named "
                            "by offer %s" % offer_uuid)

                proxy_found = False
                for endpoint_app in endpoint_apps:
                    proxy_raw_id = source_model.uuid + ":" + str(endpoint_app)
                    proxy = remote_docs.get(proxy_raw_id)
                    if proxy is not None and \
                            proxy.get("is-consumer-proxy") is True and \
                            proxy.get("source-model-uuid") == consumer_uuid:
                        proxy_found = True
                        break
                if not proxy_found:
                    if any(is_remote_app(a) and
                           a not in source_model.remote_apps
                           for a in endpoint_apps):
                        # The proxy application document is missing
                        # entirely; the relations->remote-application
                        # check reports the relation with the goal-state
                        # breakage - don't duplicate the finding here.
                        # consumer-proxy stays meaningful only when the
                        # document exists but is the wrong proxy.
                        continue
                    source_model.add_finding(
                        "ORPHAN", "applicationofferconnections->consumer-proxy",
                        "applicationOfferConnections", raw_id,
                        str(consumer_uuid),
                        "relation has no consumer proxy remoteApplication "
                        "for consuming model %s" % consumer_uuid)


def relation_counts(relations):
    """Number of relations per endpoint application name."""
    counts = collections.Counter()
    for rdoc in relations.values():
        seen = set()
        for ep in rdoc.get("endpoints", []):
            if not isinstance(ep, dict):
                continue
            ep_app = ep.get("ApplicationName", ep.get("applicationname"))
            if isinstance(ep_app, str) and ep_app not in seen:
                seen.add(ep_app)
                counts[ep_app] += 1
    return counts


def check_remote_applications(model):
    """Verify remote application relationcount against actual relations.

    relationcount is a soft refcount maintained with $inc, but the
    unit-departure decrement path (state/relation.go removeRemoteEndpointOps
    with unitDying=true) asserts `$or[life==Alive, !consumer-proxy, count>1]`
    instead of a hard `count > 0`, so repeated force-destroys can drive it
    negative.  The relations collection is the source of truth here; a mismatch
    (including a negative count) is a bookkeeping error, fixed by setting
    the count to the actual number of relations.
    """
    docs = model.docs.get("remoteApplications", {})
    if not docs:
        return
    counts = relation_counts(model.docs.get("relations", {}))
    for raw_id, doc in docs.items():
        local = strip_prefix(raw_id, model.uuid) or raw_id
        name = doc.get("name", local)
        actual = counts.get(name, 0)
        count = doc.get("relationcount")
        if not isinstance(count, int) or count == actual:
            continue
        model.add_finding(
            "MISMATCH", "remoteapps-relationcount", "remoteApplications",
            raw_id, "relation %d != actual %d" % (count, actual))
        model.docs.setdefault("_cleanup", {}).setdefault(
            "remoteApplications", {})[raw_id] = {"set_count": actual}


def check_application_relationcount(model):
    """Verify local application relationcount against actual relations.

    DestroyApplicationOperation asserts ``len(rels) == RelationCount``
    (state/application.go), so a stale count makes ``juju
    remove-application`` fail permanently until the count is fixed.
    The relations collection is the source of truth; a mismatch is a
    bookkeeping error, fixed by setting the count to the actual number
    of relations.
    """
    docs = model.docs.get("applications", {})
    if not docs:
        return
    counts = relation_counts(model.docs.get("relations", {}))
    for raw_id, doc in docs.items():
        local = strip_prefix(raw_id, model.uuid) or raw_id
        name = doc.get("name", local)
        actual = counts.get(name, 0)
        count = doc.get("relationcount")
        if not isinstance(count, int) or count == actual:
            continue
        model.add_finding(
            "MISMATCH", "applications-relationcount", "applications",
            raw_id, "relation %d != actual %d" % (count, actual))
        model.docs.setdefault("_cleanup", {}).setdefault(
            "applications", {})[raw_id] = {"set_count": actual}


def run_checks(models):
    check_cross_model_offered_applications(models)
    check_cross_model_relations(models)
    for model in models.values():
        check_foreign_model_uuids(model)
        check_unitstates(model)
        check_relationscopes(model)
        check_relationscope_settings(model)
        check_settings(model)
        check_statuses(model)
        check_minunits(model)
        check_annotations(model)
        check_constraints(model)
        check_endpointbindings(model)
        check_units(model)
        check_relations(model)
        check_applications(model)
        check_remote_applications(model)
        check_application_relationcount(model)


def all_findings(models):
    findings = []
    for model in models.values():
        findings.extend(model.findings)
    return findings


def classify_findings(models, findings):
    """Split findings into auto-repairable and manual-repair lists.

    A finding is auto-repairable when --cleanup would emit an op that
    resolves it: its document is removed (a FULL_DOC_KINDS orphan) or
    field-corrected (a stashed update).  Everything else - CRASH
    findings, report-only cross-model kinds, and docs whose only cleanup
    path is blocked by a CRASH finding - needs manual repair.
    """
    plans = {model.uuid: repair_decisions(model)
             for model in models.values()}
    auto, manual = [], []
    for finding in findings:
        plan = None
        if isinstance(finding.doc_id, str):
            for model in models.values():
                if finding.doc_id.startswith(model.uuid + ":"):
                    plan = plans[model.uuid]
                    break
        if plan is None:
            manual.append(finding)
            continue
        _, removed, updated = plan
        key = (finding.collection, finding.doc_id)
        if key in removed or key in updated:
            auto.append(finding)
        else:
            manual.append(finding)
    return auto, manual


def revno_assert(doc, use_revno):
    revno = doc.get("txn-revno")
    if use_revno and isinstance(revno, int) and \
            not isinstance(revno, bool):
        return {"txn-revno": revno}
    return "d+"


# Findings kinds where the whole document is garbage and removal is the fix.
# Embedded references (e.g. unitstates relation-state keys) are handled with
# targeted $unset ops instead and must not be listed here.
# unitstates->unit is deliberate: if the owning unit is gone, the whole
# persisted unit state (uniter-state, relation-state, ...) is garbage.
FULL_DOC_KINDS = {
    "unitstates->unit",
    "relationscopes->relation",
    "relationscopes->unit",
    "settings->relation",
    "settings->application",
    "statuses->unit",
    "statuses->application",
    "statuses->machine",
    "statuses->relation",
    "statuses->model",
    "statuses->remote-app",
    "minunits->application",
    "annotations->unit",
    "annotations->application",
    "annotations->machine",
    "annotations->relation",
    "annotations->remote-app",
    "constraints->unit",
    "constraints->application",
    "constraints->machine",
    "constraints->model",
    "endpointbindings->application",
    "units->application",
    "units->machine",
    "applicationofferconnections->relation",
    "applicationofferconnections->offer",
    "applications->charm",
    "model-uuid",
    "foreign-id",
}


def repair_decisions(model):
    """Return (crash_docs, removed, updated) doc keys for one model.

    ``removed`` holds the docs cleanup_ops removes (ORPHAN findings in
    FULL_DOC_KINDS, excluding CRASH docs); ``updated`` holds docs with
    stashed field corrections (excluding removed docs).  This is the
    single source of truth for cleanup op generation and for the
    auto/manual split in the report.  Computed once per model and
    cached, since every report phase calls it.
    """
    if model.repair_plan is not None:
        return model.repair_plan
    crash_docs = {
        (finding.collection, finding.doc_id)
        for finding in model.findings if finding.severity == "CRASH"
    }
    removed = {
        (finding.collection, finding.doc_id)
        for finding in model.findings
        if finding.severity == "ORPHAN" and
        finding.kind in FULL_DOC_KINDS
    } - crash_docs
    updated = set()
    for coll in ("unitstates", "remoteApplications",
                 "applicationOfferConnections", "applications"):
        for raw_id in model.docs.get("_cleanup", {}).get(coll, {}):
            if (coll, raw_id) not in removed:
                updated.add((coll, raw_id))
    model.repair_plan = (crash_docs, removed, updated)
    return model.repair_plan


def cleanup_ops(models, use_revno):
    """Build mgo-run-txn JSON ops for the ORPHAN/MISMATCH findings.

    Documents carrying a CRASH finding are never removed: a scope on a live
    relation whose settings doc is missing requires manual repair, and a
    removal op would be the destructive action the CRASH category warns
    against.  Update ops from the "_cleanup" stash are skipped for
    documents that also get a removal op - a document is never repaired
    and removed in the same transaction.
    """
    ops = []
    for model in models.values():
        _, removed, _ = repair_decisions(model)
        for collection, docs in model.docs.items():
            if collection == "models" or collection == "_cleanup":
                continue
            for raw_id, doc in docs.items():
                if (collection, raw_id) not in removed:
                    continue
                ops.append({
                    "c": collection,
                    "d": raw_id,
                    "a": revno_assert(doc, use_revno),
                    "r": True,
                })
        unset_map = {}
        for raw_id, rids in model.docs.get("_cleanup", {}).get(
                "unitstates", {}).items():
            for rid in rids:
                unset_map.setdefault(raw_id, []).append(
                    "relation-state.%d" % rid)
        for raw_id, keys in unset_map.items():
            if ("unitstates", raw_id) in removed:
                # The whole document is garbage; the removal op
                # supersedes the $unset.
                continue
            doc = model.docs["unitstates"][raw_id]
            ops.append({
                "c": "unitstates",
                "d": raw_id,
                "a": revno_assert(doc, use_revno),
                "u": {"$unset": {key: 1 for key in keys}},
            })
        for raw_id, action in model.docs.get("_cleanup", {}).get(
                "remoteApplications", {}).items():
            if ("remoteApplications", raw_id) in removed:
                continue
            doc = model.docs["remoteApplications"][raw_id]
            ops.append({
                "c": "remoteApplications",
                "d": raw_id,
                "a": revno_assert(doc, use_revno),
                "u": {"$set": {"relationcount": action["set_count"]}},
            })
        for raw_id, action in model.docs.get("_cleanup", {}).get(
                "applicationOfferConnections", {}).items():
            if ("applicationOfferConnections", raw_id) in removed:
                continue
            doc = model.docs["applicationOfferConnections"][raw_id]
            ops.append({
                "c": "applicationOfferConnections",
                "d": raw_id,
                "a": revno_assert(doc, use_revno),
                "u": {"$set": {
                    "relation-key": action["set_relation_key"]}},
            })
        for raw_id, action in model.docs.get("_cleanup", {}).get(
                "applications", {}).items():
            if ("applications", raw_id) in removed:
                continue
            doc = model.docs["applications"][raw_id]
            ops.append({
                "c": "applications",
                "d": raw_id,
                "a": revno_assert(doc, use_revno),
                "u": {"$set": {"relationcount": action["set_count"]}},
            })
    return ops


# Manual findings whose fix is to tear down a whole relation: the
# relation is alive locally but an endpoint application is missing or
# its cross-model half is broken, so the relation is garbage.
MANUAL_TEARDOWN_KINDS = {
    "applicationofferconnections->consumer-proxy",
    "applicationofferconnections->offered-application",
    "relations->remote-application",
    "relations->application",
}


def relation_tag_local(rel_key):
    """Local id of a relation's remoteEntities/secretPermissions docs.

    ``names.NewRelationTag(key).String()`` turns a relation key
    "app1:ep1 app2:ep2" into "relation-app1.ep1#app2.ep2" (':' becomes
    '.', ' ' becomes '#'); the remoteEntities _id and the
    secretPermissions scope-tag both use that form, NOT the numeric id.
    """
    return "relation-" + rel_key.replace(":", ".").replace(" ", "#")


def manual_cleanup_ops(models, use_revno):
    """Build last-resort teardown ops for manual relation findings.

    For every relation carrying a MANUAL_TEARDOWN_KINDS finding,
    generate ops that remove the whole relation and its satellite
    documents - the document set of Relation.removeOps (state/
    relation.go) minus the application lifecycle conditionals: the
    relation doc, its relationscopes and settings, its status doc, its
    relationNetworks, remoteEntities and secretPermissions docs (all
    keyed by the relation tag), its offer connections, relationcount
    decrements for the local and remote endpoint applications (mirroring
    removeLocalEndpointOps and removeRemoteEndpointOps) and, for Dying
    applications, a cleanupApplication insert, plus the stale
    relation-state keys in unitstates.  Documents the
    regular --cleanup ops would remove or update are left to those
    ops.  The consuming model's half of a cross-model relation is not
    visible in this dump and must be cleaned separately.

    Returns a list of op groups, one group per relation, so the writer
    never splits a relation's teardown across two transactions.
    """
    groups = []
    for model in models.values():
        relations_by_id = {}
        for raw_id, doc in model.docs.get("relations", {}).items():
            rid = doc.get("id")
            if isinstance(rid, int):
                relations_by_id[rid] = (raw_id, doc)
        _, removed, updated = repair_decisions(model)
        touched = removed | updated

        relids = set()
        for finding in model.findings:
            if finding.kind not in MANUAL_TEARDOWN_KINDS:
                continue
            if finding.collection == "applicationOfferConnections":
                conn = model.docs.get(
                    "applicationOfferConnections", {}).get(finding.doc_id)
                if conn is not None and \
                        isinstance(conn.get("relation-id"), int):
                    relids.add(conn["relation-id"])
            elif finding.collection == "relations":
                rel_doc = model.docs.get("relations", {}).get(finding.doc_id)
                if rel_doc is not None and \
                        isinstance(rel_doc.get("id"), int):
                    relids.add(rel_doc["id"])

        for rid in sorted(relids):
            entry = relations_by_id.get(rid)
            if entry is None:
                continue
            rel_raw, rel_doc = entry
            if ("relations", rel_raw) in touched:
                # The regular cleanup is already dismantling this
                # relation; its satellites follow there.
                continue
            ops = []

            def add(collection, raw_id):
                ops.append({
                    "c": collection,
                    "d": raw_id,
                    "a": revno_assert(
                        model.docs[collection][raw_id], use_revno),
                    "r": True,
                })

            add("relations", rel_raw)
            prefix = "r#%d#" % rid
            rel_key = rel_doc.get("key")
            for collection in ("relationscopes", "settings"):
                for raw_id in model.docs.get(collection, {}):
                    local = strip_prefix(raw_id, model.uuid)
                    if not (isinstance(local, str) and
                            local.startswith(prefix)):
                        continue
                    if (collection, raw_id) not in touched:
                        add(collection, raw_id)
            for raw_id in model.docs.get("statuses", {}):
                if strip_prefix(raw_id, model.uuid) == "r#%d" % rid and \
                        ("statuses", raw_id) not in touched:
                    add("statuses", raw_id)
            for raw_id, cdoc in model.docs.get(
                    "applicationOfferConnections", {}).items():
                if cdoc.get("relation-id") == rid and \
                        ("applicationOfferConnections", raw_id) \
                        not in touched:
                    add("applicationOfferConnections", raw_id)
            tag = relation_tag_local(rel_key) \
                if isinstance(rel_key, str) else None
            if tag is not None:
                for raw_id in model.docs.get("remoteEntities", {}):
                    if strip_prefix(raw_id, model.uuid) == tag:
                        add("remoteEntities", raw_id)
                for raw_id in model.docs.get("relationNetworks", {}):
                    local = strip_prefix(raw_id, model.uuid)
                    if isinstance(local, str) and \
                            local.startswith(rel_key + ":") and \
                            ("relationNetworks", raw_id) not in touched:
                        add("relationNetworks", raw_id)
                for raw_id, pdoc in model.docs.get(
                        "secretPermissions", {}).items():
                    if pdoc.get("scope-tag") == tag and \
                            ("secretPermissions", raw_id) not in touched:
                        add("secretPermissions", raw_id)
            seen_apps = set()
            for ep in rel_doc.get("endpoints", []):
                if not isinstance(ep, dict):
                    continue
                ep_app = relation_endpoint_app(ep)
                if not isinstance(ep_app, str) or ep_app in seen_apps:
                    continue
                if ep_app in model.applications:
                    seen_apps.add(ep_app)
                    app_raw = model.uuid + ":" + ep_app
                    app_doc = model.docs.get("applications", {}).get(app_raw)
                    if app_doc is None or \
                            ("applications", app_raw) in touched:
                        continue
                    # Mirror removeLocalEndpointOps: decrement the local
                    # application's relationcount (state/relation.go) so
                    # juju remove-application's len(rels) == RelationCount
                    # assertion is not broken by the teardown.
                    ops.append({
                        "c": "applications",
                        "d": app_raw,
                        "a": revno_assert(app_doc, use_revno),
                        "u": {"$inc": {"relationcount": -1}},
                    })
                    if isinstance(app_doc.get("life"), int) and \
                            app_doc["life"] != 0:
                        # Mirror the cleanupApplication op Juju queues
                        # when a Dying application loses its last
                        # relation (state/relation.go
                        # removeLocalEndpointOps).
                        ops.append({
                            "c": "cleanups",
                            "d": "%s:%s" % (model.uuid,
                                            uuid.uuid4().hex),
                            "a": "d-",
                            "i": {
                                "kind": "application",
                                "prefix": ep_app,
                                "model-uuid": model.uuid,
                                "args": [False, False],
                            },
                        })
                elif ep_app in model.remote_apps:
                    # Mirror removeRemoteEndpointOps
                    # (state/relation.go): decrement the remote
                    # application's relationcount too, so a surviving
                    # apps's refcount agrees with the relations that
                    # remain.
                    seen_apps.add(ep_app)
                    proxy_raw = model.uuid + ":" + ep_app
                    proxy_doc = model.docs.get(
                        "remoteApplications", {}).get(proxy_raw)
                    if proxy_doc is None or \
                            ("remoteApplications", proxy_raw) in touched:
                        continue
                    ops.append({
                        "c": "remoteApplications",
                        "d": proxy_raw,
                        "a": revno_assert(proxy_doc, use_revno),
                        "u": {"$inc": {"relationcount": -1}},
                    })
            for raw_id, udoc in model.docs.get("unitstates", {}).items():
                if ("unitstates", raw_id) in touched:
                    continue
                local = strip_prefix(raw_id, model.uuid) or raw_id
                if local.startswith("u#") and local.endswith("#charm"):
                    unit = local[2:-len("#charm")]
                    if unit not in model.units:
                        # The regular ops remove the whole doc.
                        continue
                relation_state = udoc.get("relation-state")
                if isinstance(relation_state, dict) and \
                        str(rid) in relation_state:
                    ops.append({
                        "c": "unitstates",
                        "d": raw_id,
                        "a": revno_assert(udoc, use_revno),
                        "u": {"$unset": {"relation-state.%d" % rid: 1}},
                    })
            if ops:
                groups.append(ops)
    return groups


def write_cleanup_ops(ops, path, chunk_size):
    return write_op_groups([[op] for op in ops], path, chunk_size)


def write_op_groups(groups, path, chunk_size):
    """Write op groups, never splitting a group across files.

    Each group is one logical repair (a relation teardown); keeping it
    in one transaction means a chunk boundary can never commit a
    relation's satellite removals while the relation-doc op aborts in
    another file.
    """
    if not groups:
        print("no cleanup ops to write")
        return 0
    written = 0
    chunk = []
    chunk_index = 0

    def chunk_path(base, index):
        if index == 0:
            return base
        dirname, basename = os.path.split(base)
        stem, ext = os.path.splitext(basename)
        if ext:
            name = "%s-%d%s" % (stem, index, ext)
        else:
            name = "%s-%d" % (stem, index)
        return os.path.join(dirname, name) if dirname else name

    for group in groups:
        if chunk and len(chunk) + len(group) > chunk_size:
            with open(chunk_path(path, chunk_index), "w") as f:
                json.dump(chunk, f, indent=2, sort_keys=True)
                f.write("\n")
            print("wrote %s (%d ops)" % (
                chunk_path(path, chunk_index), len(chunk)))
            written += 1
            chunk = []
            chunk_index += 1
        chunk.extend(group)
    if chunk:
        with open(chunk_path(path, chunk_index), "w") as f:
            json.dump(chunk, f, indent=2, sort_keys=True)
            f.write("\n")
        print("wrote %s (%d ops)" % (chunk_path(path, chunk_index), len(chunk)))
        written += 1
    return written


def strip_uuid_prefix(raw_id, uuids):
    if not isinstance(raw_id, str):
        return raw_id
    for uid in uuids:
        prefix = uid + ":"
        if raw_id.startswith(prefix):
            return raw_id[len(prefix):]
    return raw_id


PLACEHOLDER_RE = re.compile(r"\{([a-z_]+)\}")


def render_cmd(template, ctx):
    """Fill a fix-command template; None when a placeholder is unset."""
    names = PLACEHOLDER_RE.findall(template)
    if not all(ctx.get(name) for name in names):
        return None
    return PLACEHOLDER_RE.sub(lambda m: ctx[m.group(1)], template)


def wrap_text(text):
    return textwrap.fill(text, width=90, initial_indent="  ",
                         subsequent_indent="      ",
                         break_on_hyphens=False, break_long_words=False)


def finding_context(models, finding):
    """Resolve dump values (relation id, offer name) for a fix command.

    ``juju remove-relation`` accepts a bare relation id, and offer doc
    ids are ``<uuid>:<offer-name>``, so both suggestions can be made
    concrete from the dump.
    """
    ctx = {"rid": "", "offer_name": ""}
    model = None
    if isinstance(finding.doc_id, str):
        for candidate in models.values():
            if finding.doc_id.startswith(candidate.uuid + ":"):
                model = candidate
                break
    if model is None:
        return ctx
    local = strip_prefix(finding.doc_id, model.uuid) or finding.doc_id
    if finding.collection == "relationscopes":
        match = re.match(r"^r#(\d+)#", local)
        if match:
            ctx["rid"] = match.group(1)
    elif finding.collection == "applicationOfferConnections":
        ctx["rid"] = local
        conn = model.docs.get(
            "applicationOfferConnections", {}).get(finding.doc_id)
        if conn is not None and isinstance(conn.get("relation-id"), int):
            ctx["rid"] = str(conn["relation-id"])
    elif finding.collection == "applicationOffers":
        ctx["offer_name"] = local
    elif finding.collection == "relations":
        rel_doc = model.docs.get("relations", {}).get(finding.doc_id)
        if rel_doc is not None and isinstance(rel_doc.get("id"), int):
            ctx["rid"] = str(rel_doc["id"])
    return ctx


# Suggested manual repairs for finding kinds never touched by cleanup
# ops.  "cmd" is a per-finding juju CLI command with placeholders filled
# from the dump; "fix" and "check" are printed once per kind.
MANUAL_FIXES = {
    "relations->remote-application": {
        "cmd": "juju remove-relation {rid} --force",
        "fix": "the relation's remote application document is gone while "
               "the relation stayed Alive, so the relation half is "
               "dead; tear the relation down by id - the remote "
               "application name no longer resolves",
        "check": "this state fails the goal-state hook tool (saas "
                 "application not found), breaking every charm hook "
                 "that calls planned_units(); try the command without "
                 "--force first; if the CLI cannot remove it, rerun "
                 "with --manual-cleanup to generate the exact DB "
                 "teardown ops",
    },
    "relations->application": {
        "cmd": "juju remove-relation {rid} --force",
        "fix": "the relation references an application with no document "
               "in the model, so the relation cannot work; tear it down "
               "by id",
        "check": "confirm the application is really gone and this is "
                 "not a restore or migration artifact - the missing "
                 "application may also be the cross-model (saas) side, "
                 "which additionally breaks the goal-state hook tool; "
                 "try the command without --force first; if the CLI "
                 "cannot remove it, rerun with --manual-cleanup for "
                 "the exact DB teardown ops",
    },
    "relationscopes->missing-settings": {
        "cmd": "juju remove-relation {rid} --force",
        "fix": "restore the missing settings document (from a backup, or "
               "modelled on a healthy unit's settings for the same "
               "relation), or complete the relation teardown with the "
               "command below",
        "check": "never delete the relationscope document itself - the "
                 "relationUnitsWatcher requires scope and settings docs "
                 "in pairs, and removing the scope on a live relation "
                 "corrupts it; restoring the settings doc is preferred "
                 "when the relation is still wanted",
    },
    "applicationoffers->application": {
        "cmd": "juju remove-offer {offer_name} --force",
        "fix": "the backing application is gone; withdraw the offer (the "
               "command also tears down any relations still attached to "
               "it)",
        "check": "consuming models on other controllers are invisible in "
                 "this dump - run `juju offers` to list connected "
                 "consumers and confirm they no longer need the offer "
                 "(consumers remove it with `juju remove-saas`); if the "
                 "CLI refuses because the application is already gone, "
                 "the offer needs manual DB removal along with its "
                 "permissions, refcount and remoteEntities companions "
                 "- no ops are generated for offers, because their "
                 "permissions live in the controller model, outside "
                 "this dump",
    },
    "applicationoffers->application-referenced": {
        "cmd": "juju remove-offer {offer_name} --force",
        "fix": "the backing application is gone but connections or "
               "entity exports still reference the offer; the command "
               "also destroys the offer-side relations and their "
               "connection records",
        "check": "consumers are (or were) attached - verify with `juju "
                 "offers` and the consuming models that nothing still "
                 "needs the offer before removing",
    },
    "applicationofferconnections->offered-application": {
        "cmd": "juju remove-relation {rid} --force",
        "fix": "no direct CLI repair: the offer's application-name and "
               "the relation endpoints disagree; inspect `juju offers` "
               "and `juju relations` to see which side is stale, and "
               "remove the relation if it is the wrong one",
        "check": "confirm which of the offer or the relation is wrong "
                 "before removing anything - removing the wrong one "
                 "strands the other; if the relation is the stale side "
                 "and the CLI cannot remove it, rerun with "
                 "--manual-cleanup to generate the exact DB teardown "
                 "ops for it",
    },
    "applicationofferconnections->consumer-proxy": {
        "cmd": "juju remove-relation {rid} --force",
        "fix": "the consuming model's proxy application is missing "
               "locally; if the consuming model still exists, remove the "
               "relation there so both halves tear down cleanly, "
               "otherwise force-remove the orphaned half here",
        "check": "`juju offers` lists connected consumers - prefer "
                 "removing from the consuming model when it still "
                 "exists; --force skips the clean relation teardown; if "
                 "the CLI cannot remove the relation, rerun with "
                 "--manual-cleanup to generate the exact DB teardown "
                 "ops for it",
    },
}

DEFAULT_MANUAL_FIX = {
    "cmd": None,
    "fix": "no CLI fix suggested for this finding kind; inspect the "
           "document and its references manually before repairing",
    "check": None,
}


def print_report(finding_list, uuids, models=None, with_hints=False):
    by_kind = {}
    for finding in finding_list:
        by_kind.setdefault(finding.kind, []).append(finding)
    if not by_kind:
        print("no findings")
        return
    for kind in sorted(by_kind):
        items = by_kind[kind]
        sev = items[0].severity
        print("%s %s (%d)" % (sev, kind, len(items)))
        hint = MANUAL_FIXES.get(kind, DEFAULT_MANUAL_FIX) \
            if with_hints else None
        for finding in items:
            doc_id = strip_uuid_prefix(finding.doc_id, uuids)
            body = finding.detail if finding.detail else "refs %s" % finding.ref
            print(wrap_text("%s %s" % (doc_id, body)))
            if hint is not None and hint.get("cmd") and models is not None:
                cmd = render_cmd(
                    hint["cmd"], finding_context(models, finding))
                if cmd is not None:
                    print("      -> %s" % cmd)
        if hint is not None:
            if hint.get("fix"):
                print(wrap_text("fix: " + hint["fix"]))
            if hint.get("check"):
                print(wrap_text("check first: " + hint["check"]))


def main(argv=None):
    parser = argparse.ArgumentParser(
        description="Find orphaned documents in a juju dump-db YAML dump")
    parser.add_argument("dump", nargs="?",
                        help="path to the dump (defaults to stdin)")
    parser.add_argument("--cleanup", nargs="?", const="orphan-cleanup.json",
                        metavar="FILE",
                        help="write mgo-run-txn JSON ops for ORPHAN findings")
    parser.add_argument("--manual-cleanup", nargs="?",
                        const="manual-cleanup.json", metavar="FILE",
                        help="also write mgo-run-txn ops that "
                             "tear down relations whose cross-model half "
                             "is broken (use when the juju CLI cannot "
                             "remove them)")
    parser.add_argument("--cleanup-chunk-size", type=int, default=200,
                        help="max ops per cleanup file (default 200)")
    parser.add_argument("--json", action="store_true", dest="as_json",
                        help="emit findings as JSON")
    parser.add_argument("--no-revno-assert", action="store_true",
                        help="do not assert txn-revno in cleanup ops")
    args = parser.parse_args(argv)

    loader = getattr(yaml, "CSafeLoader", yaml.SafeLoader)
    try:
        if args.dump:
            with open(args.dump) as f:
                data = yaml.load(f, Loader=loader)
        else:
            data = yaml.load(sys.stdin, Loader=loader)
    except IOError as e:
        parser.error("cannot read dump: %s" % e)
        return 2
    except yaml.YAMLError as e:
        parser.error("cannot parse dump: %s" % e)
        return 2
    if not isinstance(data, dict):
        parser.error("dump does not contain a collection map")
        return 2

    stray = []
    models = build_models(data, stray)
    if not models:
        parser.error("dump contains no models collection")
        return 2
    run_checks(models)

    findings = stray + all_findings(models)
    orphans = [f for f in findings if f.severity == "ORPHAN"]
    mismatches = [f for f in findings if f.severity == "MISMATCH"]
    crashes = [f for f in findings if f.severity == "CRASH"]
    auto, manual = classify_findings(models, findings)

    if args.as_json:
        manual_ids = set(id(f) for f in manual)
        out = {
            "models": sorted(models.keys()),
            "orphan_count": len(orphans),
            "mismatch_count": len(mismatches),
            "crash_count": len(crashes),
            "auto_repair_count": len(auto),
            "manual_repair_count": len(manual),
            "findings": [
                dict(f.as_dict(),
                     repair="manual" if id(f) in manual_ids else "auto")
                for f in findings
            ],
        }
        if args.cleanup:
            ops = cleanup_ops(models, not args.no_revno_assert)
            out["cleanup_ops"] = ops
        if args.manual_cleanup:
            groups = manual_cleanup_ops(models, not args.no_revno_assert)
            out["manual_cleanup_ops"] = [
                op for group in groups for op in group]
        print(json.dumps(out, indent=2, sort_keys=True))
    else:
        uuids = set(models.keys())
        print("== AUTO-REPAIRABLE (%d) ==" % len(auto))
        print("  run with --cleanup to emit mgo-run-txn ops that remove "
              "or correct these findings through Juju's transaction "
              "machinery")
        print_report(auto, uuids)
        print("")
        print("== MANUAL REPAIR (%d) ==" % len(manual))
        print("  each item below has a suggested fix and "
              "checks to make first before running the fix")
        print_report(manual, uuids, models=models, with_hints=True)
        if any(f.kind in MANUAL_TEARDOWN_KINDS for f in manual):
            print("")
            print("  run with --manual-cleanup to also write "
                  "DB teardown ops for the relation finding(s) above")
        if args.cleanup:
            ops = cleanup_ops(models, not args.no_revno_assert)
            print("== cleanup (%d ops) ==" % len(ops))
            write_cleanup_ops(ops, args.cleanup, args.cleanup_chunk_size)
            print("The ops are executed through Juju's txn machinery:")
            print("  go run ./scripts/mgo-run-txn %s" % args.cleanup)
            print("Against juju-db 4.4.30+ also pass the controller TLS "
                  "material, e.g.:")
            print("  go run ./scripts/mgo-run-txn --ca-file "
                  "/var/snap/juju-db/common/ca.crt --cert-file "
                  "/var/snap/juju-db/common/server.pem %s" % args.cleanup)
        if args.manual_cleanup:
            groups = manual_cleanup_ops(models, not args.no_revno_assert)
            if not groups:
                print("no manual cleanup ops to write")
            else:
                nops = sum(len(g) for g in groups)
                print("")
                print("== manual cleanup (%d ops, %d relation(s)) ==" % (
                    nops, len(groups)))
                print(wrap_text(
                    "for the manual repairs: try the suggested juju "
                    "commands first, and if possible, also clean up the "
                    "consuming model's half separately; if that fails, "
                    "the ops in the following file tear down relations "
                    "at the DB level"))
                print(wrap_text(
                    "each relation's ops are kept in one transaction; a "
                    "live unit agent may re-write its unitstates entry "
                    "until it reconciles the removed relation"))
                write_op_groups(
                    groups, args.manual_cleanup, args.cleanup_chunk_size)
                print("The ops are executed through Juju's txn machinery:")
                print("  go run ./scripts/mgo-run-txn %s" %
                      args.manual_cleanup)
                print("Against juju-db 4.4.30+ also pass the controller "
                      "TLS material, e.g.:")
                print("  go run ./scripts/mgo-run-txn --ca-file "
                      "/var/snap/juju-db/common/ca.crt --cert-file "
                      "/var/snap/juju-db/common/server.pem %s" %
                      args.manual_cleanup)

    if orphans or mismatches or crashes:
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
