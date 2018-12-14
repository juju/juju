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

import importlib
from charmhelpers.osplatform import get_platform
from yaml import safe_load
from charmhelpers.core.hookenv import (
    config,
    log,
)

import six
if six.PY3:
    from urllib.parse import urlparse, urlunparse
else:
    from urlparse import urlparse, urlunparse


# The order of this list is very important. Handlers should be listed in from
# least- to most-specific URL matching.
FETCH_HANDLERS = (
    'charmhelpers.fetch.archiveurl.ArchiveUrlFetchHandler',
    'charmhelpers.fetch.bzrurl.BzrUrlFetchHandler',
    'charmhelpers.fetch.giturl.GitUrlFetchHandler',
)


class SourceConfigError(Exception):
    pass


class UnhandledSource(Exception):
    pass


class AptLockError(Exception):
    pass


class GPGKeyError(Exception):
    """Exception occurs when a GPG key cannot be fetched or used.  The message
    indicates what the problem is.
    """
    pass


class BaseFetchHandler(object):

    """Base class for FetchHandler implementations in fetch plugins"""

    def can_handle(self, source):
        """Returns True if the source can be handled. Otherwise returns
        a string explaining why it cannot"""
        return "Wrong source type"

    def install(self, source):
        """Try to download and unpack the source. Return the path to the
        unpacked files or raise UnhandledSource."""
        raise UnhandledSource("Wrong source type {}".format(source))

    def parse_url(self, url):
        return urlparse(url)

    def base_url(self, url):
        """Return url without querystring or fragment"""
        parts = list(self.parse_url(url))
        parts[4:] = ['' for i in parts[4:]]
        return urlunparse(parts)


__platform__ = get_platform()
module = "charmhelpers.fetch.%s" % __platform__
fetch = importlib.import_module(module)

filter_installed_packages = fetch.filter_installed_packages
filter_missing_packages = fetch.filter_missing_packages
install = fetch.apt_install
upgrade = fetch.apt_upgrade
update = _fetch_update = fetch.apt_update
purge = fetch.apt_purge
add_source = fetch.add_source

if __platform__ == "ubuntu":
    apt_cache = fetch.apt_cache
    apt_install = fetch.apt_install
    apt_update = fetch.apt_update
    apt_upgrade = fetch.apt_upgrade
    apt_purge = fetch.apt_purge
    apt_autoremove = fetch.apt_autoremove
    apt_mark = fetch.apt_mark
    apt_hold = fetch.apt_hold
    apt_unhold = fetch.apt_unhold
    import_key = fetch.import_key
    get_upstream_version = fetch.get_upstream_version
elif __platform__ == "centos":
    yum_search = fetch.yum_search


def configure_sources(update=False,
                      sources_var='install_sources',
                      keys_var='install_keys'):
    """Configure multiple sources from charm configuration.

    The lists are encoded as yaml fragments in the configuration.
    The fragment needs to be included as a string. Sources and their
    corresponding keys are of the types supported by add_source().

    Example config:
        install_sources: |
          - "ppa:foo"
          - "http://example.com/repo precise main"
        install_keys: |
          - null
          - "a1b2c3d4"

    Note that 'null' (a.k.a. None) should not be quoted.
    """
    sources = safe_load((config(sources_var) or '').strip()) or []
    keys = safe_load((config(keys_var) or '').strip()) or None

    if isinstance(sources, six.string_types):
        sources = [sources]

    if keys is None:
        for source in sources:
            add_source(source, None)
    else:
        if isinstance(keys, six.string_types):
            keys = [keys]

        if len(sources) != len(keys):
            raise SourceConfigError(
                'Install sources and keys lists are different lengths')
        for source, key in zip(sources, keys):
            add_source(source, key)
    if update:
        _fetch_update(fatal=True)


def install_remote(source, *args, **kwargs):
    """Install a file tree from a remote source.

    The specified source should be a url of the form:
        scheme://[host]/path[#[option=value][&...]]

    Schemes supported are based on this modules submodules.
    Options supported are submodule-specific.
    Additional arguments are passed through to the submodule.

    For example::

        dest = install_remote('http://example.com/archive.tgz',
                              checksum='deadbeef',
                              hash_type='sha1')

    This will download `archive.tgz`, validate it using SHA1 and, if
    the file is ok, extract it and return the directory in which it
    was extracted.  If the checksum fails, it will raise
    :class:`charmhelpers.core.host.ChecksumError`.
    """
    # We ONLY check for True here because can_handle may return a string
    # explaining why it can't handle a given source.
    handlers = [h for h in plugins() if h.can_handle(source) is True]
    for handler in handlers:
        try:
            return handler.install(source, *args, **kwargs)
        except UnhandledSource as e:
            log('Install source attempt unsuccessful: {}'.format(e),
                level='WARNING')
    raise UnhandledSource("No handler found for source {}".format(source))


def install_from_config(config_var_name):
    """Install a file from config."""
    charm_config = config()
    source = charm_config[config_var_name]
    return install_remote(source)


def plugins(fetch_handlers=None):
    if not fetch_handlers:
        fetch_handlers = FETCH_HANDLERS
    plugin_list = []
    for handler_name in fetch_handlers:
        package, classname = handler_name.rsplit('.', 1)
        try:
            handler_class = getattr(
                importlib.import_module(package),
                classname)
            plugin_list.append(handler_class())
        except NotImplementedError:
            # Skip missing plugins so that they can be ommitted from
            # installation if desired
            log("FetchHandler {} not found, skipping plugin".format(
                handler_name))
    return plugin_list
