#!/bin/bash
set -euo pipefail

project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
metadata_py="${project_dir}/scripts/snap/build_metadata.py"

# The values are quoted by build_metadata.py; version comes from the checked
# in version file and grade is one of devel/stable.
metadata="$(python3 "${metadata_py}" --project-dir "${project_dir}" --format shell)"
eval "${metadata}"

# shellcheck disable=SC2154  # assigned by eval above
echo "Adopting snap version ${version} (grade ${grade})"
snapcraftctl set-version "${version}"
snapcraftctl set-grade "${grade}"
