#!/bin/bash

#  inc-juju.bash
#  
#
#  Created by Curtis on 12/11/13.

set -eux

GIT_USER="sinzui"

URI=$1
TARBALL=$(basename $URI)
VERSION=$(basename $TARBALL .tar.gz | cut -d _ -f 2)
cd ~/Downloads
curl -L -o ~/Downloads/$TARBALL $URI
SHASUM=$(shasum ~/Downloads/$TARBALL | cut -d ' ' -f 1)
echo "$TARBALL is $SHASUM"

cd /usr/local/Library/Formula
git checkout master
brew update
brew uninstall juju
git rebase origin/master
git checkout -b juju-$VERSION
FORMULA=$(sed -e "s,^  url '.*juju-core.*.tar.gz',  url '$URI',; s,^  sha1 '.*',  sha1 '$SHASUM',;" juju.rb)
echo "$FORMULA" > juju.rb
git commit -m "Update juju $VERSION." juju.rb
brew install juju
juju --version
echo "You can push this to github (fork, sinzui/homebrew)."
echo "  git push fork juju-$VERSION"