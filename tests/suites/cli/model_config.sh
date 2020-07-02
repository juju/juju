run_model_config_isomorphic() {
  echo

  FILE=$(mktemp)

  juju model-config --format=yaml > "${FILE}" && \
    sed -i '/^agent\-version/,/source\: .*/d' "${FILE}" && \
    juju model-config "${FILE}"
}

run_model_config_cloudinit_userdata() {
  echo

  FILE=$(mktemp)

  cat << EOF > "${FILE}"
cloudinit-userdata: |
  packages:
    - jq
    - shellcheck
EOF

  juju model-config "${FILE}"
  juju model-config cloudinit-userdata | grep -q "shellcheck"

  # cloudinit-userdata is not present from the default tabluar output
  ! juju model-config cloudinit-userdata | grep -q "^cloudinit-userdata: |$"

  # cloudinit-userdata is hidden in the normal output
  juju model-config | grep -q "<value set, see juju model-config cloudinit-userdata>"
}

test_model_config() {
  if [ "$(skip 'test_model_config')" ]; then
    echo "==> TEST SKIPPED: model config"
    return
  fi

  (
    set_verbosity

    cd .. || exit

    run "run_model_config_isomorphic"
    run "run_model_config_cloudinit_userdata"
  )
}