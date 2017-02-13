# Copyright 2016 Canonical Limited.
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

from charmhelpers.contrib.hardening.audits.file import (
    FilePermissionAudit,
    ReadOnly,
)
from charmhelpers.contrib.hardening import utils


def get_audits():
    """Get OS hardening access audits.

    :returns:  dictionary of audits
    """
    audits = []
    settings = utils.get_settings('os')

    # Remove write permissions from $PATH folders for all regular users.
    # This prevents changing system-wide commands from normal users.
    path_folders = {'/usr/local/sbin',
                    '/usr/local/bin',
                    '/usr/sbin',
                    '/usr/bin',
                    '/bin'}
    extra_user_paths = settings['environment']['extra_user_paths']
    path_folders.update(extra_user_paths)
    audits.append(ReadOnly(path_folders))

    # Only allow the root user to have access to the shadow file.
    audits.append(FilePermissionAudit('/etc/shadow', 'root', 'root', 0o0600))

    if 'change_user' not in settings['security']['users_allow']:
        # su should only be accessible to user and group root, unless it is
        # expressly defined to allow users to change to root via the
        # security_users_allow config option.
        audits.append(FilePermissionAudit('/bin/su', 'root', 'root', 0o750))

    return audits
