# Checks whether the cwd is used for the juju local deploy.
run_deploy_local_charm_revision() {
  echo

  file="${TEST_DIR}/local-charm-deploy-no-git.txt"

  ensure "local-charm-deploy" "${file}"

  TMP_NO_GIT=$(mktemp -d -t ci-XXXXXXXXXX)
  cd "${TMP_NO_GIT}" || exit 1

  git clone --depth=1 --quiet https://github.com/lampkicking/charm-ntp.git ntp
  cd "${TMP_NO_GIT}/ntp" || exit 1

  OUTPUT=$(juju deploy . 2>&1)

  check_not_contains "${OUTPUT}" "exit status 128"

  destroy_model "local-charm-deploy"
}

# CWD with git, deploy charm with git, but -> check that git describe is correct
run_deploy_local_charm_revision_invalid_git() {
  echo

  file="${TEST_DIR}/local-charm-deploy-wrong-git.txt"

  ensure "local-charm-deploy-wrong-git" "${file}"

  TMP_CHARM_GIT=$(mktemp -d -t ci-XXXXXXXXXX)
  TMP_NO_CHARM_GIT=$(mktemp -d -t ci-XXXXXXXXXX)

  cd "${TMP_CHARM_GIT}" || exit 1
  git clone --depth=1 --quiet https://github.com/lampkicking/charm-ntp.git ntp

  cd "${TMP_CHARM_GIT}/ntp" || exit 1
  WANTED_CHARM_SHA=\"$(git describe --dirty --always)\"

  cd "${TMP_NO_CHARM_GIT}" || exit 1

  create_local_git_folder

  juju deploy "${TMP_CHARM_GIT}"/ntp ntp

  wait_for "ntp" ".applications | keys[0]"

  CURRENT_CHARM_SHA=$(juju status --format=json | jq '.applications.ntp."charm-version"')
  if [ "${WANTED_CHARM_SHA}" != "${CURRENT_CHARM_SHA}" ]; then
    echo "The expected sha does not equal the current SHA"
    exit 1
  fi

  destroy_model "local-charm-deploy-wrong-git"
}

create_local_git_folder() {
  git init .
  touch rand_file
  git add rand_file
  git commit -am "rand_file"
}

test_local_charms() {
    if [ "$(skip 'test_local_charms')" ]; then
        echo "==> TEST SKIPPED: deploy local charm tests"
        return
    fi

    (
        set_verbosity

        cd .. || exit

        run "run_deploy_local_charm_revision"
        run "run_deploy_local_charm_revision_invalid_git"
    )
}
