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
SHASUM=$(shasum ~/Downloads/$TARBALL | cut -d ' ' -f 1)
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
        sed -e "s,^  url '.*juju-core.*.tar.gz',  url '$URI',;" |
        sed -e "s,^  sha1 '.*',  sha1 '$SHASUM',;")
    MESSAGE="Update juju stable $VERSION."
    OPT=""
else
    # The development block is indented 4 spaces.
    FORMULA=$(cat juju.rb |
        sed -e "/devel do/,/bottle do/ s,^    url [\'\"].*juju-core.*.tar.gz[\'\"],    url '$URI',;" |
        sed -e "/devel do/,/bottle do/s,^    sha1 [\'\"].*[\'\"],    sha1 '$SHASUM',;")
    MESSAGE="Update juju devel $VERSION."
    OPT="--devel"
fi
echo "$FORMULA" > juju.rb
git commit -m "$MESSAGE" juju.rb
brew install $OPT juju
juju --version
echo "You can push this to github (fork, sinzui/homebrew)."
echo "  git push fork juju-$VERSION"
