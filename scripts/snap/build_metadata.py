#!/usr/bin/env python3
"""Compute build metadata for the Juju snap.

The same values feed both snapcraft's adopted version/grade and the Go
linker flags.

Launchpad provides SNAPCRAFT_IMAGE_INFO as a JSON object containing at least
``build_url`` and ``build-request-id``. The recipe name in ``build_url``
selects the build mode:

- ``juju-edge-*``: edge build. The snap version is ``<base>-<commit>``,
  grade is ``devel``, the Launchpad build request id (``lp-`` prefix
  removed) is recorded as the build number, and OCI images come from the
  test registry.
- ``juju-release`` or ``juju-release-*``: release build. The snap version is
  the checked-in version, grade is ``stable``, and OCI images come from
  ``ghcr.io/juju``.

Local builds have no image info and fall back to the checked-in version with
devel metadata; the same applies to CI builders that set
SNAPCRAFT_IMAGE_INFO purely for manifest provenance. A Launchpad build whose
recipe matches neither pattern fails.
"""

import argparse
import json
import os
import re
import shlex
import subprocess
import sys

EDGE_OCI_NAMESPACE = (
    "us-central1-docker.pkg.dev/gothic-list-89514/juju-testing/juju"
)
RELEASE_OCI_NAMESPACE = "ghcr.io/juju"


def image_info():
    raw = os.environ.get("SNAPCRAFT_IMAGE_INFO")
    if not raw:
        return {}
    try:
        info = json.loads(raw)
    except (TypeError, ValueError) as e:
        raise SystemExit(f"invalid SNAPCRAFT_IMAGE_INFO: {e}") from e
    if not isinstance(info, dict):
        raise SystemExit(
            "invalid SNAPCRAFT_IMAGE_INFO: expected a JSON object"
        )
    return info


def detect_mode(info):
    build_url = str(info.get("build_url", ""))
    if not build_url:
        return "local"
    match = re.search(r"/\+snap/([^/?#]+)", build_url)
    if not match:
        # Not a Launchpad snap recipe build; some CI builders set
        # SNAPCRAFT_IMAGE_INFO for manifest provenance only.
        return "local"
    recipe = match.group(1)
    if recipe.startswith("juju-edge-"):
        return "edge"
    if recipe == "juju-release" or recipe.startswith("juju-release-"):
        return "release"
    raise SystemExit(
        f"unsupported Launchpad snap recipe {recipe!r}: "
        "expected juju-edge-* or juju-release*"
    )


def git_commit(project_dir):
    try:
        return subprocess.check_output(
            ["git", "-C", project_dir, "rev-parse", "HEAD"],
            stderr=subprocess.DEVNULL,
            text=True,
        ).strip()
    except (OSError, subprocess.CalledProcessError):
        return ""


def git_tree_state(project_dir, commit):
    if not commit:
        return "archive"
    try:
        status = subprocess.check_output(
            ["git", "-C", project_dir, "status", "--porcelain"],
            stderr=subprocess.DEVNULL,
            text=True,
        )
    except (OSError, subprocess.CalledProcessError):
        return "archive"
    return "dirty" if status.strip() else "clean"


def base_version(project_dir):
    candidates = (
        os.path.join(project_dir, "version", "version.go"),
        os.path.join(project_dir, "core", "version", "version.go"),
    )
    for path in candidates:
        try:
            with open(path, encoding="utf-8") as f:
                match = re.search(r'^const version = "(.*)"', f.read(), re.M)
        except OSError:
            continue
        if match:
            return match.group(1)
    raise SystemExit(f"could not find the Juju version in {project_dir}")


def build_number(info, mode):
    if mode != "edge":
        return ""
    value = str(info.get("build-request-id", ""))
    if value.startswith("lp-"):
        value = value[3:]
    return value if value.isdigit() else ""


def metadata(project_dir):
    info = image_info()
    mode = detect_mode(info)
    commit = git_commit(project_dir)
    base = base_version(project_dir)
    if mode in ("edge", "release") and not commit:
        raise SystemExit(f"{mode} build requires a git checkout")
    build = build_number(info, mode)
    if mode == "edge" and not build:
        raise SystemExit(
            "edge build requires a numeric Launchpad build-request-id"
        )
    version = base
    if mode == "edge":
        version = f"{base}-{commit[:7]}"
    return {
        "mode": mode,
        "version": version,
        "grade": "devel" if mode in ("edge", "local") else "stable",
        "git_commit": commit,
        "git_tree_state": git_tree_state(project_dir, commit),
        "build": build,
        "oci_namespace": {
            "edge": EDGE_OCI_NAMESPACE,
            "release": RELEASE_OCI_NAMESPACE,
        }.get(mode, ""),
    }


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--project-dir", default=os.getcwd())
    parser.add_argument(
        "--format", choices=("json", "shell"), default="json"
    )
    parser.add_argument(
        "--field",
        choices=(
            "mode",
            "version",
            "grade",
            "git_commit",
            "git_tree_state",
            "build",
            "oci_namespace",
        ),
    )
    args = parser.parse_args()
    values = metadata(args.project_dir)
    if args.field:
        print(values[args.field])
    elif args.format == "shell":
        for key in ("version", "grade"):
            print(f"{key}={shlex.quote(values[key])}")
    else:
        json.dump(values, sys.stdout, sort_keys=True)
        print()


if __name__ == "__main__":
    main()
