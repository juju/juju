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

''' Helper for managing alternatives for file conflict resolution '''

import subprocess
import shutil
import os


def install_alternative(name, target, source, priority=50):
    ''' Install alternative configuration '''
    if (os.path.exists(target) and not os.path.islink(target)):
        # Move existing file/directory away before installing
        shutil.move(target, '{}.bak'.format(target))
    cmd = [
        'update-alternatives', '--force', '--install',
        target, name, source, str(priority)
    ]
    subprocess.check_call(cmd)
