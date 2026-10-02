#!/usr/bin/env python3
"""Offline replay eval for LLM suite selection.

Rebuilds the selection decision for past merge commits and scores it against
ground truth: which suite(s) first went red on Jenkins after each merge.

labels.json format (hand/Jenkins-transcribed one-off):
  [
    {"sha": "<merge commit>",
     "base": "<parent sha>",
     "first_fail": ["secrets_k8s"],
     "green_suites": ["deploy", "cmr", ...]}   // suites verified green that
                                               // nightly window (negatives)
  ]

Both arms run the SHIPPED selector — select_suites.merge_policy (same
threshold/max_runs the workflow reads) over build_evidence of the full
merge diff, deletions included, exactly like a live run — differing only
in the answers layer:
  static+llm  the production ranking; a Jev failure degrades that merge
              to static-only, exactly as a live run would
  static      the same pipeline with the LLM layer disabled
The delta between the arms is the LLM's marginal value over the free
deterministic layer; flip shadow -> required only if it clears the
targets below.

Metrics (per arm, over the arm's run set = top max_runs of the ranking):
  recall   first_fail suites covered by the runs
  waste    run slots spent on suites that stayed green

Usage:
  JEV_API_KEY=*** python3 tests/tools/eval_selector.py \
      --labels labels.json [--limit 50]
Cost is ~$0.001/merge; a 6-month backlog is cents.
"""

import argparse
import json
import os
import subprocess
import sys

import yaml

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import select_suites  # noqa: E402
import suite_manifest  # noqa: E402

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.abspath(os.path.join(HERE, os.pardir, os.pardir))
MANIFEST = os.path.join(HERE, os.pardir, "suites.manifest.yaml")


def evidence_for(base, head):
    """(evidence, changed_files) for one merge, via the production
    pipeline.

    No --diff-filter: the production pipeline keeps deleted files (a
    deletion is as much a behavioral change as an edit — see
    parse_diff_files), so the same evidence must feed the static layer
    here as in a live run.
    """
    def git(*args):
        return subprocess.run(
            ["git"] + list(args), cwd=REPO,
            capture_output=True, text=True, timeout=120).stdout
    diff = git("diff", f"{base}...{head}")
    files = [l for l in git("diff", "--name-only", f"{base}...{head}")
             .splitlines() if l.strip()]
    msg = git("log", "-1", "--format=%s%n%b", head)
    title, _, body = msg.partition("\n")
    return select_suites.build_evidence(diff, title, body, files), files


def _new_rec():
    return {"n": 0, "hits": 0, "targets": 0, "picks": 0, "waste": 0,
            "degraded": 0}


def run(labels, doc, suites, api_key):
    """Score both arms over the labelled merges.

    Returns (arms, per_suite): arms maps arm name -> counters, per_suite
    maps arm name -> {suite: [picked, hit_first_fail]}.
    """
    arms = {"static+llm": _new_rec(), "static": _new_rec()}
    per_suite = {"static+llm": {}, "static": {}}
    for lab in labels:
        ev, files = evidence_for(lab["base"], lab["sha"])
        targets = lab.get("first_fail", [])
        green = set(lab.get("green_suites", []))

        answers, err = select_suites.ask_jev(ev, suites, api_key)
        if err:
            # A live run degrades to static-only; score the production
            # arm exactly as it would have shipped, and keep the note.
            print(f"Jev failed on {lab['sha'][:8]}: {err} "
                  f"(merge degrades to static-only)", file=sys.stderr)
            answers = {}
        answers_by_arm = {"static+llm": answers, "static": {}}

        for arm, ans in answers_by_arm.items():
            runs = select_suites.merge_policy(
                doc, suites, files, ans)["runs"]
            rec = arms[arm]
            rec["n"] += 1
            rec["targets"] += len(targets)
            rec["hits"] += sum(1 for t in targets if t in runs)
            rec["picks"] += len(runs)
            rec["waste"] += sum(1 for s in runs if s in green)
            if arm == "static+llm" and err:
                rec["degraded"] += 1
            for s in runs:
                per_suite[arm].setdefault(s, [0, 0])
                per_suite[arm][s][0] += 1
                if s in targets:
                    per_suite[arm][s][1] += 1
    return arms, per_suite


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--labels", required=True)
    ap.add_argument("--limit", type=int, default=0)
    args = ap.parse_args()

    key = os.environ.get("JEV_API_KEY", "")
    if not key:
        print("JEV_API_KEY required", file=sys.stderr)
        return 1
    with open(MANIFEST) as f:
        doc = yaml.safe_load(f)
    suites = suite_manifest.load_manifest(MANIFEST)
    with open(args.labels) as f:
        labels = json.load(f)
    if args.limit:
        labels = labels[:args.limit]
    arms, per_suite = run(labels, doc, suites, key)
    for arm in ("static+llm", "static"):
        rec = arms[arm]
        recall = (rec["hits"] / rec["targets"]) if rec["targets"] else 0.0
        waste = (rec["waste"] / rec["picks"]) if rec["picks"] else 0.0
        header = f"== {arm}: {rec['n']} merges"
        if arm == "static+llm" and rec["degraded"]:
            header += f" ({rec['degraded']} degraded to static-only)"
        print(header)
        print(f"breakages     : {rec['targets']}")
        print(f"recall        : {recall:.2%}   (flip target >= 70%)")
        print(f"waste rate    : {waste:.2%}   (flip target <= 40%)")
    print("per-suite picked/hit:")
    for arm in ("static+llm", "static"):
        for s, (p, h) in sorted(per_suite[arm].items(),
                               key=lambda kv: -kv[1][0]):
            print(f"  [{arm:11s}] {s:20s} picked={p:4d} hits={h:3d}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
