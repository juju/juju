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

from charmhelpers.contrib.hardening.utils import get_settings
from charmhelpers.contrib.hardening.audits.apt import (
    AptConfig,
    RestrictedPackages,
)


def get_audits():
    """Get OS hardening apt audits.

    :returns:  dictionary of audits
    """
    audits = [AptConfig([{'key': 'APT::Get::AllowUnauthenticated',
                          'expected': 'false'}])]

    settings = get_settings('os')
    clean_packages = settings['security']['packages_clean']
    if clean_packages:
        security_packages = settings['security']['packages_list']
        if security_packages:
            audits.append(RestrictedPackages(security_packages))

    return audits
