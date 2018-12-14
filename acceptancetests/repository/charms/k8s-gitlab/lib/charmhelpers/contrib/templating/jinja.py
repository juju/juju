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
Templating using the python-jinja2 package.
"""
import six
from charmhelpers.fetch import apt_install, apt_update
try:
    import jinja2
except ImportError:
    apt_update(fatal=True)
    if six.PY3:
        apt_install(["python3-jinja2"], fatal=True)
    else:
        apt_install(["python-jinja2"], fatal=True)
    import jinja2


DEFAULT_TEMPLATES_DIR = 'templates'


def render(template_name, context, template_dir=DEFAULT_TEMPLATES_DIR):
    templates = jinja2.Environment(
        loader=jinja2.FileSystemLoader(template_dir))
    template = templates.get_template(template_name)
    return template.render(context)
