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
from subprocess import check_call, CalledProcessError
from charmhelpers.fetch import (
    BaseFetchHandler,
    UnhandledSource,
    filter_installed_packages,
    install,
)

if filter_installed_packages(['git']) != []:
    install(['git'])
    if filter_installed_packages(['git']) != []:
        raise NotImplementedError('Unable to install git')


class GitUrlFetchHandler(BaseFetchHandler):
    """Handler for git branches via generic and github URLs."""

    def can_handle(self, source):
        url_parts = self.parse_url(source)
        # TODO (mattyw) no support for ssh git@ yet
        if url_parts.scheme not in ('http', 'https', 'git', ''):
            return False
        elif not url_parts.scheme:
            return os.path.exists(os.path.join(source, '.git'))
        else:
            return True

    def clone(self, source, dest, branch="master", depth=None):
        if not self.can_handle(source):
            raise UnhandledSource("Cannot handle {}".format(source))

        if os.path.exists(dest):
            cmd = ['git', '-C', dest, 'pull', source, branch]
        else:
            cmd = ['git', 'clone', source, dest, '--branch', branch]
            if depth:
                cmd.extend(['--depth', depth])
        check_call(cmd)

    def install(self, source, branch="master", dest=None, depth=None):
        url_parts = self.parse_url(source)
        branch_name = url_parts.path.strip("/").split("/")[-1]
        if dest:
            dest_dir = os.path.join(dest, branch_name)
        else:
            dest_dir = os.path.join(os.environ.get('CHARM_DIR'), "fetched",
                                    branch_name)
        try:
            self.clone(source, dest_dir, branch, depth)
        except CalledProcessError as e:
            raise UnhandledSource(e)
        except OSError as e:
            raise UnhandledSource(e.strerror)
        return dest_dir
