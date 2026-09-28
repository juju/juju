"""Unit tests for LLM CI suite selection (stdlib unittest, no pytest).

Run:  python3 -m unittest discover -s tests/tools -v
"""

import io
import json
import os
import sys
import time
import tempfile
import unittest
from unittest import mock

HERE = os.path.dirname(os.path.abspath(__file__))
sys.path.insert(0, HERE)

import select_suites  # noqa: E402
import suite_manifest  # noqa: E402

MANIFEST = os.path.join(HERE, os.pardir, "suites.manifest.yaml")
SUITES_DIR = os.path.join(HERE, os.pardir, "suites")


def load():
    return suite_manifest.load_manifest(MANIFEST)


class TestManifest(unittest.TestCase):
    def test_loads_and_covers_all_suite_dirs(self):
        suites = load()
        suite_manifest.check_matches_suites_dir(suites, SUITES_DIR)
        # 48 today; grows with the suite set, never shrinks silently.
        self.assertGreaterEqual(len(suites), 48)

    def test_secrets_k8s_shape(self):
        s = load()["secrets_k8s"]
        self.assertTrue(s["gh_eligible"])
        self.assertEqual(s["provider"], "k8s")
        self.assertIn("internal/worker/uniter/secrets/**", s["paths"])

    def test_unknown_provider_rejected(self):
        with self.assertRaises(suite_manifest.ManifestError):
            suite_manifest.load_manifest(_bad_manifest(
                "provider: wat\ngh_eligible: true\n"
                "description: x\nkeywords: []\n"))

    def test_path_globs_match_something(self):
        import glob
        os.chdir(os.path.join(HERE, os.pardir, os.pardir))
        for name, s in load().items():
            for p in s["paths"]:
                self.assertTrue(
                    glob.glob(p), f"{name}: path glob matches nothing: {p}")


class TestGlobMatch(unittest.TestCase):
    def test_doublestar(self):
        gm = suite_manifest._glob_match
        self.assertTrue(gm("internal/worker/uniter/**",
                           "internal/worker/uniter/foo/bar.go"))
        self.assertFalse(gm("internal/worker/uniter/**",
                            "internal/worker/uniterx/a.go"))
        self.assertTrue(gm("cmd/juju/**/deploy.go", "cmd/juju/x/deploy.go"))
        self.assertTrue(gm("domain/secret/**", "domain/secret/state/x.go"))

    def test_matching_paths(self):
        suites = {"s": {"paths": ["domain/secret/**"],
                        "gh_eligible": True}}
        got = suite_manifest.matching_paths(
            suites, ["domain/secret/service.go", "docs/foo.md"])
        self.assertEqual(got, {"s"})


class TestChurn(unittest.TestCase):
    SAMPLE = (
        "aaaa" + "0" * 36 + "\ntests/suites/secrets_k8s/task.sh\n"
        "internal/worker/uniter/secrets/store.go\n"
        "core/secrets/secret.go\n"
        + "bbbb" + "0" * 36 + "\n"
        + "core/secrets/secret.go\n"
        "domain/secret/state/secrets.go\n")

    def test_parse_churn(self):
        ranked = suite_manifest.parse_churn(self.SAMPLE)
        self.assertEqual(ranked[0][0], "core/secrets/secret.go")
        self.assertEqual(ranked[0][1], 2)

    def test_churn_excludes_suite_itself(self):
        import suite_manifest as sm

        class FakeRun:
            returncode = 0
            stdout = TestChurn.SAMPLE

        with mock.patch.object(sm.subprocess, "run",
                               return_value=FakeRun()):
            top = sm.churn_top_files("/repo", "secrets_k8s")
        paths = [p for p, _ in top]
        self.assertNotIn("tests/suites/secrets_k8s/task.sh", paths)


class TestEvidence(unittest.TestCase):
    DIFF = (
        "diff --git a/core/secrets/secret.go b/core/secrets/secret.go\n"
        "--- a/core/secrets/secret.go\n"
        "+++ b/core/secrets/secret.go\n"
        "@@ -1 +1 @@\n-old\n+new\n"
        "diff --git a/docs/readme.md b/docs/readme.md\n"
        "--- a/docs/readme.md\n"
        "+++ b/docs/readme.md\n"
        "@@ -1 +1 @@\n-doc\n+docs\n"
        "diff --git a/_deps/x.go b/_deps/x.go\n"
        "--- /dev/null\n+++ b/_deps/x.go\nx\n")

    def test_drops_vendor_keeps_go(self):
        ev = select_suites.build_evidence(
            self.DIFF, "t", "b", ["core/secrets/secret.go"])
        self.assertIn("core/secrets/secret.go", ev["changed_files"])
        self.assertNotIn("_deps/x.go", ev["changed_files"])
        self.assertNotIn("_deps/x.go", ev["hunks"])

    def test_priority_order_within_budget(self):
        ev = select_suites.build_evidence(
            self.DIFF, "t", "b", [])
        self.assertLess(
            ev["hunks"].index("core/secrets/secret.go"),
            ev["hunks"].index("docs/readme.md"))

    def test_budget_omits_low_priority(self):
        ev = select_suites.build_evidence(
            self.DIFF, "t", "b", [], budget=30)  # tiny budget
        self.assertTrue(ev["hunks_omited_for_size"])

    def test_state_respects_api_char_limit(self):
        # 200 files x 80-char hunks would blow the 8000-char state cap.
        big = "".join(
            f"diff --git a/p{i}/x.go b/p{i}/x.go\n"
            f"--- a/p{i}/x.go\n+++ b/p{i}/x.go\n@@ -1 +1 @@\n-{i}\n+{i}\n"
            for i in range(200))
        ev = select_suites.build_evidence(big, "t" * 300, "b" * 3000,
                                          [f"p{i}/x.go" for i in range(200)])
        self.assertLessEqual(len(json.dumps(ev)), select_suites.STATE_CHAR_LIMIT)
        # shrinking keeps the highest-priority hunks and lists the rest
        self.assertTrue(ev["hunks_omited_for_size"] or ev["changed_files"])


class TestJevRequestResponse(unittest.TestCase):
    def test_request_one_noul_per_suite(self):
        suites = {"deploy": {"description": "d\n", "keywords": ["k"]},
                  "cmr": {"description": "c\n", "keywords": []}}
        req = select_suites.build_jev_request({"hunks": "x"}, suites)
        self.assertEqual(req["model"], "jev-1.13.0")
        self.assertEqual(set(req["questions"]), {"deploy", "cmr"})
        self.assertEqual(req["questions"]["deploy"]["type"], "noul")

    def test_response_whitelist(self):
        payload = {"answers": {
            "deploy": {"noul": 0.8, "confidence": 0.9},
            "rm -rf /": {"noul": 0.99, "confidence": 0.99},  # injected key
            "cmr": {"noul": 0.2},
        }}
        got = select_suites.parse_jev_response(payload, {"deploy", "cmr"})
        self.assertEqual(set(got), {"deploy", "cmr"})
        self.assertEqual(got["deploy"], (0.8, 0.9))
        # no confidence in payload -> noul doubles as confidence
        self.assertEqual(got["cmr"], (0.2, 0.2))


class TestAskJevRetry(unittest.TestCase):
    def test_degrades_on_timeout(self):
        with mock.patch.object(select_suites.urllib.request, "urlopen",
                               side_effect=TimeoutError("boom")):
            with mock.patch.object(select_suites.time, "sleep"):
                answers, err = select_suites.ask_jev({}, {"s": {}}, "k")
        self.assertEqual(answers, {})
        self.assertIn("TimeoutError", err)

    def test_batches_more_than_eight_suites(self):
        names = [f"s{i}" for i in range(9)]
        suites = {n: {"description": "d", "keywords": []} for n in names}

        class Resp:
            def __init__(self, payload):
                self._p = payload

            def read(self):
                return io.BytesIO(json.dumps(self._p).encode()).read()

            def __enter__(self):
                return self

            def __exit__(self, *a):
                return False

        calls = []

        def fake(req, timeout=None):
            body = json.loads(req.data.decode())
            calls.append(body)
            qs = list(body["questions"])
            return Resp({"answers": {q: {"noul": 0.5} for q in qs}})

        with mock.patch.object(select_suites.urllib.request, "urlopen",
                               side_effect=fake):
            answers, err = select_suites.ask_jev({}, suites, "k")
        self.assertIsNone(err)
        self.assertEqual(len(calls), 2)  # 9 suites -> 8 + 1 per API limit
        self.assertLessEqual(len(calls[0]["questions"]), 8)
        self.assertLessEqual(len(calls[1]["questions"]), 8)
        self.assertEqual(set(answers), set(names))
        self.assertEqual(answers["s8"], (0.5, 0.5))

    def test_batch_error_keeps_other_batches(self):
        suites = {f"s{i}": {"description": "d", "keywords": []}
                  for i in range(9)}

        class Resp:
            def __init__(self, payload):
                self._p = payload

            def read(self):
                return io.BytesIO(json.dumps(self._p).encode()).read()

            def __enter__(self):
                return self

            def __exit__(self, *a):
                return False

        def fake(req, timeout=None):
            body = json.loads(req.data.decode())
            if "s0" in body["questions"]:
                raise TimeoutError("boom")
            return Resp({"answers": {q: {"noul": 0.9}
                                     for q in body["questions"]}})

        with mock.patch.object(select_suites.urllib.request, "urlopen",
                               side_effect=fake):
            with mock.patch.object(select_suites.time, "sleep"):
                answers, err = select_suites.ask_jev({}, suites, "k")
        self.assertEqual(answers, {"s8": (0.9, 0.9)})
        self.assertIn("TimeoutError", err)

    def test_stalled_network_respects_total_budget(self):
        # Server accepts the connection but never answers (e.g. corporate
        # proxy, blackholed route): each urlopen burns its whole timeout,
        # and the fan-out must end within the wall-clock budget instead of
        # retries x batches x timeout.
        suites = {f"s{i}": {"description": "d", "keywords": []}
                  for i in range(16)}  # 2 batches

        sleeps = {"n": 0}

        def fake_sleep(s):
            sleeps["n"] += 1
            t0[0] += s  # backoff sleeps advance the fake clock too

        t0 = [1000.0]
        used_timeouts = []

        def fake_monotonic():
            return t0[0]

        def fake(req, timeout=None):
            # a stalled socket consumes exactly the timeout it was given
            self.assertLessEqual(timeout, select_suites.HTTP_TIMEOUT)
            used_timeouts.append(timeout)
            t0[0] += timeout
            raise TimeoutError("stalled")

        with mock.patch.object(select_suites.urllib.request, "urlopen",
                               side_effect=fake), \
             mock.patch.object(select_suites.time, "sleep", fake_sleep), \
             mock.patch.object(select_suites.time, "monotonic",
                               fake_monotonic):
            answers, err = select_suites.ask_jev({}, suites, "k")
        self.assertEqual(answers, {})
        self.assertIn("time budget", err)
        # the per-attempt timeout shrinks as the budget runs out
        self.assertLess(used_timeouts[-1], select_suites.HTTP_TIMEOUT)
        # and the fan-out stopped early instead of retrying forever
        self.assertLessEqual(len(used_timeouts), 8)

    def test_http_error_includes_body_snippet(self):
        import urllib.error
        err403 = urllib.error.HTTPError(
            "u", 403, "forbidden", {},
            io.BytesIO(b'{"error":{"message":"Key limit exceeded"}}'))  # type: ignore[arg-type]

        def fake(*a, **k):
            raise err403

        with mock.patch.object(select_suites.urllib.request, "urlopen",
                               side_effect=fake):
            answers, err = select_suites.ask_jev({}, {"s": {}}, "k")
        err403.close()
        self.assertEqual(answers, {})
        self.assertIn("403", err)
        self.assertIn("Key limit exceeded", err)

    def test_retries_429_then_succeeds(self):
        import urllib.error
        ok = io.BytesIO(json.dumps(
            {"answers": {"s": {"noul": 0.7, "confidence": 0.8}}}).encode())

        class Resp:
            def read(self):
                return ok.read()

            def __enter__(self):
                return self

            def __exit__(self, *a):
                return False

        err429 = urllib.error.HTTPError(
            "u", 429, "too many", {"retry-after": "0"}, None)  # type: ignore[arg-type]
        calls = {"n": 0}

        def fake(*a, **k):
            calls["n"] += 1
            if calls["n"] == 1:
                raise err429
            return Resp()

        with mock.patch.object(select_suites.urllib.request, "urlopen",
                               side_effect=fake):
            with mock.patch.object(select_suites.time, "sleep"):
                answers, err = select_suites.ask_jev(
                    {}, {"s": {"description": "d", "keywords": []}}, "k")
        err429.close()
        self.assertEqual(calls["n"], 2)
        self.assertIsNone(err)
        self.assertEqual(answers["s"], (0.7, 0.8))


class TestPolicy(unittest.TestCase):
    def _suites(self):
        return {
            "deploy": {"gh_eligible": True, "paths": ["cmd/juju/**"],
                       "description": "d", "keywords": []},
            "secrets_k8s": {"gh_eligible": True, "paths": [],
                            "description": "d", "keywords": []},
            "model": {"gh_eligible": True, "paths": [],
                      "description": "d", "keywords": []},
            "storage": {"gh_eligible": True, "paths": [],
                        "description": "d", "keywords": []},
            "firewall": {"gh_eligible": False, "paths": [],
                         "description": "d", "keywords": []},
        }

    def test_precise_glob_beats_hub(self):
        # A glob used by one suite (cmd/juju/** here) ranks above
        # LLM-flagged suites with no static match.
        suites = {
            "deploy": {"gh_eligible": True, "paths": ["cmd/juju/**"],
                        "description": "d", "keywords": []},
            "model": {"gh_eligible": True, "paths": [],
                      "description": "d", "keywords": []},
        }
        answers = {"model": (0.9, 0.9)}  # LLM likes model, no static match
        r = select_suites.merge_policy(
            {"max_runs": 2}, suites, ["cmd/juju/deploy.go"], answers)
        self.assertEqual(r["runs"][0], "deploy")  # precise glob wins
        self.assertIn("model", r["runs"])         # LLM score lifts it in

    def test_caps_at_max_runs_and_trims(self):
        suites = {
            f"s{i}": {"gh_eligible": True,
                      "paths": [f"pkg{i}/**"],
                      "description": "d", "keywords": []}
            for i in range(8)
        }
        files = [f"pkg{i}/x.go" for i in range(8)]
        r = select_suites.merge_policy(
            {"max_runs": 3}, suites, files, {})
        self.assertEqual(len(r["runs"]), 3)
        self.assertEqual(len(r["trimmed"]), 5)
        # all 8 appear in the ranked table for shadow transparency
        self.assertEqual(len(r["ranked"]), 8)

    def test_already_required_excluded(self):
        suites = {
            "smoke": {"gh_eligible": True, "already_required": True,
                      "paths": ["cmd/juju/**"],
                      "description": "d", "keywords": []},
            "deploy": {"gh_eligible": True, "paths": ["cmd/juju/**"],
                       "description": "d", "keywords": []},
        }
        answers = {"smoke": (0.99, 0.99), "deploy": (0.8, 0.9)}
        r = select_suites.merge_policy(
            {"max_runs": 6, "threshold": 0.5}, suites,
            ["cmd/juju/deploy.go"], answers)
        self.assertNotIn("smoke", r["runs"])
        self.assertNotIn("smoke", r["trimmed"])
        self.assertEqual(r["runs"], ["deploy"])
        self.assertEqual(r["skipped_required"], ["smoke"])

    def test_degraded_mode_ranks_static_only(self):
        # No API key: answers empty, ranking is pure static specificity.
        # domain/secret/** is used by one suite (spec 1.0); the hub glob
        # is shared by three (spec 0.33), so precise wins.
        suites = {
            "precise": {"gh_eligible": True, "paths": ["domain/secret/**"],
                        "description": "d", "keywords": []},
            "hub_a": {"gh_eligible": True,
                      "paths": ["internal/worker/uniter/**"],
                      "description": "d", "keywords": []},
            "hub_b": {"gh_eligible": True,
                      "paths": ["internal/worker/uniter/**"],
                      "description": "d", "keywords": []},
            "hub_c": {"gh_eligible": True,
                      "paths": ["internal/worker/uniter/**"],
                      "description": "d", "keywords": []},
        }
        r = select_suites.merge_policy(
            {"max_runs": 2}, suites,
            ["domain/secret/state.go", "internal/worker/uniter/foo.go"], {})
        self.assertEqual(r["runs"][0], "precise")  # spec 1.0 > 0.33
        self.assertIn("hub_a", r["runs"])          # top hub by tie-break

    def test_docs_only_diff_zero_runs(self):
        ev = select_suites.build_evidence(
            "--- a/docs/x.md\n+++ b/docs/x.md\n+hi\n", "docs", "",
            ["docs/x.md"])
        self.assertIn("docs/x.md", ev["changed_files"])
        suites = {"deploy": {"gh_eligible": True, "paths": ["cmd/juju/**"],
                              "description": "d", "keywords": []}}
        r = select_suites.merge_policy({"max_runs": 6}, suites, ["docs/x.md"], {})
        self.assertEqual(r["runs"], [])


class TestComment(unittest.TestCase):
    def test_renders_sections(self):
        result = {"runs": ["deploy", "model"], "ranked": [
            {"name": "deploy", "score": 1.5, "source": "static"},
            {"name": "model", "score": 0.9, "source": "static+llm"}],
            "trimmed": ["storage"],
            "skipped_required": ["smoke"],
            "all_scores": {"deploy": {"noul": 0.9, "confidence": 0.8}}}
        md = select_suites.render_comment(result, "jev-1.13.0", None,
                                          ["deploy", "model"])
        self.assertIn("deploy", md)
        self.assertIn("Trimmed", md)
        self.assertIn("already required on every PR", md)
        self.assertIn("static", md)
        md2 = select_suites.render_comment(result, "jev", "no key", [])
        self.assertIn("disabled", md2)


class TestCLI(unittest.TestCase):
    def test_select_end_to_end_offline(self):
        diff = ("--- a/core/secrets/secret.go\n"
                "+++ b/core/secrets/secret.go\n@@\n-x\n+y\n")
        with tempfile.TemporaryDirectory() as td:
            out = os.path.join(td, "out")
            comment = os.path.join(td, "comment.md")
            rec = os.path.join(td, "rec.jsonl")
            changed = os.path.join(td, "changed")
            with open(changed, "w") as f:
                f.write("core/secrets/secret.go\n")
            sys.stdin = io.StringIO(diff)
            os.environ.pop("JEV_API_KEY", None)
            rc = select_suites.main([
                "select", "--diff", "-", "--changed-files", changed,
                "--title", "secrets fix",
                "--out", out, "--comment", comment, "--json-out", rec])
            self.assertEqual(rc, 0)
            with open(out) as fh:
                body = fh.read()
            parsed = json.loads(body.splitlines()[0].removeprefix("suites="))
            names = [e["name"] for e in parsed]
            self.assertIn("secrets_k8s", names)  # static MUST_RUN triggered
            self.assertIn("secrets_iaas", names)
            self.assertIn("has_picks=", body)
            self.assertEqual(
                [e["cloud"] for e in parsed if e["name"] == "secrets_k8s"],
                ["microk8s"])
            with open(rec) as fh:
                json.loads(fh.read().strip())
            with open(comment) as fh:
                self.assertIn("deterministic", fh.read())


def _bad_manifest(body):
    f = tempfile.NamedTemporaryFile("w", suffix=".yaml", delete=False)
    f.write("suites:\n  x:\n    " + body.replace("\n", "\n    "))
    f.close()
    return f.name


if __name__ == "__main__":
    unittest.main()
