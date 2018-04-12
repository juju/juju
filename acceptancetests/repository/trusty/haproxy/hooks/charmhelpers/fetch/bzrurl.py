# Copyright 2014-2015 Canonical Limited.
#
# This file is part of charm-helpers.
#
# charm-helpers is free software: you can redistribute it and/or modify
# it under the terms of the GNU Lesser General Public License version 3 as
# published by the Free Software Foundation.
#
# charm-helpers is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU Lesser General Public License for more details.
#
# You should have received a copy of the GNU Lesser General Public License
# along with charm-helpers.  If not, see <http://www.gnu.org/licenses/>.

import os
from charmhelpers.fetch import (
    BaseFetchHandler,
    UnhandledSource
)
from charmhelpers.core.host import mkdir

import six
if six.PY3:
    raise ImportError('bzrlib does not support Python3')

try:
    from bzrlib.branch import Branch
    from bzrlib import bzrdir, workingtree, errors
except ImportError:
    from charmhelpers.fetch import apt_install
    apt_install("python-bzrlib")
    from bzrlib.branch import Branch
    from bzrlib import bzrdir, workingtree, errors


class BzrUrlFetchHandler(BaseFetchHandler):
    """Handler for bazaar branches via generic and lp URLs"""
    def can_handle(self, source):
        url_parts = self.parse_url(source)
        if url_parts.scheme not in ('bzr+ssh', 'lp'):
            return False
        else:
            return True

    def branch(self, source, dest):
        url_parts = self.parse_url(source)
        # If we use lp:branchname scheme we need to load plugins
        if not self.can_handle(source):
            raise UnhandledSource("Cannot handle {}".format(source))
        if url_parts.scheme == "lp":
            from bzrlib.plugin import load_plugins
            load_plugins()
        try:
            local_branch = bzrdir.BzrDir.create_branch_convenience(dest)
        except errors.AlreadyControlDirError:
            local_branch = Branch.open(dest)
        try:
            remote_branch = Branch.open(source)
            remote_branch.push(local_branch)
            tree = workingtree.WorkingTree.open(dest)
            tree.update()
        except Exception as e:
            raise e

    def install(self, source):
        url_parts = self.parse_url(source)
        branch_name = url_parts.path.strip("/").split("/")[-1]
        dest_dir = os.path.join(os.environ.get('CHARM_DIR'), "fetched",
                                branch_name)
        if not os.path.exists(dest_dir):
            mkdir(dest_dir, perms=0o755)
        try:
            self.branch(source, dest_dir)
        except OSError as e:
            raise UnhandledSource(e.strerror)
        return dest_dir
