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

Metrics:
  recall@K   of the first_fail suites vs the model's top-K ranking
  waste      picks (above threshold) on suites that stayed green

Usage:
  JEV_API_KEY=*** python3 tests/tools/eval_selector.py \
      --labels labels.json [--top-k 3] [--limit 50]
Cost is ~$0.001/merge; a 6-month backlog is cents.
"""

import argparse
import json
import os
import subprocess
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import select_suites  # noqa: E402
import suite_manifest  # noqa: E402

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.abspath(os.path.join(HERE, os.pardir, os.pardir))


def evidence_for(base, head):
    def git(*args):
        return subprocess.run(
            ["git"] + list(args), cwd=REPO,
            capture_output=True, text=True, timeout=120).stdout
    diff = git("diff", "--diff-filter=ACMRT", f"{base}...{head}")
    files = [l for l in git("diff", "--name-only", "--diff-filter=ACMRT",
                            f"{base}...{head}").splitlines() if l.strip()]
    msg = git("log", "-1", "--format=%s%n%b", head)
    title, _, body = msg.partition("\n")
    return select_suites.build_evidence(diff, title, body, files)


def run(labels, suites, api_key, top_k=3):
    rec = {"n": 0, "hits": 0, "targets": 0, "picks": 0, "waste": 0}
    per_suite = {}
    for lab in labels:
        ev = evidence_for(lab["base"], lab["sha"])
        answers, err = select_suites.ask_jev(ev, suites, api_key)
        if err:
            print(f"skip {lab['sha'][:8]}: {err}", file=sys.stderr)
            continue
        ranked = sorted(
            ((n, p) for n, (p, _) in answers.items()
             if suites[n].get("gh_eligible")
             and not suites[n].get("already_required")),
            key=lambda x: -x[1])
        top = [n for n, _ in ranked[:top_k]]
        above = [n for n, p in ranked
                 if p >= select_suites.DEFAULT_THRESHOLD]
        targets = [s for s in lab.get("first_fail", [])]
        green = set(lab.get("green_suites", []))
        rec["n"] += 1
        rec["targets"] += len(targets)
        rec["hits"] += sum(1 for t in targets if t in top)
        for s in above:
            rec["picks"] += 1
            if s in green:
                rec["waste"] += 1
            per_suite.setdefault(s, [0, 0])
            per_suite[s][0] += 1
            if s in targets:
                per_suite[s][1] += 1
    recall = (rec["hits"] / rec["targets"]) if rec["targets"] else 0.0
    waste = (rec["waste"] / rec["picks"]) if rec["picks"] else 0.0
    return rec, recall, waste, per_suite


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--labels", required=True)
    ap.add_argument("--top-k", type=int, default=3)
    ap.add_argument("--limit", type=int, default=0)
    args = ap.parse_args()

    key = os.environ.get("JEV_API_KEY", "")
    if not key:
        print("JEV_API_KEY required", file=sys.stderr)
        return 1
    suites = suite_manifest.load_manifest(os.path.join(
        HERE, os.pardir, "suites.manifest.yaml"))
    with open(args.labels) as f:
        labels = json.load(f)
    if args.limit:
        labels = labels[:args.limit]
    rec, recall, waste, per_suite = run(labels, suites, key, args.top_k)
    print(f"merges scored : {rec['n']}")
    print(f"breakages     : {rec['targets']}")
    print(f"recall@{args.top_k}      : {recall:.2%}   (exit target >= 70%)")
    print(f"waste rate    : {waste:.2%}   (exit target <= 40%)")
    print("per-suite picked/hit:")
    for s, (p, h) in sorted(per_suite.items(), key=lambda kv: -kv[1][0]):
        print(f"  {s:20s} picked={p:4d} hits={h:3d}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
