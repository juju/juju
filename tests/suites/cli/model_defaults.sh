run_model_defaults_isomorphic() {
  echo

  FILE=$(mktemp)

  juju model-defaults --format=yaml | juju model-defaults --ignore-read-only-fields -
}

run_model_defaults_cloudinit_userdata() {
  echo

  FILE=$(mktemp)

  cat << EOF > "${FILE}"
cloudinit-userdata: |
  packages:
    - shellcheck
EOF

  juju model-defaults "${FILE}"
  juju model-defaults cloudinit-userdata --format=yaml | grep -q "default: \"\""
  juju model-defaults cloudinit-userdata --format=yaml  | grep -q "shellcheck"
}

test_model_defaults() {
  if [ "$(skip 'test_model_defaults')" ]; then
    echo "==> TEST SKIPPED: model defaults"
    return
  fi

  (
    set_verbosity

    cd .. || exit

    run "run_model_defaults_isomorphic"
    run "run_model_defaults_cloudinit_userdata"
  )
}