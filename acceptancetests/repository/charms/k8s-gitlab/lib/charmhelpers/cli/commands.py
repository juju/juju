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

"""
This module loads sub-modules into the python runtime so they can be
discovered via the inspect module. In order to prevent flake8 from (rightfully)
telling us these are unused modules, throw a ' # noqa' at the end of each import
so that the warning is suppressed.
"""

from . import CommandLine  # noqa

"""
Import the sub-modules which have decorated subcommands to register with chlp.
"""
from . import host  # noqa
from . import benchmark  # noqa
from . import unitdata  # noqa
from . import hookenv  # noqa
