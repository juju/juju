#!/bin/bash

#  inc-juju.bash
#  
#
#  Created by Curtis on 12/11/13.

set -eux

GIT_USER="sinzui"

PURPOSE=$1
URI=$2
TARBALL=$(basename $URI)
VERSION=$(basename $TARBALL .tar.gz | cut -d _ -f 2)
cd ~/Downloads
curl -L -o ~/Downloads/$TARBALL $URI
SHASUM=$(shasum -a 256 ~/Downloads/$TARBALL | cut -d ' ' -f 1)
echo "$TARBALL is $SHASUM"

cd /usr/local/Library/Formula
git checkout master
brew update
brew uninstall --force juju
git rebase origin/master
git checkout -b juju-$VERSION
if [[ $PURPOSE == 'stable' ]]; then
    # The stable block is indented 2 spaces.
    FORMULA=$(cat juju.rb |
        sed -e "s,^  url \".*juju-core.*.tar.gz\",  url \"$URI\",;" |
        sed -e "s,^  sha256 \".*\",  sha256 \"$SHASUM\",;")
    OPT=""
else
    # The development block is indented 4 spaces.
    FORMULA=$(cat juju.rb |
        sed -e "/devel do/,/bottle do/ s,^    url [\'\"].*juju-core.*.tar.gz[\'\"],    url \"$URI\",;" |
        sed -e "/devel do/,/bottle do/s,^    sha256 [\'\"].*[\'\"],    sha256 \"$SHASUM\",;")
    OPT="--devel"
fi
echo "$FORMULA" > juju.rb
brew audit juju
brew audit --online --strict juju
git commit -m "juju $VERSION" juju.rb
brew install $OPT juju
juju --version

echo "You can push this to github (fork, sinzui/homebrew)."
echo "  git push fork juju-$VERSION"
