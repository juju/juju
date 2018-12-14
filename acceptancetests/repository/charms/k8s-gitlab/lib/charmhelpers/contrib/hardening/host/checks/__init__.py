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

from charmhelpers.core.hookenv import (
    log,
    DEBUG,
)
from charmhelpers.contrib.hardening.host.checks import (
    apt,
    limits,
    login,
    minimize_access,
    pam,
    profile,
    securetty,
    suid_sgid,
    sysctl
)


def run_os_checks():
    log("Starting OS hardening checks.", level=DEBUG)
    checks = apt.get_audits()
    checks.extend(limits.get_audits())
    checks.extend(login.get_audits())
    checks.extend(minimize_access.get_audits())
    checks.extend(pam.get_audits())
    checks.extend(profile.get_audits())
    checks.extend(securetty.get_audits())
    checks.extend(suid_sgid.get_audits())
    checks.extend(sysctl.get_audits())

    for check in checks:
        log("Running '%s' check" % (check.__class__.__name__), level=DEBUG)
        check.ensure_compliance()

    log("OS hardening checks complete.", level=DEBUG)
