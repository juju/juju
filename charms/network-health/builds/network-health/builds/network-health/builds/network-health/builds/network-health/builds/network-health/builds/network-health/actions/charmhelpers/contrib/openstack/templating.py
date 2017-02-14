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

import six

from charmhelpers.fetch import apt_install, apt_update
from charmhelpers.core.hookenv import (
    log,
    ERROR,
    INFO
)
from charmhelpers.contrib.openstack.utils import OPENSTACK_CODENAMES

try:
    from jinja2 import FileSystemLoader, ChoiceLoader, Environment, exceptions
except ImportError:
    apt_update(fatal=True)
    apt_install('python-jinja2', fatal=True)
    from jinja2 import FileSystemLoader, ChoiceLoader, Environment, exceptions


class OSConfigException(Exception):
    pass


def get_loader(templates_dir, os_release):
    """
    Create a jinja2.ChoiceLoader containing template dirs up to
    and including os_release.  If directory template directory
    is missing at templates_dir, it will be omitted from the loader.
    templates_dir is added to the bottom of the search list as a base
    loading dir.

    A charm may also ship a templates dir with this module
    and it will be appended to the bottom of the search list, eg::

        hooks/charmhelpers/contrib/openstack/templates

    :param templates_dir (str): Base template directory containing release
        sub-directories.
    :param os_release (str): OpenStack release codename to construct template
        loader.
    :returns: jinja2.ChoiceLoader constructed with a list of
        jinja2.FilesystemLoaders, ordered in descending
        order by OpenStack release.
    """
    tmpl_dirs = [(rel, os.path.join(templates_dir, rel))
                 for rel in six.itervalues(OPENSTACK_CODENAMES)]

    if not os.path.isdir(templates_dir):
        log('Templates directory not found @ %s.' % templates_dir,
            level=ERROR)
        raise OSConfigException

    # the bottom contains tempaltes_dir and possibly a common templates dir
    # shipped with the helper.
    loaders = [FileSystemLoader(templates_dir)]
    helper_templates = os.path.join(os.path.dirname(__file__), 'templates')
    if os.path.isdir(helper_templates):
        loaders.append(FileSystemLoader(helper_templates))

    for rel, tmpl_dir in tmpl_dirs:
        if os.path.isdir(tmpl_dir):
            loaders.insert(0, FileSystemLoader(tmpl_dir))
        if rel == os_release:
            break
    log('Creating choice loader with dirs: %s' %
        [l.searchpath for l in loaders], level=INFO)
    return ChoiceLoader(loaders)


class OSConfigTemplate(object):
    """
    Associates a config file template with a list of context generators.
    Responsible for constructing a template context based on those generators.
    """
    def __init__(self, config_file, contexts):
        self.config_file = config_file

        if hasattr(contexts, '__call__'):
            self.contexts = [contexts]
        else:
            self.contexts = contexts

        self._complete_contexts = []

    def context(self):
        ctxt = {}
        for context in self.contexts:
            _ctxt = context()
            if _ctxt:
                ctxt.update(_ctxt)
                # track interfaces for every complete context.
                [self._complete_contexts.append(interface)
                 for interface in context.interfaces
                 if interface not in self._complete_contexts]
        return ctxt

    def complete_contexts(self):
        '''
        Return a list of interfaces that have satisfied contexts.
        '''
        if self._complete_contexts:
            return self._complete_contexts
        self.context()
        return self._complete_contexts


class OSConfigRenderer(object):
    """
    This class provides a common templating system to be used by OpenStack
    charms.  It is intended to help charms share common code and templates,
    and ease the burden of managing config templates across multiple OpenStack
    releases.

    Basic usage::

        # import some common context generates from charmhelpers
        from charmhelpers.contrib.openstack import context

        # Create a renderer object for a specific OS release.
        configs = OSConfigRenderer(templates_dir='/tmp/templates',
                                   openstack_release='folsom')
        # register some config files with context generators.
        configs.register(config_file='/etc/nova/nova.conf',
                         contexts=[context.SharedDBContext(),
                                   context.AMQPContext()])
        configs.register(config_file='/etc/nova/api-paste.ini',
                         contexts=[context.IdentityServiceContext()])
        configs.register(config_file='/etc/haproxy/haproxy.conf',
                         contexts=[context.HAProxyContext()])
        # write out a single config
        configs.write('/etc/nova/nova.conf')
        # write out all registered configs
        configs.write_all()

    **OpenStack Releases and template loading**

    When the object is instantiated, it is associated with a specific OS
    release.  This dictates how the template loader will be constructed.

    The constructed loader attempts to load the template from several places
    in the following order:
    - from the most recent OS release-specific template dir (if one exists)
    - the base templates_dir
    - a template directory shipped in the charm with this helper file.

    For the example above, '/tmp/templates' contains the following structure::

        /tmp/templates/nova.conf
        /tmp/templates/api-paste.ini
        /tmp/templates/grizzly/api-paste.ini
        /tmp/templates/havana/api-paste.ini

    Since it was registered with the grizzly release, it first seraches
    the grizzly directory for nova.conf, then the templates dir.

    When writing api-paste.ini, it will find the template in the grizzly
    directory.

    If the object were created with folsom, it would fall back to the
    base templates dir for its api-paste.ini template.

    This system should help manage changes in config files through
    openstack releases, allowing charms to fall back to the most recently
    updated config template for a given release

    The haproxy.conf, since it is not shipped in the templates dir, will
    be loaded from the module directory's template directory, eg
    $CHARM/hooks/charmhelpers/contrib/openstack/templates.  This allows
    us to ship common templates (haproxy, apache) with the helpers.

    **Context generators**

    Context generators are used to generate template contexts during hook
    execution.  Doing so may require inspecting service relations, charm
    config, etc.  When registered, a config file is associated with a list
    of generators.  When a template is rendered and written, all context
    generates are called in a chain to generate the context dictionary
    passed to the jinja2 template. See context.py for more info.
    """
    def __init__(self, templates_dir, openstack_release):
        if not os.path.isdir(templates_dir):
            log('Could not locate templates dir %s' % templates_dir,
                level=ERROR)
            raise OSConfigException

        self.templates_dir = templates_dir
        self.openstack_release = openstack_release
        self.templates = {}
        self._tmpl_env = None

        if None in [Environment, ChoiceLoader, FileSystemLoader]:
            # if this code is running, the object is created pre-install hook.
            # jinja2 shouldn't get touched until the module is reloaded on next
            # hook execution, with proper jinja2 bits successfully imported.
            apt_install('python-jinja2')

    def register(self, config_file, contexts):
        """
        Register a config file with a list of context generators to be called
        during rendering.
        """
        self.templates[config_file] = OSConfigTemplate(config_file=config_file,
                                                       contexts=contexts)
        log('Registered config file: %s' % config_file, level=INFO)

    def _get_tmpl_env(self):
        if not self._tmpl_env:
            loader = get_loader(self.templates_dir, self.openstack_release)
            self._tmpl_env = Environment(loader=loader)

    def _get_template(self, template):
        self._get_tmpl_env()
        template = self._tmpl_env.get_template(template)
        log('Loaded template from %s' % template.filename, level=INFO)
        return template

    def render(self, config_file):
        if config_file not in self.templates:
            log('Config not registered: %s' % config_file, level=ERROR)
            raise OSConfigException
        ctxt = self.templates[config_file].context()

        _tmpl = os.path.basename(config_file)
        try:
            template = self._get_template(_tmpl)
        except exceptions.TemplateNotFound:
            # if no template is found with basename, try looking for it
            # using a munged full path, eg:
            #   /etc/apache2/apache2.conf -> etc_apache2_apache2.conf
            _tmpl = '_'.join(config_file.split('/')[1:])
            try:
                template = self._get_template(_tmpl)
            except exceptions.TemplateNotFound as e:
                log('Could not load template from %s by %s or %s.' %
                    (self.templates_dir, os.path.basename(config_file), _tmpl),
                    level=ERROR)
                raise e

        log('Rendering from template: %s' % _tmpl, level=INFO)
        return template.render(ctxt)

    def write(self, config_file):
        """
        Write a single config file, raises if config file is not registered.
        """
        if config_file not in self.templates:
            log('Config not registered: %s' % config_file, level=ERROR)
            raise OSConfigException

        _out = self.render(config_file)

        with open(config_file, 'wb') as out:
            out.write(_out)

        log('Wrote template %s.' % config_file, level=INFO)

    def write_all(self):
        """
        Write out all registered config files.
        """
        [self.write(k) for k in six.iterkeys(self.templates)]

    def set_release(self, openstack_release):
        """
        Resets the template environment and generates a new template loader
        based on a the new openstack release.
        """
        self._tmpl_env = None
        self.openstack_release = openstack_release
        self._get_tmpl_env()

    def complete_contexts(self):
        '''
        Returns a list of context interfaces that yield a complete context.
        '''
        interfaces = []
        [interfaces.extend(i.complete_contexts())
         for i in six.itervalues(self.templates)]
        return interfaces

    def get_incomplete_context_data(self, interfaces):
        '''
        Return dictionary of relation status of interfaces and any missing
        required context data. Example:
            {'amqp': {'missing_data': ['rabbitmq_password'], 'related': True},
             'zeromq-configuration': {'related': False}}
        '''
        incomplete_context_data = {}

        for i in six.itervalues(self.templates):
            for context in i.contexts:
                for interface in interfaces:
                    related = False
                    if interface in context.interfaces:
                        related = context.get_related()
                        missing_data = context.missing_data
                        if missing_data:
                            incomplete_context_data[interface] = {'missing_data': missing_data}
                        if related:
                            if incomplete_context_data.get(interface):
                                incomplete_context_data[interface].update({'related': True})
                            else:
                                incomplete_context_data[interface] = {'related': True}
                        else:
                            incomplete_context_data[interface] = {'related': False}
        return incomplete_context_data
