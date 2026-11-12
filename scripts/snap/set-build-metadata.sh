#!/bin/bash
set -euo pipefail

project_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
metadata_py="${project_dir}/scripts/snap/build_metadata.py"

# The Go plugin computes the metadata first so the linker flags and the
# adopted snap version come from the same observation of the source tree.
# Fall back to computing it here if the plugin did not leave a file.
metadata_file="${JUJU_BUILD_METADATA_FILE:-}"
if [[ -z "${metadata_file}" && -n "${SNAPCRAFT_PART_BUILD:-}" ]]; then
  metadata_file="${SNAPCRAFT_PART_BUILD}/build-metadata.json"
fi
if [[ -n "${metadata_file}" && -f "${metadata_file}" ]]; then
  metadata="$(python3 "${metadata_py}" --input "${metadata_file}" --format shell)"
else
  metadata="$(python3 "${metadata_py}" --project-dir "${project_dir}" --format shell)"
fi
eval "${metadata}"

# shellcheck disable=SC2154  # assigned by eval above
echo "Adopting snap version ${version} (grade ${grade})"
craftctl set version="${version}"
craftctl set grade="${grade}"
