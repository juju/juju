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

import glob
import grp
import os
import pwd
import six
import yaml

from charmhelpers.core.hookenv import (
    log,
    DEBUG,
    INFO,
    WARNING,
    ERROR,
)


# Global settings cache. Since each hook fire entails a fresh module import it
# is safe to hold this in memory and not risk missing config changes (since
# they will result in a new hook fire and thus re-import).
__SETTINGS__ = {}


def _get_defaults(modules):
    """Load the default config for the provided modules.

    :param modules: stack modules config defaults to lookup.
    :returns: modules default config dictionary.
    """
    default = os.path.join(os.path.dirname(__file__),
                           'defaults/%s.yaml' % (modules))
    return yaml.safe_load(open(default))


def _get_schema(modules):
    """Load the config schema for the provided modules.

    NOTE: this schema is intended to have 1-1 relationship with they keys in
    the default config and is used a means to verify valid overrides provided
    by the user.

    :param modules: stack modules config schema to lookup.
    :returns: modules default schema dictionary.
    """
    schema = os.path.join(os.path.dirname(__file__),
                          'defaults/%s.yaml.schema' % (modules))
    return yaml.safe_load(open(schema))


def _get_user_provided_overrides(modules):
    """Load user-provided config overrides.

    :param modules: stack modules to lookup in user overrides yaml file.
    :returns: overrides dictionary.
    """
    overrides = os.path.join(os.environ['JUJU_CHARM_DIR'],
                             'hardening.yaml')
    if os.path.exists(overrides):
        log("Found user-provided config overrides file '%s'" %
            (overrides), level=DEBUG)
        settings = yaml.safe_load(open(overrides))
        if settings and settings.get(modules):
            log("Applying '%s' overrides" % (modules), level=DEBUG)
            return settings.get(modules)

        log("No overrides found for '%s'" % (modules), level=DEBUG)
    else:
        log("No hardening config overrides file '%s' found in charm "
            "root dir" % (overrides), level=DEBUG)

    return {}


def _apply_overrides(settings, overrides, schema):
    """Get overrides config overlayed onto modules defaults.

    :param modules: require stack modules config.
    :returns: dictionary of modules config with user overrides applied.
    """
    if overrides:
        for k, v in six.iteritems(overrides):
            if k in schema:
                if schema[k] is None:
                    settings[k] = v
                elif type(schema[k]) is dict:
                    settings[k] = _apply_overrides(settings[k], overrides[k],
                                                   schema[k])
                else:
                    raise Exception("Unexpected type found in schema '%s'" %
                                    type(schema[k]), level=ERROR)
            else:
                log("Unknown override key '%s' - ignoring" % (k), level=INFO)

    return settings


def get_settings(modules):
    global __SETTINGS__
    if modules in __SETTINGS__:
        return __SETTINGS__[modules]

    schema = _get_schema(modules)
    settings = _get_defaults(modules)
    overrides = _get_user_provided_overrides(modules)
    __SETTINGS__[modules] = _apply_overrides(settings, overrides, schema)
    return __SETTINGS__[modules]


def ensure_permissions(path, user, group, permissions, maxdepth=-1):
    """Ensure permissions for path.

    If path is a file, apply to file and return. If path is a directory,
    apply recursively (if required) to directory contents and return.

    :param user: user name
    :param group: group name
    :param permissions: octal permissions
    :param maxdepth: maximum recursion depth. A negative maxdepth allows
                     infinite recursion and maxdepth=0 means no recursion.
    :returns: None
    """
    if not os.path.exists(path):
        log("File '%s' does not exist - cannot set permissions" % (path),
            level=WARNING)
        return

    _user = pwd.getpwnam(user)
    os.chown(path, _user.pw_uid, grp.getgrnam(group).gr_gid)
    os.chmod(path, permissions)

    if maxdepth == 0:
        log("Max recursion depth reached - skipping further recursion",
            level=DEBUG)
        return
    elif maxdepth > 0:
        maxdepth -= 1

    if os.path.isdir(path):
        contents = glob.glob("%s/*" % (path))
        for c in contents:
            ensure_permissions(c, user=user, group=group,
                               permissions=permissions, maxdepth=maxdepth)
