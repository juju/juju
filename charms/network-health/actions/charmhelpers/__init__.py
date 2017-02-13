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

# Bootstrap charm-helpers, installing its dependencies if necessary using
# only standard libraries.
import subprocess
import sys

try:
    import six  # flake8: noqa
except ImportError:
    if sys.version_info.major == 2:
        subprocess.check_call(['apt-get', 'install', '-y', 'python-six'])
    else:
        subprocess.check_call(['apt-get', 'install', '-y', 'python3-six'])
    import six  # flake8: noqa

try:
    import yaml  # flake8: noqa
except ImportError:
    if sys.version_info.major == 2:
        subprocess.check_call(['apt-get', 'install', '-y', 'python-yaml'])
    else:
        subprocess.check_call(['apt-get', 'install', '-y', 'python3-yaml'])
    import yaml  # flake8: noqa
