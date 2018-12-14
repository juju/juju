# Copyright 2014-2015 Canonical Limited.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#  http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

import os
from subprocess import STDOUT, check_output
from charmhelpers.fetch import (
    BaseFetchHandler,
    UnhandledSource,
    filter_installed_packages,
    install,
)
from charmhelpers.core.host import mkdir


if filter_installed_packages(['bzr']) != []:
    install(['bzr'])
    if filter_installed_packages(['bzr']) != []:
        raise NotImplementedError('Unable to install bzr')


class BzrUrlFetchHandler(BaseFetchHandler):
    """Handler for bazaar branches via generic and lp URLs."""

    def can_handle(self, source):
        url_parts = self.parse_url(source)
        if url_parts.scheme not in ('bzr+ssh', 'lp', ''):
            return False
        elif not url_parts.scheme:
            return os.path.exists(os.path.join(source, '.bzr'))
        else:
            return True

    def branch(self, source, dest, revno=None):
        if not self.can_handle(source):
            raise UnhandledSource("Cannot handle {}".format(source))
        cmd_opts = []
        if revno:
            cmd_opts += ['-r', str(revno)]
        if os.path.exists(dest):
            cmd = ['bzr', 'pull']
            cmd += cmd_opts
            cmd += ['--overwrite', '-d', dest, source]
        else:
            cmd = ['bzr', 'branch']
            cmd += cmd_opts
            cmd += [source, dest]
        check_output(cmd, stderr=STDOUT)

    def install(self, source, dest=None, revno=None):
        url_parts = self.parse_url(source)
        branch_name = url_parts.path.strip("/").split("/")[-1]
        if dest:
            dest_dir = os.path.join(dest, branch_name)
        else:
            dest_dir = os.path.join(os.environ.get('CHARM_DIR'), "fetched",
                                    branch_name)

        if dest and not os.path.exists(dest):
            mkdir(dest, perms=0o755)

        try:
            self.branch(source, dest_dir, revno)
        except OSError as e:
            raise UnhandledSource(e.strerror)
        return dest_dir
