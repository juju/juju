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
import sys

from charmhelpers.core import host
from charmhelpers.core import hookenv


def render(source, target, context, owner='root', group='root',
           perms=0o444, templates_dir=None, encoding='UTF-8',
           template_loader=None, config_template=None):
    """
    Render a template.

    The `source` path, if not absolute, is relative to the `templates_dir`.

    The `target` path should be absolute.  It can also be `None`, in which
    case no file will be written.

    The context should be a dict containing the values to be replaced in the
    template.

    config_template may be provided to render from a provided template instead
    of loading from a file.

    The `owner`, `group`, and `perms` options will be passed to `write_file`.

    If omitted, `templates_dir` defaults to the `templates` folder in the charm.

    The rendered template will be written to the file as well as being returned
    as a string.

    Note: Using this requires python-jinja2 or python3-jinja2; if it is not
    installed, calling this will attempt to use charmhelpers.fetch.apt_install
    to install it.
    """
    try:
        from jinja2 import FileSystemLoader, Environment, exceptions
    except ImportError:
        try:
            from charmhelpers.fetch import apt_install
        except ImportError:
            hookenv.log('Could not import jinja2, and could not import '
                        'charmhelpers.fetch to install it',
                        level=hookenv.ERROR)
            raise
        if sys.version_info.major == 2:
            apt_install('python-jinja2', fatal=True)
        else:
            apt_install('python3-jinja2', fatal=True)
        from jinja2 import FileSystemLoader, Environment, exceptions

    if template_loader:
        template_env = Environment(loader=template_loader)
    else:
        if templates_dir is None:
            templates_dir = os.path.join(hookenv.charm_dir(), 'templates')
        template_env = Environment(loader=FileSystemLoader(templates_dir))

    # load from a string if provided explicitly
    if config_template is not None:
        template = template_env.from_string(config_template)
    else:
        try:
            source = source
            template = template_env.get_template(source)
        except exceptions.TemplateNotFound as e:
            hookenv.log('Could not load template %s from %s.' %
                        (source, templates_dir),
                        level=hookenv.ERROR)
            raise e
    content = template.render(context)
    if target is not None:
        target_dir = os.path.dirname(target)
        if not os.path.exists(target_dir):
            # This is a terrible default directory permission, as the file
            # or its siblings will often contain secrets.
            host.mkdir(os.path.dirname(target), owner, group, perms=0o755)
        host.write_file(target, content.encode(encoding), owner, group, perms)
    return content
