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

import os
import six

from charmhelpers.core.hookenv import (
    log,
    DEBUG,
    WARNING,
)

try:
    from jinja2 import FileSystemLoader, Environment
except ImportError:
    from charmhelpers.fetch import apt_install
    from charmhelpers.fetch import apt_update
    apt_update(fatal=True)
    if six.PY2:
        apt_install('python-jinja2', fatal=True)
    else:
        apt_install('python3-jinja2', fatal=True)
    from jinja2 import FileSystemLoader, Environment


# NOTE: function separated from main rendering code to facilitate easier
#       mocking in unit tests.
def write(path, data):
    with open(path, 'wb') as out:
        out.write(data)


def get_template_path(template_dir, path):
    """Returns the template file which would be used to render the path.

    The path to the template file is returned.
    :param template_dir: the directory the templates are located in
    :param path: the file path to be written to.
    :returns: path to the template file
    """
    return os.path.join(template_dir, os.path.basename(path))


def render_and_write(template_dir, path, context):
    """Renders the specified template into the file.

    :param template_dir: the directory to load the template from
    :param path: the path to write the templated contents to
    :param context: the parameters to pass to the rendering engine
    """
    env = Environment(loader=FileSystemLoader(template_dir))
    template_file = os.path.basename(path)
    template = env.get_template(template_file)
    log('Rendering from template: %s' % template.name, level=DEBUG)
    rendered_content = template.render(context)
    if not rendered_content:
        log("Render returned None - skipping '%s'" % path,
            level=WARNING)
        return

    write(path, rendered_content.encode('utf-8').strip())
    log('Wrote template %s' % path, level=DEBUG)
