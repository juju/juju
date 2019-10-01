#checks whether the cwd is used for the juju local deploy
test_cwd_no_git() {
  file="${1}"
  ensure "local-charm-deploy" "${file}"

  TMP_CHARM_GIT=$(mktemp -d -t ci-XXXXXXXXXX)
  TMP_NO_GIT=$(mktemp -d -t ci-XXXXXXXXXX)
  cd "$TMP_CHARM_GIT" || exit 1

  echo "cloning ntp charm"
  git clone --depth=1 --quiet https://git.launchpad.net/ntp-charm ntp
  cd "$TMP_NO_GIT" || exit 1
  OUTPUT=$(juju deploy "$TMP_CHARM_GIT/ntp" 2>&1)

  ERR_MSG="exit status 128"

  check_not_contains "$OUTPUT" "$ERR_MSG"

  destroy_model "local-charm-deploy"
}

#2. cwd with git, deploy charm with git, but -> check that git describe is correct
#3. cwd no git, deploy charm with child no git, but parent -> check no warning

create_local_git_folder() {
  echo "creating local git folder"
  cd "$1" || exit 1
  git init .
  touch rand_file
  git add rand_file
  git commit -am "rand_file"
}
