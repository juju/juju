#!/bin/bash
set -euo pipefail

project_dir="${SNAPCRAFT_PROJECT_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)}"
metadata_py="${project_dir}/scripts/snap/build_metadata.py"

version="$(python3 "${metadata_py}" --project-dir "${project_dir}" --field version)"
grade="$(python3 "${metadata_py}" --project-dir "${project_dir}" --field grade)"

echo "Adopting snap version ${version} (grade ${grade})"
snapcraftctl set-version "${version}"
snapcraftctl set-grade "${grade}"
