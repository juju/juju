#checks whether the cwd is used for the juju local deploy
test_cwd_no_git() {
  file="${TEST_DIR}/local-charm-deploy-no-git.txt"

  ensure "local-charm-deploy" "${file}"

  TMP_CHARM_GIT=$(mktemp -d -t ci-XXXXXXXXXX)
  TMP_NO_GIT=$(mktemp -d -t ci-XXXXXXXXXX)
  cd "$TMP_CHARM_GIT" || exit 1

  echo "cloning ntp charm"
  git clone --depth=1 --quiet https://github.com/lampkicking/charm-ntp.git ntp
  cd "$TMP_NO_GIT" || exit 1
  OUTPUT=$(juju deploy "$TMP_CHARM_GIT/ntp" 2>&1)

  ERR_MSG="exit status 128"

  check_not_contains "$OUTPUT" "$ERR_MSG"

  destroy_model "local-charm-deploy"
}

#cwd with git, deploy charm with git, but -> check that git describe is correct
test_cwd_wrong_git() {
  file="${TEST_DIR}/local-charm-deploy-wrong-git.txt"
  ensure "local-charm-deploy-wrong-git" "${file}"

  TMP_CHARM_GIT=$(mktemp -d -t ci-XXXXXXXXXX)
  TMP_NO_CHARM_GIT=$(mktemp -d -t ci-XXXXXXXXXX)

  echo "cloning ntp charm"
  cd "$TMP_CHARM_GIT" || exit 1
  git clone --depth=1 --quiet https://github.com/lampkicking/charm-ntp.git ntp
  cd ntp || exit 1
  WANTED_CHARM_SHA=\"$(git describe --dirty --always)\"

  cd "$TMP_NO_CHARM_GIT" || exit 1
  echo "creating local git folder"
  create_local_git_folder

  juju deploy "$TMP_CHARM_GIT"/ntp ntp

  wait_for "ntp" ".applications | keys[0]"
  CURRENT_CHARM_SHA=$(juju status --format=json | jq '.applications.ntp."charm-version"')
  if [ "$WANTED_CHARM_SHA" = "$CURRENT_CHARM_SHA" ]; then
    echo "Juju status returns the expected sha "
  else
    echo "The expected sha does not equal the current sha "
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
