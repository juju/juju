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
    raise ImportError('GitPython does not support Python 3')

try:
    from git import Repo
except ImportError:
    from charmhelpers.fetch import apt_install
    apt_install("python-git")
    from git import Repo

from git.exc import GitCommandError  # noqa E402


class GitUrlFetchHandler(BaseFetchHandler):
    """Handler for git branches via generic and github URLs"""
    def can_handle(self, source):
        url_parts = self.parse_url(source)
        # TODO (mattyw) no support for ssh git@ yet
        if url_parts.scheme not in ('http', 'https', 'git'):
            return False
        else:
            return True

    def clone(self, source, dest, branch):
        if not self.can_handle(source):
            raise UnhandledSource("Cannot handle {}".format(source))

        repo = Repo.clone_from(source, dest)
        repo.git.checkout(branch)

    def install(self, source, branch="master", dest=None):
        url_parts = self.parse_url(source)
        branch_name = url_parts.path.strip("/").split("/")[-1]
        if dest:
            dest_dir = os.path.join(dest, branch_name)
        else:
            dest_dir = os.path.join(os.environ.get('CHARM_DIR'), "fetched",
                                    branch_name)
        if not os.path.exists(dest_dir):
            mkdir(dest_dir, perms=0o755)
        try:
            self.clone(source, dest_dir, branch)
        except GitCommandError as e:
            raise UnhandledSource(e.message)
        except OSError as e:
            raise UnhandledSource(e.strerror)
        return dest_dir
