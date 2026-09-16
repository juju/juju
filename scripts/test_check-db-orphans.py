#!/usr/bin/env python3
#
# Copyright 2026 Canonical Ltd.
# Licensed under the AGPLv3, see LICENCE file for details.
#
# Unit tests for scripts/check-db-orphans.py, run directly:
#   python3 scripts/test_check-db-orphans.py
#
# The coverage is focused on the op-generation invariants whose failure
# mode is production data loss: repair decisions, the exact content of
# cleanup and manual teardown ops, and the write_op_groups chunking boundary.

import importlib.util
import json
import os
import sys
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
SPEC = importlib.util.spec_from_file_location(
    "check_db_orphans", os.path.join(HERE, "check-db-orphans.py"))
check = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(check)

UUID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
CONSUMER_UUID = "11111111-2222-3333-4444-555555555555"


def model_doc(local, **fields):
    doc = {"_id": "%s:%s" % (UUID, local), "model-uuid": UUID}
    doc.update(fields)
    return doc


def make_dump(*collections):
    data = {"models": [{"_id": UUID}]}
    for collection, docs in collections:
        data[collection] = docs
    return data


def find_op(ops, collection, doc_id=None):
    for op in ops:
        if op["c"] == collection and (doc_id is None or op["d"] == doc_id):
            return op
    return None


class TestRevnoAssert(unittest.TestCase):

    def test_int_revno_used(self):
        self.assertEqual(check.revno_assert({"txn-revno": 7}, True),
                         {"txn-revno": 7})

    def test_bool_revno_ignored(self):
        # bool is a subclass of int; True must not be emitted as a
        # txn-revno assert.
        self.assertEqual(check.revno_assert({"txn-revno": True}, True), "d+")
        self.assertEqual(check.revno_assert({"txn-revno": False}, True), "d+")

    def test_revno_disabled(self):
        self.assertEqual(check.revno_assert({"txn-revno": 7}, False), "d+")

    def test_missing_revno(self):
        self.assertEqual(check.revno_assert({}, True), "d+")


class TestBuildModels(unittest.TestCase):

    def test_foreign_id(self):
        stray = []
        models = check.build_models(make_dump(
            ("units", [{"_id": "stray-id"}])), stray)
        self.assertEqual(len(stray), 1)
        self.assertEqual(stray[0].kind, "foreign-id")
        self.assertEqual(stray[0].ref, "<unknown-model>")
        self.assertEqual(list(models.keys()), [UUID])
        self.assertNotIn("stray-id", models[UUID].docs.get("units", {}))

    def test_model_uuid_attribution(self):
        raw = "0123456789abcdef0123456789abcdef"
        stray = []
        models = check.build_models(make_dump(
            ("statuseshistory", [{"_id": raw, "model-uuid": UUID}])), stray)
        self.assertEqual(stray, [])
        self.assertIn(raw, models[UUID].docs["statuseshistory"])

    def test_missing_id_with_model_uuid(self):
        # Regression: a doc carrying model-uuid but no _id used to raise
        # AttributeError in strip_prefix.
        stray = []
        models = check.build_models(make_dump(
            ("units", [{"model-uuid": UUID}])), stray)
        self.assertEqual(len(stray), 1)
        self.assertEqual(stray[0].kind, "foreign-id")
        self.assertEqual(stray[0].ref, "<no-string-id>")
        self.assertIn(UUID, models)

    def test_non_string_id(self):
        stray = []
        models = check.build_models(make_dump(
            ("units", [{"_id": 12345, "model-uuid": UUID}])), stray)
        self.assertEqual(len(stray), 1)
        self.assertEqual(stray[0].kind, "foreign-id")


class TestRepairDecisions(unittest.TestCase):

    def test_crash_doc_excluded_from_removal(self):
        models = check.build_models(make_dump(
            ("relations", [model_doc("app1:ep1 app2:ep2", id=1, life=0)]),
            ("relationscopes", [model_doc("r#1#app1/0")]),
            ("units", [model_doc("app1/0", application="app1",
                                 machineid="0")]),
            ("machines", [model_doc("0")]),
            ("applications", [model_doc("app1"), model_doc("app2")]),
        ), [])
        model = models[UUID]
        check.run_checks(models)
        crash, removed, updated = check.repair_decisions(model)
        scope_raw = "%s:r#1#app1/0" % UUID
        self.assertIn(("relationscopes", scope_raw), crash)
        self.assertNotIn(("relationscopes", scope_raw), removed)
        self.assertNotIn(("relationscopes", scope_raw), updated)

    def test_removal_wins_over_update(self):
        models = check.build_models(make_dump(
            ("unitstates", [model_doc(
                "u#app1/0#charm", **{"relation-state": {"5": {}}})]),
        ), [])
        model = models[UUID]
        check.check_unitstates(model)
        crash, removed, updated = check.repair_decisions(model)
        raw = "%s:u#app1/0#charm" % UUID
        self.assertIn(("unitstates", raw), removed)
        self.assertNotIn(("unitstates", raw), updated)
        ops = check.cleanup_ops(models, False)
        self.assertEqual(len(ops), 1)
        self.assertEqual(ops[0], {
            "c": "unitstates", "d": raw, "a": "d+", "r": True})

    def test_classify_crash_manual_and_orphan_auto(self):
        models = check.build_models(make_dump(
            ("relations", [model_doc("app1:ep1 app2:ep2", id=1, life=0)]),
            ("relationscopes", [model_doc("r#1#app1/0")]),
            ("settings", [model_doc("r#9#provider#app1/0")]),
            ("units", [model_doc("app1/0", application="app1",
                                 machineid="0")]),
            ("machines", [model_doc("0")]),
            ("applications", [model_doc("app1"), model_doc("app2")]),
        ), [])
        check.run_checks(models)
        findings = check.all_findings(models)
        auto, manual = check.classify_findings(models, findings)
        manual_kinds = {f.kind for f in manual}
        auto_kinds = {f.kind for f in auto}
        self.assertIn("relationscopes->missing-settings", manual_kinds)
        self.assertIn("settings->relation", auto_kinds)


class TestCleanupOps(unittest.TestCase):

    def test_settings_relation_removal_op(self):
        raw = "%s:r#9#provider#app1/0" % UUID
        models = check.build_models(make_dump(
            ("settings", [{"_id": raw, "model-uuid": UUID,
                           "txn-revno": 42}])), [])
        model = models[UUID]
        check.check_settings(model)
        ops = check.cleanup_ops(models, True)
        op = find_op(ops, "settings", raw)
        self.assertIsNotNone(op)
        self.assertEqual(op, {
            "c": "settings", "d": raw, "a": {"txn-revno": 42}, "r": True})

    def test_relationcount_update_ops(self):
        models = check.build_models(make_dump(
            ("relations", [model_doc(
                "app1:ep1 remote-1:ep2", id=1, life=0,
                endpoints=[{"ApplicationName": "app1"},
                           {"ApplicationName": "remote-1"}])]),
            ("applications", [model_doc("app1", relationcount=5)]),
            ("remoteApplications", [model_doc("remote-1", name="remote-1",
                                              relationcount=-2)]),
        ), [])
        check.run_checks(models)
        model = models[UUID]
        kinds = {f.kind for f in model.findings}
        self.assertIn("applications-relationcount", kinds)
        self.assertIn("remoteapps-relationcount", kinds)
        ops = check.cleanup_ops(models, False)
        app_op = find_op(ops, "applications", "%s:app1" % UUID)
        rem_op = find_op(ops, "remoteApplications", "%s:remote-1" % UUID)
        self.assertEqual(app_op["u"], {"$set": {"relationcount": 1}})
        self.assertEqual(rem_op["u"], {"$set": {"relationcount": 1}})


class TestManualCleanupOps(unittest.TestCase):

    def setUp(self):
        # Relation 1 has a local Dying application and a consumer-proxy
        # whose source-model-uuid does not match the connection, which
        # produces the applicationofferconnections->consumer-proxy
        # finding that triggers a manual teardown.
        self.dump = make_dump(
            ("relations", [model_doc(
                "app1:ep1 proxy1:ep2", id=1, life=0,
                key="app1:ep1 proxy1:ep2",
                endpoints=[{"ApplicationName": "app1"},
                           {"ApplicationName": "proxy1"}])]),
            ("applications", [model_doc("app1", life=1)]),
            ("remoteApplications", [model_doc(
                "proxy1", name="proxy1",
                **{"is-consumer-proxy": False,
                   "source-model-uuid": CONSUMER_UUID, "relationcount": 1})]),
            ("applicationOfferConnections", [model_doc(
                "conn1", **{"relation-id": 1, "offer-uuid": "o1",
                            "source-model-uuid": CONSUMER_UUID,
                            "relation-key": "app1:ep1 proxy1:ep2"})]),
        )

    def test_teardown_group_remote_decrement(self):
        models = check.build_models(self.dump, [])
        check.run_checks(models)
        groups = check.manual_cleanup_ops(models, False)
        self.assertEqual(len(groups), 1)
        ops = groups[0]
        rem_op = find_op(ops, "remoteApplications", "%s:proxy1" % UUID)
        self.assertIsNotNone(rem_op)
        self.assertEqual(rem_op["u"], {"$inc": {"relationcount": -1}})
        app_op = find_op(ops, "applications", "%s:app1" % UUID)
        self.assertEqual(app_op["u"], {"$inc": {"relationcount": -1}})

    def test_teardown_cleanup_insert_assert_and_args(self):
        models = check.build_models(self.dump, [])
        check.run_checks(models)
        groups = check.manual_cleanup_ops(models, False)
        ops = groups[0]
        cleanup_op = find_op(ops, "cleanups")
        self.assertIsNotNone(cleanup_op)
        self.assertEqual(cleanup_op["a"], "d-")
        self.assertEqual(cleanup_op["i"]["kind"], "application")
        self.assertEqual(cleanup_op["i"]["prefix"], "app1")
        self.assertEqual(cleanup_op["i"]["args"], [False, False])

    def test_teardown_relation_removed(self):
        models = check.build_models(self.dump, [])
        check.run_checks(models)
        groups = check.manual_cleanup_ops(models, False)
        ops = groups[0]
        rel_op = find_op(ops, "relations", "%s:app1:ep1 proxy1:ep2" % UUID)
        self.assertIsNotNone(rel_op)
        self.assertTrue(rel_op["r"])


class TestWriteOpGroups(unittest.TestCase):

    def test_group_never_split_across_chunks(self):
        tmpdir = tempfile.mkdtemp()
        path = os.path.join(tmpdir, "ops.json")
        groups = [
            [{"c": "relations", "d": "r1"},
             {"c": "settings", "d": "s1"},
             {"c": "settings", "d": "s2"}],
            [{"c": "unitstates", "d": "u1"}],
        ]
        check.write_op_groups(groups, path, 2)
        with open(path) as f:
            first = json.load(f)
        with open(os.path.join(tmpdir, "ops-1.json")) as f:
            second = json.load(f)
        self.assertEqual(len(first), 3)
        self.assertEqual(len(second), 1)

    def test_chunk_path_with_dot_in_dirname(self):
        # Regression: chunking used to split on the whole path, so
        # <dir with a dot>/ops produced <dir>-1/ops whose parent did
        # not exist and FileNotFoundError after the first chunk.
        tmpdir = tempfile.mkdtemp()
        path = os.path.join(tmpdir, "x.y", "ops")
        os.makedirs(os.path.dirname(path))
        groups = [[{"c": "a", "d": "1"}], [{"c": "a", "d": "2"}]]
        check.write_op_groups(groups, path, 1)
        with open(os.path.join(tmpdir, "x.y", "ops")) as f:
            self.assertEqual(len(json.load(f)), 1)
        with open(os.path.join(tmpdir, "x.y", "ops-1")) as f:
            self.assertEqual(len(json.load(f)), 1)


class TestExitCodes(unittest.TestCase):

    def _write(self, tmpdir, data):
        path = os.path.join(tmpdir, "dump.json")
        with open(path, "w") as f:
            json.dump(data, f)
        return path

    def test_clean_dump_exit_zero(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            path = self._write(tmpdir, make_dump())
            self.assertEqual(check.main([path]), 0)

    def test_orphan_dump_exit_one(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            path = self._write(tmpdir, make_dump(
                ("settings", [model_doc("r#9#provider#app1/0")])))
            self.assertEqual(check.main([path]), 1)

    def test_missing_file_exit_two(self):
        with self.assertRaises(SystemExit) as cm:
            check.main(["/nonexistent/does-not-exist.yaml"])
        self.assertEqual(cm.exception.code, 2)

    def test_cleanup_writes_ops(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            dump_path = self._write(tmpdir, make_dump(
                ("settings", [model_doc("r#9#provider#app1/0")])))
            out_path = os.path.join(tmpdir, "cleanup.json")
            rc = check.main([dump_path, "--cleanup", out_path])
            self.assertEqual(rc, 1)
            with open(out_path) as f:
                ops = json.load(f)
            self.assertEqual(len(ops), 1)
            self.assertTrue(ops[0]["r"])


if __name__ == "__main__":
    unittest.main()
