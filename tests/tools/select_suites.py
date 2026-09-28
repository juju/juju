#!/usr/bin/env python3
"""LLM-driven CI test-suite selection.

Given a PR's changed files + diff + title/body, decide which integration
suites (tests/suites/<name>) are plausibly affected and should run as
required checks.

Two layers:
  1. deterministic: manifest `paths` globs (MUST_RUN) — can only ADD coverage
  2. LLM (Jev): one batched call, one Noul per suite, thresholded in code

Policy invariants (enforced here, never by the model):
  - model may only ADD suites; MUST_RUN always runs
  - picks capped at top_k, filtered to GH-runner-eligible suites
  - unknown suite names in model answers are dropped
  - ANY API/parse failure degrades to deterministic-only (never raises)

Subcommands:
  select  full decision; writes GITHUB_OUTPUT lines, comment markdown,
          and a shadow JSONL record.
  ask-jev raw scorer backend (used by eval_selector.py); reads evidence
          JSON on stdin, prints {suite: {noul, confidence}} or {}.

Env:
  JEV_API_KEY      required for the LLM layer; absent -> deterministic only
  JEV_BASE_URL     default https://api.typesafe.ai/v1/systemone
  JEV_MODEL        default jev-1.13.0 (pin; bump deliberately)
"""

import argparse
import json
import os
import re
import sys
import time
import urllib.error
import urllib.request
import uuid

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import suite_manifest  # noqa: E402

DEFAULT_BASE_URL = "https://jevmodel.org/v1/systemone"
DEFAULT_MODEL = "jev-1.13.0"
DEFAULT_THRESHOLD = 0.6
DEFAULT_CONFIDENCE = 0.5
DEFAULT_TOP_K = 3
DEFAULT_MAX_RUNS = 6
# Jev contract (jevmodel.org/docs): `state` <= 8000 chars after JSON
# serialization; 1-8 questions per request; a noul has NO separate
# confidence field — the probability is the certainty measure.
STATE_CHAR_LIMIT = 7800
MAX_QUESTIONS_PER_REQUEST = 8
# Hunks get most of the state budget; title/body/files/rollup take the rest.
HUNK_CHAR_BUDGET = 5200
HTTP_TIMEOUT = 30
# Wall-clock ceiling for the whole Jev fan-out (all batches, retries
# included). A stalled network must cost at most this much of a CI job.
TOTAL_BUDGET_S = 120
RETRY_STATUS = (429, 500, 502, 503, 504)
MAX_ATTEMPTS = 3

# File priority for hunk inclusion: behaviour-relevant first.
PRIORITY = [
    (re.compile(r"\.sql$|domain/schema/"), 0),
    (re.compile(r"\.go$"), 1),
    (re.compile(r"\.sh$|tests/"), 2),
    (re.compile(r"\.yaml$|\.yml$|\.json$"), 3),
    (re.compile(r"\.md$|docs/"), 4),
]
DROP = re.compile(r"^_deps/|/testdata/|^vendor/|_mock\.go$|\.pb\.go$")


# ---------------------------------------------------------------- evidence

def file_priority(path):
    for rx, prio in PRIORITY:
        if rx.search(path):
            return prio
    return 5


def build_evidence(diff_text, title, body, changed_files,
                   budget=HUNK_CHAR_BUDGET):
    """Shape a raw unified diff into the JSON state sent to the model.

    The Jev API caps `state` at 8000 chars after JSON serialization, so the
    whole dict must fit: title/body/files are trimmed first, hunks are
    appended in priority order until the remaining budget is exhausted;
    leftovers are listed so the model knows what it did not see.
    """
    files = parse_diff_files(diff_text)
    ordered = sorted(files, key=lambda f: (file_priority(f["path"]), f["path"]))

    rollup = {}
    for f in files:
        top = "/".join(f["path"].split("/")[:2])
        rollup[top] = rollup.get(top, 0) + 1

    hunks = []
    included = []
    cost = 0
    omitted = []
    for f in ordered:
        block = f"--- a/{f['path']}\n+++ b/{f['path']}\n{f['hunk']}"
        c = len(block)
        if cost + c > budget:
            omitted.append(f["path"])
            continue
        hunks.append(block)
        included.append(f["path"])
        cost += c

    ev = {
        "pr_title": (title or "")[:200],
        "pr_body": (body or "")[:1500],
        "changed_files": [f["path"] for f in files][:150],
        "dir_rollup": rollup,
        "hunks": "\n\n".join(hunks),
        "hunks_omited_for_size": omitted,
    }
    # Hard backstop: shrink the least-valuable fields until the serialized
    # state fits the API limit. Hunks go first (lowest priority appended
    # last), then body, then the files list, then the title.
    while len(json.dumps(ev)) > STATE_CHAR_LIMIT:
        if hunks:
            hunks.pop()
            omitted.append(included.pop())
            ev["hunks"] = "\n\n".join(hunks)
            continue
        if ev["pr_body"]:
            ev["pr_body"] = ev["pr_body"][: max(0, len(ev["pr_body"]) // 2)]
            continue
        if len(ev["changed_files"]) > 60:
            ev["changed_files"] = ev["changed_files"][:60]
            continue
        if len(ev["pr_title"]) > 40:
            ev["pr_title"] = ev["pr_title"][:40]
            continue
        break
    return ev


def parse_diff_files(diff_text):
    """Split a unified diff into [{path, hunk}] with DROP-paths removed."""
    files = []
    cur = None
    for line in diff_text.splitlines():
        m = re.match(r"^\+\+\+ b/(.+)$", line)
        if m:
            path = m.group(1).strip()
            if path.startswith("/dev/null") or DROP.search(path):
                cur = None
                continue
            cur = {"path": path, "lines": []}
            files.append(cur)
            continue
        if cur is None:
            continue
        if line.startswith("diff --git"):
            continue
        cur["lines"].append(line)
    out = []
    for f in files:
        hunk = "\n".join(f["lines"])
        # Trim very large single-file hunks: keep head+tail.
        if len(hunk) > 6000:
            hunk = hunk[:3500] + "\n... [trimmed] ...\n" + hunk[-2000:]
        out.append({"path": f["path"], "hunk": hunk})
    return out


# ---------------------------------------------------------------- Jev client

def build_jev_request(evidence, suites, model=DEFAULT_MODEL):
    """One state, one Noul per suite (docs.typesafe.ai fan-out pattern)."""
    questions = {}
    for name, s in suites.items():
        questions[name] = {
            "type": "noul",
            "instructions": {
                "question": (
                    "In `hunks`, do the changes plausibly alter behavior "
                    "that `target` verifies end to end on a live "
                    "controller?"),
                "target": (s.get("description") or name).strip(),
            },
            "criteria": {
                "true": (
                    "A failure of this test could reasonably be caused by "
                    "these changes (keywords: "
                    + ", ".join(s.get("keywords") or []) + ")"),
                "false": (
                    "No observable CLI/charm-facing behavior verified by "
                    "this test changes"),
            },
        }
    return {"model": model, "state": evidence, "questions": questions}


def parse_jev_response(payload, expected_suites):
    """Extract {suite: (noul, confidence)}; ignore unknown question keys.

    A noul has no separate confidence field — the probability IS the
    certainty measure — so it doubles as confidence when absent.
    """
    answers = {}
    ans = payload.get("answers") or {}
    for key, a in ans.items():
        if key not in expected_suites:
            continue  # whitelist: model cannot introduce new names
        noul = a.get("noul")
        if noul is None:
            continue
        conf = a.get("confidence")
        answers[key] = (float(noul), float(conf) if conf is not None
                        else float(noul))
    return answers


def ask_jev(evidence, suites, api_key, base_url=DEFAULT_BASE_URL,
            model=DEFAULT_MODEL, total_budget_s=TOTAL_BUDGET_S):
    """Call the Jev API with backoff, fanning out in <=8-question batches.

    Never raises: ({}, error_or_None). A batch error is reported but does
    not discard answers from the other batches. The whole fan-out runs
    under a wall-clock budget: unreachable networks (corporate proxies,
    blackholed IPv6) must degrade the selection quickly instead of
    hanging the CI job or the developer terminal.
    """
    names = list(suites)
    answers = {}
    errs = []
    n_batches = max(
        1, (len(names) + MAX_QUESTIONS_PER_REQUEST - 1)
        // MAX_QUESTIONS_PER_REQUEST)
    start = time.monotonic()
    for bi, i in enumerate(range(0, len(names), MAX_QUESTIONS_PER_REQUEST), 1):
        remaining = total_budget_s - (time.monotonic() - start)
        if remaining <= 0:
            errs.append(
                f"time budget {total_budget_s}s exhausted after "
                f"{bi - 1}/{n_batches} batches")
            break
        chunk = {n: suites[n] for n in names[i:i + MAX_QUESTIONS_PER_REQUEST]}
        if n_batches > 1:
            # Liveness: without this a slow network looks like a hang.
            print(f"[select] Jev batch {bi}/{n_batches} "
                  f"({len(chunk)} suites)", file=sys.stderr, flush=True)
        ans, err = _ask_jev_chunk(evidence, chunk, api_key, base_url, model,
                                  deadline=start + total_budget_s)
        answers.update(ans)
        if err:
            errs.append(err)
    if errs:
        uniq = list(dict.fromkeys(errs))
        err = ", ".join(uniq)
        if len(errs) > 1:
            err += f" ({len(errs)}/{n_batches} batches)"
        return answers, err
    return answers, None


def _ask_jev_chunk(evidence, suites, api_key, base_url, model, deadline):
    """One Jev request for <=8 suites, retried with backoff."""
    body = json.dumps(build_jev_request(evidence, suites, model)).encode()
    req = urllib.request.Request(
        base_url, data=body, method="POST",
        headers={
            "Authorization": f"Bearer {api_key}",
            "Content-Type": "application/json",
            # API guidance: idempotency key when retrying (stable per batch).
            "Idempotency-Key": uuid.uuid4().hex,
        })
    err = None
    for attempt in range(1, MAX_ATTEMPTS + 1):
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            return {}, "time budget exhausted"
        try:
            with urllib.request.urlopen(
                    req, timeout=min(HTTP_TIMEOUT, remaining)) as resp:
                payload = json.loads(resp.read().decode())
            return parse_jev_response(payload, set(suites)), None
        except urllib.error.HTTPError as e:
            snippet = ""
            try:
                snippet = e.read(200).decode(errors="replace")
                snippet = " ".join(snippet.split())[:200]
            except Exception:  # noqa: BLE001 - best-effort diagnostics
                pass
            err = f"http {e.code}" + (f": {snippet}" if snippet else "")
            if e.code not in RETRY_STATUS or attempt == MAX_ATTEMPTS:
                return {}, err
            time.sleep(min(2 ** attempt,
                           int(e.headers.get("retry-after") or 2 ** attempt)))
        except Exception as e:  # noqa: BLE001 - degrade, never block PRs
            err = f"{type(e).__name__}: {e}"
            if attempt == MAX_ATTEMPTS:
                return {}, err
            time.sleep(2)
    return {}, err


# ---------------------------------------------------------------- policy

def rank(suites, changed_files, answers):
    """One ranked list of eligible suites by how strongly the change
    flexes them. score = static specificity (precise globs beat hubs)
    + LLM p(yes) when available. Already-required suites are excluded;
    they are covered by other CI. Returns [(name, score, source)] desc.
    """
    static = dict(suite_manifest.static_rank(suites, changed_files))
    ranked = []
    for name, s in suites.items():
        if not s.get("gh_eligible") or s.get("already_required"):
            continue
        score = static.get(name, 0.0)
        source = "static"
        ans = answers.get(name)
        if ans:
            score += ans[0]  # LLM p(yes), 0..1, breaks ties / expands
            source = "llm" if static.get(name, 0.0) == 0 else "static+llm"
        if score > 0:
            ranked.append((name, score, source))
    ranked.sort(key=lambda x: (-x[1], x[0]))
    return ranked


def merge_policy(manifest_doc, suites, changed_files, answers,
                 max_runs=DEFAULT_MAX_RUNS):
    """Rank every eligible suite, keep the top max_runs, report the rest.

    There is no separate must-run bypass: hubs that match almost every
    PR rank low (shared specificity ~0.17) and compete for slots like
    anything else. Precise ownership globs (secrets, schema) rank high
    by construction. Already-required suites never appear. Trimmed
    suites are surfaced in the comment, never hidden.
    """
    max_runs = manifest_doc.get("max_runs", max_runs)
    excluded = {n for n, s in suites.items() if s.get("already_required")}
    ranked = rank(suites, changed_files, answers)
    runs = [n for n, _, _ in ranked[:max_runs]]
    trimmed = [n for n, _, _ in ranked[max_runs:]]
    skipped_required = sorted(
        n for n, (p, _) in answers.items()
        if n in excluded and p >= manifest_doc.get("threshold",
                                                     DEFAULT_THRESHOLD))
    return {
        "runs": runs,
        "ranked": [{"name": n, "score": round(s, 3), "source": src}
                   for n, s, src in ranked],
        "trimmed": trimmed,
        "skipped_required": skipped_required,
        "all_scores": {n: {"noul": p, "confidence": c}
                       for n, (p, c) in answers.items()},
    }


# ---------------------------------------------------------------- output

def render_comment(result, model_id, llm_error, runs):
    lines = ["### 🤖 LLM test-suite selection (shadow mode)", ""]
    if llm_error:
        lines.append(f"> LLM layer disabled/failed (`{llm_error}`) — "
                     "deterministic layer only.")
    else:
        lines.append(f"> Model `{model_id}` · picks are advisory and "
                     "non-blocking for now.")
    lines += ["", f"**Runs (top {len(runs)} by score):** "
              f"{', '.join(runs) or '—'}"]
    if result.get("trimmed"):
        lines.append(f"**Trimmed (runner budget):** "
                     f"{', '.join(result['trimmed'])}")
    if result.get("skipped_required"):
        lines.append(f"**Excluded (already required on every PR):** "
                     f"{', '.join(result['skipped_required'])}")
    if result.get("ranked"):
        lines += ["", "| suite | score | signal |", "|---|---|---|"]
        for r in result["ranked"][:10]:
            lines.append(f"| {r['name']} | {r['score']:.2f} | "
                          f"{r['source']} |")
    lines += ["", "<sub>feedback: reply on this PR; data recorded for "
              "offline eval (see tests/tools/eval_selector.py)</sub>"]
    return "\n".join(lines)


SUITE_TO_CLOUD = {"lxd": "localhost", "k8s": "microk8s",
                  "localhost": "localhost"}


def write_output(path, suites, runs, has_picks):
    """GITHUB_OUTPUT: suites=[{"name","provider","cloud"}], has_picks."""
    entries = [{
        "name": n,
        "provider": suites[n]["provider"],
        "cloud": SUITE_TO_CLOUD.get(suites[n]["provider"], "localhost"),
    } for n in runs]
    with open(path, "a") as f:
        f.write(f"suites={json.dumps(entries)}\n")
        f.write(f"has_picks={'true' if runs else 'false'}\n")


# ---------------------------------------------------------------- CLI

def main(argv=None):
    ap = argparse.ArgumentParser(prog="select_suites.py")
    ap.add_argument("cmd", choices=["select", "ask-jev"])
    ap.add_argument("--manifest", default=None)
    ap.add_argument("--diff", default="-", help="unified diff file (- stdin)")
    ap.add_argument("--changed-files", default="-", help="newline list")
    ap.add_argument("--title", default="")
    ap.add_argument("--body", default="")
    ap.add_argument("--out", help="GITHUB_OUTPUT file")
    ap.add_argument("--comment", help="write markdown comment here")
    ap.add_argument("--json-out", help="write shadow JSONL record here")
    args = ap.parse_args(argv)

    here = os.path.dirname(os.path.abspath(__file__))
    manifest_path = args.manifest or os.path.join(
        here, os.pardir, "suites.manifest.yaml")
    with open(manifest_path) as f:
        doc = yaml_load(f)
    suites = suite_manifest.load_manifest(manifest_path)

    diff = read(args.diff)
    changed = [l.strip() for l in read(args.changed_files).splitlines()
               if l.strip()]
    if args.cmd == "ask-jev":
        evidence = json.loads(diff)
        key = os.environ.get("JEV_API_KEY", "")
        answers, _ = ask_jev(
            evidence, suites, key,
            os.environ.get("JEV_BASE_URL") or DEFAULT_BASE_URL,
            os.environ.get("JEV_MODEL") or DEFAULT_MODEL)
        out = {n: {"noul": p, "confidence": c}
               for n, (p, c) in answers.items()}
        print(json.dumps(out))
        return 0

    evidence = build_evidence(diff, args.title, args.body, changed)

    key = os.environ.get("JEV_API_KEY", "")
    model_id = os.environ.get("JEV_MODEL") or DEFAULT_MODEL
    llm_error = None
    answers = {}
    if key:
        answers, llm_error = ask_jev(
            evidence, suites, key,
            os.environ.get("JEV_BASE_URL") or DEFAULT_BASE_URL, model_id)
    else:
        llm_error = "JEV_API_KEY not set"

    result = merge_policy(doc, suites, changed, answers)
    runs = result["runs"]

    if args.out:
        # has_picks gates the runner matrix: any suite to execute (static
        # must-run OR LLM picks) means the matrix must fan out.
        write_output(args.out, suites, runs, bool(runs))
    if args.comment:
        with open(args.comment, "w") as f:
            f.write(render_comment(result, model_id, llm_error, runs))
    if args.json_out:
        rec = {
            "pr": os.environ.get("PR_NUMBER", ""),
            "sha": os.environ.get("PR_SHA", ""),
            "model": model_id,
            "llm_error": llm_error,
            "changed_files": changed[:200],
            "result": result,
            "runs": runs,
        }
        with open(args.json_out, "a") as f:
            f.write(json.dumps(rec) + "\n")
    print(json.dumps({"runs": runs, **{
        k: result[k] for k in ("trimmed", "skipped_required")}}))
    return 0


def yaml_load(f):
    import yaml
    return yaml.safe_load(f)


def read(path):
    if path == "-":
        return sys.stdin.read()
    with open(path) as f:
        return f.read()


if __name__ == "__main__":
    sys.exit(main())
