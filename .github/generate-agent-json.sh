#!/bin/bash
set -e
# Used to generate agent metadata for CI tests

output_file_name="${output_dir}/build-${JUJU_VERSION}-${SHORT_GIT_COMMIT}-agent.json"

content_id_part=build-${SHORT_GIT_COMMIT}
version=${JUJU_VERSION}
version_name=$(date +"%Y%m%d")

if [ ! -f "$output_file_name" ]; then
  echo "[]" > $output_file_name
fi

output_volatile=$(cat "$output_file_name")

platform_loop=(${platforms})
for platform in ${platform_loop[@]}; do
    final_comma=","
    if [[ $platform == ${platform_loop[-1]} ]]; then
        final_comma=""
    fi

    arch=$(echo "${platform}" | cut -f 2 -d '/')
    os=$(echo "${platform}" | cut -f 1 -d '/')

    agent_loop=(${product_name})
    index_type="agents"

    agent_file_name="juju-${JUJU_VERSION}-${os}-${arch}.tgz"
    agent_file_md5_sum=$(md5sum ${agent_file_name} | awk '{{print $1}}')
    agent_file_sha256_sum=$(sha256sum ${agent_file_name} | awk '{{print $1}}')
    agent_file_size=$(du -b ${agent_file_name} | awk '{{print $1}}')

    for agent_type in ${agent_loop[@]}; do
      item_name=${version}-${agent_type}-${arch}
      series_code=$(echo $agent_type | cut -d: -f2)
      release_name=$(echo $agent_type | cut -d: -f1)

      element=$(cat << EOF
[{
    "arch": "${arch}",
    "content_id": "com.ubuntu.juju:${content_id_part}:${index_type}",
    "format": "products:1.0",
    "ftype": "tar.gz",
    "item_name": "${item_name}",
    "md5": "${agent_file_md5_sum}",
    "path": "${agent_file_name}",
    "product_name": "com.ubuntu.juju:${series_code}:${arch}",
    "release": "${release_name}",
    "sha256": "${agent_file_sha256_sum}",
    "size": ${agent_file_size},
    "version": "${version}",
    "version_name": "${version_name}"
}]
EOF
)

      output_volatile=$(echo "$output_volatile" | jq ". |= . + ${element//$'\n'/}")

    done

done

echo "$output_volatile" > "$output_file_name"
