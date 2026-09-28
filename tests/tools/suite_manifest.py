"""Load and validate tests/suites.manifest.yaml.

The manifest is the single source of truth for LLM-driven CI test-suite
selection: provider, GitHub-runner eligibility, behavior descriptions (used
as LLM question criteria), keywords, and deterministic must-run path globs.

Keep this module dependency-light: PyYAML + stdlib only. It is imported by
select_suites.py in CI and by unit tests locally.
"""

import fnmatch
import os
import re
import subprocess
import sys

import yaml

# Providers recognised in the manifest (mirrors tests/main.sh -p values;
# "none" means the suite needs no controller at all, e.g. static_analysis).
KNOWN_PROVIDERS = {
    "lxd", "k8s", "aws", "ec2", "gce", "google", "azure", "maas",
    "vsphere", "openstack", "unmanaged", "localhost", "none",
}

# Suites that GitHub self-hosted runners can execute today. Anything else
# requires Jenkins credentials/clouds and cannot be run on a PR.
GH_PROVIDERS = {"lxd", "k8s", "localhost", "none"}

REQUIRED_FIELDS = ("provider", "gh_eligible", "description", "keywords")


class ManifestError(Exception):
    """Raised when the manifest is missing, malformed, or out of sync."""


def load_manifest(path):
    """Load and validate the manifest, returning the suites dict.

    Adds a private "_name" key is NOT used; suite name is the dict key.
    """
    try:
        with open(path) as f:
            doc = yaml.safe_load(f)
    except FileNotFoundError:
        raise ManifestError(f"manifest not found: {path}")
    except yaml.YAMLError as e:
        raise ManifestError(f"manifest is not valid YAML: {path}: {e}")

    if not isinstance(doc, dict) or not isinstance(doc.get("suites"), dict):
        raise ManifestError(f"manifest missing top-level 'suites' map: {path}")

    suites = doc["suites"]
    errors = []

    for name, s in suites.items():
        if not isinstance(s, dict):
            errors.append(f"{name}: entry must be a mapping")
            continue
        for field in REQUIRED_FIELDS:
            if field not in s:
                errors.append(f"{name}: missing required field '{field}'")
        prov = s.get("provider")
        if prov is not None and prov not in KNOWN_PROVIDERS:
            errors.append(f"{name}: unknown provider '{prov}'")
        if "gh_eligible" in s and not isinstance(s["gh_eligible"], bool):
            errors.append(f"{name}: gh_eligible must be a bool")
        desc = s.get("description") or ""
        if isinstance(desc, str) and len(desc.split()) > 60:
            errors.append(
                f"{name}: description too long "
                f"({len(desc.split())} words, keep <=40)")
        kw = s.get("keywords")
        if kw is not None and not isinstance(kw, list):
            errors.append(f"{name}: keywords must be a list")
        paths = s.get("paths", [])
        if paths is None:
            paths = []
        if not isinstance(paths, list):
            errors.append(f"{name}: paths must be a list")
        elif any(not isinstance(p, str) for p in paths):
            errors.append(f"{name}: paths entries must be strings")
        s.setdefault("paths", [])

    if errors:
        raise ManifestError(
            "manifest validation failed:\n  " + "\n  ".join(errors))

    return suites


def check_matches_suites_dir(suites, suites_dir):
    """Assert manifest suite names == directories under tests/suites/."""
    if not os.path.isdir(suites_dir):
        raise ManifestError(f"suites dir not found: {suites_dir}")
    on_disk = {
        d for d in os.listdir(suites_dir)
        if os.path.isfile(os.path.join(suites_dir, d, "task.sh"))
    }
    declared = set(suites)
    missing = sorted(on_disk - declared)
    extra = sorted(declared - on_disk)
    problems = []
    if missing:
        problems.append(f"suites missing from manifest: {missing}")
    if extra:
        problems.append(f"manifest entries with no tests/suites dir: {extra}")
    if problems:
        raise ManifestError("; ".join(problems))


def matching_paths(suites, changed_files):
    """Return the set of suite names whose must-run paths match a change."""
    picks = set()
    for name, s in suites.items():
        for pattern in s.get("paths") or []:
            for f in changed_files:
                if _glob_match(pattern, f):
                    picks.add(name)
                    break
            else:
                continue
            break
    return picks


def _glob_match(pattern, path):
    """Match a manifest glob against a repo-relative path.

    Segment-wise: '*' matches within one path segment, '**' matches any
    (possibly empty) run of segments.
    """
    if pattern == path:
        return True
    pat = pattern.split("/")
    seg = path.split("/")

    def match(p, s):
        if not p:
            return not s
        if p[0] == "**":
            # consume ** against 0..n segments
            for i in range(len(s) + 1):
                if match(p[1:], s[i:]):
                    return True
            return False
        if not s:
            return False
        if not fnmatch.fnmatch(s[0], p[0]):
            return False
        return match(p[1:], s[1:])

    return match(pat, seg)


def parse_churn(git_log_output):
    """Parse `git log --name-only` output into co-change counts.

    Input: raw stdout of e.g.
        git log --since=<date> --name-only --pretty=format:%H -- <suite task.sh>
    Lines are commit hashes (40-hex) followed by changed file paths.
    Returns list of (path, count) sorted by count desc, ties by path.
    Files on the suite's own task.sh are excluded.
    """
    counts = {}
    in_commit = False
    for line in git_log_output.splitlines():
        line = line.strip()
        if not line:
            continue
        if re.fullmatch(r"[0-9a-f]{40}", line):
            in_commit = True
            continue
        if not in_commit:
            continue
        counts[line] = counts.get(line, 0) + 1
    ranked = sorted(counts.items(), key=lambda kv: (-kv[1], kv[0]))
    return ranked


def churn_top_files(repo_dir, suite, since_days=90, limit=30):
    """Git churn: files co-changed alongside commits touching a suite."""
    target = os.path.join("tests", "suites", suite, "task.sh")
    cmd = [
        "git", "log", f"--since={since_days}.days.ago",
        "--name-only", "--pretty=format:%H", "--", target,
    ]
    try:
        out = subprocess.run(
            cmd, cwd=repo_dir, capture_output=True, text=True, timeout=60,
        )
        if out.returncode != 0:
            return []
    except (OSError, subprocess.TimeoutExpired):
        return []
    ranked = parse_churn(out.stdout)
    return [
        (p, c) for (p, c) in ranked
        if p != target
    ][:limit]


if __name__ == "__main__":  # tiny CLI for CI smoke: validate the manifest
    here = os.path.dirname(os.path.abspath(__file__))
    tests_dir = os.path.dirname(here)
    path = sys.argv[1] if len(sys.argv) > 1 else os.path.join(
        tests_dir, "suites.manifest.yaml")
    suites = load_manifest(path)
    check_matches_suites_dir(suites, os.path.join(tests_dir, "suites"))
    print(f"manifest OK: {len(suites)} suites")
