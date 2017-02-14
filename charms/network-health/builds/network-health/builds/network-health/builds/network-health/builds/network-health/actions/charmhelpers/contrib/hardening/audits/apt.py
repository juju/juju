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

from __future__ import absolute_import  # required for external apt import
from apt import apt_pkg
from six import string_types

from charmhelpers.fetch import (
    apt_cache,
    apt_purge
)
from charmhelpers.core.hookenv import (
    log,
    DEBUG,
    WARNING,
)
from charmhelpers.contrib.hardening.audits import BaseAudit


class AptConfig(BaseAudit):

    def __init__(self, config, **kwargs):
        self.config = config

    def verify_config(self):
        apt_pkg.init()
        for cfg in self.config:
            value = apt_pkg.config.get(cfg['key'], cfg.get('default', ''))
            if value and value != cfg['expected']:
                log("APT config '%s' has unexpected value '%s' "
                    "(expected='%s')" %
                    (cfg['key'], value, cfg['expected']), level=WARNING)

    def ensure_compliance(self):
        self.verify_config()


class RestrictedPackages(BaseAudit):
    """Class used to audit restricted packages on the system."""

    def __init__(self, pkgs, **kwargs):
        super(RestrictedPackages, self).__init__(**kwargs)
        if isinstance(pkgs, string_types) or not hasattr(pkgs, '__iter__'):
            self.pkgs = [pkgs]
        else:
            self.pkgs = pkgs

    def ensure_compliance(self):
        cache = apt_cache()

        for p in self.pkgs:
            if p not in cache:
                continue

            pkg = cache[p]
            if not self.is_virtual_package(pkg):
                if not pkg.current_ver:
                    log("Package '%s' is not installed." % pkg.name,
                        level=DEBUG)
                    continue
                else:
                    log("Restricted package '%s' is installed" % pkg.name,
                        level=WARNING)
                    self.delete_package(cache, pkg)
            else:
                log("Checking restricted virtual package '%s' provides" %
                    pkg.name, level=DEBUG)
                self.delete_package(cache, pkg)

    def delete_package(self, cache, pkg):
        """Deletes the package from the system.

        Deletes the package form the system, properly handling virtual
        packages.

        :param cache: the apt cache
        :param pkg: the package to remove
        """
        if self.is_virtual_package(pkg):
            log("Package '%s' appears to be virtual - purging provides" %
                pkg.name, level=DEBUG)
            for _p in pkg.provides_list:
                self.delete_package(cache, _p[2].parent_pkg)
        elif not pkg.current_ver:
            log("Package '%s' not installed" % pkg.name, level=DEBUG)
            return
        else:
            log("Purging package '%s'" % pkg.name, level=DEBUG)
            apt_purge(pkg.name)

    def is_virtual_package(self, pkg):
        return pkg.has_provides and not pkg.has_versions
