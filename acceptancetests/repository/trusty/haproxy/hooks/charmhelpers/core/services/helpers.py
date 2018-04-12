# Copyright 2014-2015 Canonical Limited.
#
# This file is part of charm-helpers.
#
# charm-helpers is free software: you can redistribute it and/or modify
# it under the terms of the GNU Lesser General Public License version 3 as
# published by the Free Software Foundation.
#
# charm-helpers is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU Lesser General Public License for more details.
#
# You should have received a copy of the GNU Lesser General Public License
# along with charm-helpers.  If not, see <http://www.gnu.org/licenses/>.

import os
import yaml
from charmhelpers.core import hookenv
from charmhelpers.core import templating

from charmhelpers.core.services.base import ManagerCallback


__all__ = ['RelationContext', 'TemplateCallback',
           'render_template', 'template']


class RelationContext(dict):
    """
    Base class for a context generator that gets relation data from juju.

    Subclasses must provide the attributes `name`, which is the name of the
    interface of interest, `interface`, which is the type of the interface of
    interest, and `required_keys`, which is the set of keys required for the
    relation to be considered complete.  The data for all interfaces matching
    the `name` attribute that are complete will used to populate the dictionary
    values (see `get_data`, below).

    The generated context will be namespaced under the relation :attr:`name`,
    to prevent potential naming conflicts.

    :param str name: Override the relation :attr:`name`, since it can vary from charm to charm
    :param list additional_required_keys: Extend the list of :attr:`required_keys`
    """
    name = None
    interface = None

    def __init__(self, name=None, additional_required_keys=None):
        if not hasattr(self, 'required_keys'):
            self.required_keys = []

        if name is not None:
            self.name = name
        if additional_required_keys:
            self.required_keys.extend(additional_required_keys)
        self.get_data()

    def __bool__(self):
        """
        Returns True if all of the required_keys are available.
        """
        return self.is_ready()

    __nonzero__ = __bool__

    def __repr__(self):
        return super(RelationContext, self).__repr__()

    def is_ready(self):
        """
        Returns True if all of the `required_keys` are available from any units.
        """
        ready = len(self.get(self.name, [])) > 0
        if not ready:
            hookenv.log('Incomplete relation: {}'.format(self.__class__.__name__), hookenv.DEBUG)
        return ready

    def _is_ready(self, unit_data):
        """
        Helper method that tests a set of relation data and returns True if
        all of the `required_keys` are present.
        """
        return set(unit_data.keys()).issuperset(set(self.required_keys))

    def get_data(self):
        """
        Retrieve the relation data for each unit involved in a relation and,
        if complete, store it in a list under `self[self.name]`.  This
        is automatically called when the RelationContext is instantiated.

        The units are sorted lexographically first by the service ID, then by
        the unit ID.  Thus, if an interface has two other services, 'db:1'
        and 'db:2', with 'db:1' having two units, 'wordpress/0' and 'wordpress/1',
        and 'db:2' having one unit, 'mediawiki/0', all of which have a complete
        set of data, the relation data for the units will be stored in the
        order: 'wordpress/0', 'wordpress/1', 'mediawiki/0'.

        If you only care about a single unit on the relation, you can just
        access it as `{{ interface[0]['key'] }}`.  However, if you can at all
        support multiple units on a relation, you should iterate over the list,
        like::

            {% for unit in interface -%}
                {{ unit['key'] }}{% if not loop.last %},{% endif %}
            {%- endfor %}

        Note that since all sets of relation data from all related services and
        units are in a single list, if you need to know which service or unit a
        set of data came from, you'll need to extend this class to preserve
        that information.
        """
        if not hookenv.relation_ids(self.name):
            return

        ns = self.setdefault(self.name, [])
        for rid in sorted(hookenv.relation_ids(self.name)):
            for unit in sorted(hookenv.related_units(rid)):
                reldata = hookenv.relation_get(rid=rid, unit=unit)
                if self._is_ready(reldata):
                    ns.append(reldata)

    def provide_data(self):
        """
        Return data to be relation_set for this interface.
        """
        return {}


class MysqlRelation(RelationContext):
    """
    Relation context for the `mysql` interface.

    :param str name: Override the relation :attr:`name`, since it can vary from charm to charm
    :param list additional_required_keys: Extend the list of :attr:`required_keys`
    """
    name = 'db'
    interface = 'mysql'

    def __init__(self, *args, **kwargs):
        self.required_keys = ['host', 'user', 'password', 'database']
        RelationContext.__init__(self, *args, **kwargs)


class HttpRelation(RelationContext):
    """
    Relation context for the `http` interface.

    :param str name: Override the relation :attr:`name`, since it can vary from charm to charm
    :param list additional_required_keys: Extend the list of :attr:`required_keys`
    """
    name = 'website'
    interface = 'http'

    def __init__(self, *args, **kwargs):
        self.required_keys = ['host', 'port']
        RelationContext.__init__(self, *args, **kwargs)

    def provide_data(self):
        return {
            'host': hookenv.unit_get('private-address'),
            'port': 80,
        }


class RequiredConfig(dict):
    """
    Data context that loads config options with one or more mandatory options.

    Once the required options have been changed from their default values, all
    config options will be available, namespaced under `config` to prevent
    potential naming conflicts (for example, between a config option and a
    relation property).

    :param list *args: List of options that must be changed from their default values.
    """

    def __init__(self, *args):
        self.required_options = args
        self['config'] = hookenv.config()
        with open(os.path.join(hookenv.charm_dir(), 'config.yaml')) as fp:
            self.config = yaml.load(fp).get('options', {})

    def __bool__(self):
        for option in self.required_options:
            if option not in self['config']:
                return False
            current_value = self['config'][option]
            default_value = self.config[option].get('default')
            if current_value == default_value:
                return False
            if current_value in (None, '') and default_value in (None, ''):
                return False
        return True

    def __nonzero__(self):
        return self.__bool__()


class StoredContext(dict):
    """
    A data context that always returns the data that it was first created with.

    This is useful to do a one-time generation of things like passwords, that
    will thereafter use the same value that was originally generated, instead
    of generating a new value each time it is run.
    """
    def __init__(self, file_name, config_data):
        """
        If the file exists, populate `self` with the data from the file.
        Otherwise, populate with the given data and persist it to the file.
        """
        if os.path.exists(file_name):
            self.update(self.read_context(file_name))
        else:
            self.store_context(file_name, config_data)
            self.update(config_data)

    def store_context(self, file_name, config_data):
        if not os.path.isabs(file_name):
            file_name = os.path.join(hookenv.charm_dir(), file_name)
        with open(file_name, 'w') as file_stream:
            os.fchmod(file_stream.fileno(), 0o600)
            yaml.dump(config_data, file_stream)

    def read_context(self, file_name):
        if not os.path.isabs(file_name):
            file_name = os.path.join(hookenv.charm_dir(), file_name)
        with open(file_name, 'r') as file_stream:
            data = yaml.load(file_stream)
            if not data:
                raise OSError("%s is empty" % file_name)
            return data


class TemplateCallback(ManagerCallback):
    """
    Callback class that will render a Jinja2 template, for use as a ready
    action.

    :param str source: The template source file, relative to
    `$CHARM_DIR/templates`

    :param str target: The target to write the rendered template to
    :param str owner: The owner of the rendered file
    :param str group: The group of the rendered file
    :param int perms: The permissions of the rendered file
    """
    def __init__(self, source, target,
                 owner='root', group='root', perms=0o444):
        self.source = source
        self.target = target
        self.owner = owner
        self.group = group
        self.perms = perms

    def __call__(self, manager, service_name, event_name):
        service = manager.get_service(service_name)
        context = {}
        for ctx in service.get('required_data', []):
            context.update(ctx)
        templating.render(self.source, self.target, context,
                          self.owner, self.group, self.perms)


# Convenience aliases for templates
render_template = template = TemplateCallback
