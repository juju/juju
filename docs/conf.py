import ast
import datetime
import os
import re
import shutil
import subprocess
import sys

sys.path.append('./')

# Configuration for the Sphinx documentation builder.
# All configuration specific to your project should be done in this file.
#
# If you're new to Sphinx and don't want any advanced or custom features,
# just go through the items marked 'TODO'.
#
# A complete list of built-in Sphinx configuration values:
# https://www.sphinx-doc.org/en/master/usage/configuration.html
#
# Our starter pack uses the custom Canonical Sphinx extension
# to keep all documentation based on it consistent and on brand:
# https://github.com/canonical/canonical-sphinx


#######################
# Project information #
#######################

# Project name
#
# TODO: Update with the official name of your project or product

project = "Juju"
author = "Canonical Ltd."


# Sidebar documentation title; best kept reasonably short
#
# TODO: To include a version number, add it here (hardcoded or automated).
#
# TODO: To disable the title, set to an empty string.

html_title = project + " documentation"


# Copyright string; shown at the bottom of the page
#
# Now, the starter pack uses CC-BY-SA as the license
# and the current year as the copyright year.
#
# TODO: If your docs need another license, specify it instead of 'CC-BY-SA'.
#
# TODO: If your documentation is a part of the code repository of your project,
#       it inherits the code license instead; specify it instead of 'CC-BY-SA'.
#
# NOTE: For static works, it is common to provide the first publication year.
#       Another option is to provide both the first year of publication
#       and the current year, especially for docs that frequently change,
#       e.g. 2022–2023 (note the en-dash).
#
#       A way to check a repo's creation date is to get a classic GitHub token
#       with 'repo' permissions; see https://github.com/settings/tokens
#       Next, use 'curl' and 'jq' to extract the date from the API's output:
#
#       curl -H 'Authorization: token <TOKEN>' \
#         -H 'Accept: application/vnd.github.v3.raw' \
#         https://api.github.com/repos/canonical/<REPO> | jq '.created_at'

copyright = "%s CC-BY-SA, %s" % (datetime.date.today().year, author)


# Documentation website URL
#
# TODO: Update with the official URL of your docs or leave empty if unsure.
#
# NOTE: The Open Graph Protocol (OGP) enhances page display in a social graph
#       and is used by social media platforms; see https://ogp.me/

ogp_site_url = "https://canonical-starter-pack.readthedocs-hosted.com/"


# Preview name of the documentation website
#
# TODO: To use a different name for the project in previews, update as needed.

ogp_site_name = project


# Preview image URL
#
# TODO: To customise the preview image, update as needed.

ogp_image = \
    "https://assets.ubuntu.com/v1/253da317-image-document-ubuntudocs.svg"


# Product favicon; shown in bookmarks, browser tabs, etc.

# TODO: To customise the favicon, uncomment and update as needed.

# html_favicon = '.sphinx/_static/favicon.png'

# Add any extra paths that contain custom files (such as robots.txt or
# .htaccess) here, relative to this directory. These files are copied
# directly to the root of the documentation.
html_extra_path = ['.sphinx/_extra']

# Dictionary of values to pass into the Sphinx context for all pages:
# https://www.sphinx-doc.org/en/master/usage/configuration.html#confval-html_context

html_context = {
    # Product page URL; can be different from product docs URL
    "product_page": "juju.is",
    # Product tag image; the orange part of your logo, shown in the page header
    # Assumes the current directory is .sphinx.
    'product_tag': '_static/logos/juju-logo-no-text.png',
    # Your Discourse instance URL
    "discourse": "https://discourse.charmhub.io",
    # Your Mattermost channel URL
    "mattermost": "",
    # Your Matrix channel URL
    "matrix": "https://matrix.to/#/#charmhub-juju:ubuntu.com",
    # Your documentation GitHub repository URL
    "github_url": "https://github.com/juju/juju",
    # Your documentation GitHub issues URL
     "github_issues": "https://github.com/juju/juju/issues",
    # Docs location in the repo; used in links for viewing the source files
    "repo_folder": "/docs/",
    # Docs branch in the repo; used in links for viewing the source files
    'repo_default_branch': 'main',
}

# Project slug; see https://meta.discourse.org/t/what-is-category-slug/87897
#
# TODO: If your documentation is hosted on https://docs.ubuntu.com/,
#       uncomment and update as needed.

slug = 'juju'


# Template and asset locations

html_static_path = [".sphinx/_static"]
templates_path = [".sphinx/_templates"]


#############
# Redirects #
#############

# To set up redirects: https://documatt.gitlab.io/sphinx-reredirects/usage.html
# For example: 'explanation/old-name.html': '../how-to/prettify.html',

# To set up redirects in the Read the Docs project dashboard:
# https://docs.readthedocs.io/en/stable/guides/redirects.html

# NOTE: If undefined, set to None, or empty,
#       the sphinx_reredirects extension will be disabled.

redirects = {
'user/reference/charm/charm-naming-guidelines/': 'https://canonical-charmcraft.readthedocs-hosted.com/en/stable/',
'reference/charm/charm-naming-guidelines/': 'https://canonical-charmcraft.readthedocs-hosted.com/en/stable/'
}


###########################
# Link checker exceptions #
###########################

# A regex list of URLs that are ignored by 'make linkcheck'
#
# TODO: Remove or adjust the ACME entry after you update the contributing guide

linkcheck_ignore = [
    "http://127.0.0.1:8000",
    "https://github.com/canonical/ACME/*"
]


# A regex list of URLs where anchors are ignored by 'make linkcheck'

linkcheck_anchors_ignore_for_url = [r"https://github\.com/.*"]


########################
# Configuration extras #
########################

# Custom MyST syntax extensions; see
# https://myst-parser.readthedocs.io/en/latest/syntax/optional.html
#
# NOTE: By default, the following MyST extensions are enabled:
#       substitution, deflist, linkify

# myst_enable_extensions = set()


# Custom Sphinx extensions; see
# https://www.sphinx-doc.org/en/master/usage/extensions/index.html

# NOTE: The canonical_sphinx extension is required for the starter pack.
#       It automatically enables the following extensions:
#       - custom-rst-roles
#       - myst_parser
#       - notfound.extension
#       - related-links
#       - sphinx_copybutton
#       - sphinx_design
#       - sphinx_reredirects
#       - sphinx_tabs.tabs
#       - sphinxcontrib.jquery
#       - sphinxext.opengraph
#       - terminal-output
#       - youtube-links

extensions = [
    'canonical_sphinx',
    'sphinx_design',
    # Make it possible to link to related RTD projects using their internal anchors
    # with, e.g., {external+ops:ref}`manage-configurations`:
    'sphinx.ext.intersphinx',
    'sphinxext.rediraffe',
    # Display an external link icon and open link in new tab:
    # new_tab_link_show_external_link_icon must also be set to True
    'sphinx_new_tab_link',
    'sphinxcontrib.lightbox2',
    ]



new_tab_link_show_external_link_icon = True

# Add redirects, so they can be updated here to land with docs being moved
# rediraffe_branch = "3.6"
rediraffe_redirects = "redirects.txt"

# Excludes files or directories from processing

exclude_patterns = [
    "doc-cheat-sheet*",
]

# Adds custom CSS files, located under 'html_static_path'

html_css_files = [
    "css/pdf.css",
]


# Adds custom JavaScript files, located under 'html_static_path'

# html_js_files = []


# Specifies a reST snippet to be appended to each .rst file

rst_epilog = """
.. include:: /reuse/links.txt
"""

# Feedback button at the top; enabled by default
#
# TODO: To disable the button, uncomment this.

# disable_feedback_button = True


# Your manpage URL
#
# TODO: To enable manpage links, uncomment and update as needed.
#
# NOTE: If set, adding ':manpage:' to an .rst file
#       adds a link to the corresponding man section at the bottom of the page.

# manpages_url = f'https://manpages.ubuntu.com/manpages/{codename}/en/' + \
#     f'man{section}/{page}.{section}.html'


# Specifies a reST snippet to be prepended to each .rst file
# This defines a :center: role that centers table cell content.
# This defines a :h2: role that styles content for use with PDF generation.

rst_prolog = """
.. role:: center
   :class: align-center
.. role:: h2
    :class: hclass2
"""

# Workaround for https://github.com/canonical/canonical-sphinx/issues/34

if "discourse_prefix" not in html_context and "discourse" in html_context:
    html_context["discourse_prefix"] = html_context["discourse"] + "/t/"


##################################
# sphinx.ext.intersphinx options #
##################################

intersphinx_mapping = {
    # 'juju': ('https://canonical-juju.readthedocs-hosted.com/en/latest/', None),
    'tfjuju': ('https://canonical-terraform-provider-juju.readthedocs-hosted.com/en/latest/', None),
    'pyjuju': ('https://pythonlibjuju.readthedocs.io/en/latest/', None),
    'jaas': ('https://canonical-jaas-documentation.readthedocs-hosted.com/latest/', None),
    'charmcraft': ('https://canonical-charmcraft.readthedocs-hosted.com/en/latest/', None),
    'ops': ('https://ops.readthedocs.io/en/latest/', None),
}

#####################
# PDF configuration #
#####################

latex_additional_files = [
    "./.sphinx/fonts/Ubuntu-B.ttf",
    "./.sphinx/fonts/Ubuntu-R.ttf",
    "./.sphinx/fonts/Ubuntu-RI.ttf",
    "./.sphinx/fonts/UbuntuMono-R.ttf",
    "./.sphinx/fonts/UbuntuMono-RI.ttf",
    "./.sphinx/fonts/UbuntuMono-B.ttf",
    "./.sphinx/images/Canonical-logo-4x.png",
    "./.sphinx/images/front-page-light.pdf",
    "./.sphinx/images/normal-page-footer.pdf",
]

latex_engine = "xelatex"
latex_show_pagerefs = True
latex_show_urls = "footnote"

with open(".sphinx/latex_elements_template.txt", "rt") as file:
    latex_config = file.read()

latex_elements = ast.literal_eval(latex_config.replace("$PROJECT", project))


##################################
# Auto-generation of documentation
##################################

def generate_cli_docs():
    cli_dir = "reference/juju-cli/"
    generated_cli_docs_dir = cli_dir + "list-of-juju-cli-commands/"
    cli_index_header = cli_dir + 'cli_index'

    # Remove existing cli folder to regenerate it
    if os.path.exists(generated_cli_docs_dir):
        shutil.rmtree(generated_cli_docs_dir)

    # Generate the CLI docs using script.
    result = subprocess.run(['go', 'run', '../scripts/md-gen/juju-cli-commands/main.go', generated_cli_docs_dir])
    if result.returncode != 0:
        raise Exception("error auto-generating juju cli commands: " + str(result.stderr))

    for page in os.listdir(generated_cli_docs_dir):
        title = "`juju " + page[:-3]+ "`"
        anchor = "command-juju-" + page[:-3]
        # Add sphinx names to each file.
        with open(os.path.join(generated_cli_docs_dir, page), 'r+') as mdfile:
            content = mdfile.read()
            # Remove trailing seperated (e.g. ----)
            content = content.rstrip(" -\n")
            mdfile.seek(0, 0)
            mdfile.write('(' + anchor + ')=\n' +
                         '# ' + title + '\n' +
                         content)

    # Add in the index file containing the command list.
    subprocess.run(['cp', cli_index_header, generated_cli_docs_dir + 'index.md'])

    print("generated juju cli command list")

def generate_controller_config_docs():
    config_reference_dir = 'reference/configuration/'
    controller_config_file = config_reference_dir + 'list-of-controller-configuration-keys.md'
    controller_config_header = config_reference_dir + 'list-of-controller-configuration-keys.header'

    # Generate the controller config using script. The first argument of the script
    # is the root directory of the juju source code. This is the parent directory
    # so use pass '..'.
    result = subprocess.run(['go', 'run', '../scripts/md-gen/controller-config/main.go', '..'],
                            capture_output=True, text=True)
    if result.returncode != 0:
        raise Exception("error auto-generating controller config: " + str(result.stderr))

    # Remove existing controller config
    if os.path.exists(controller_config_file):
        os.remove(controller_config_file)

    # Copy header for the controller config doc page in.
    subprocess.run(['cp', controller_config_header, controller_config_file])

    # Append autogenerated docs.
    with open(controller_config_file, 'a') as f:
        f.write(result.stdout)

    print("generated controller config key list")

def generate_model_config_docs():
    config_reference_dir = 'reference/configuration/'
    model_config_file = config_reference_dir + 'list-of-model-configuration-keys.md'
    model_config_header = config_reference_dir + 'list-of-model-configuration-keys.header'

    # Generate the model config using script.
    result = subprocess.run(['go', 'run', '../scripts/md-gen/model-config/main.go'],
                            capture_output=True, text=True)
    if result.returncode != 0:
        raise Exception("error auto-generating model config: " + str(result.stderr))

    # Remove existing model config
    if os.path.exists(model_config_file):
        os.remove(model_config_file)

    # Copy header for the model config doc page in.
    subprocess.run(['cp', model_config_header, model_config_file])

    # Append autogenerated docs.
    with open(model_config_file, 'a') as f:
        f.write(result.stdout)

    print("generated model config key list")

def generate_hook_command_docs():
    hook_commands_reference_dir = 'reference/hook-command/'
    generated_hook_commands_dir = hook_commands_reference_dir + 'list-of-hook-commands/'
    hook_index_header = hook_commands_reference_dir + 'hook_index'

    # Remove existing hook command folder to regenerate it
    if os.path.exists(generated_hook_commands_dir):
        shutil.rmtree(generated_hook_commands_dir)

    # Generate the hook commands doc using script.
    result = subprocess.run(['go', 'run', '../scripts/md-gen/hook-commands/main.go', generated_hook_commands_dir],
                            check=True)
    if result.returncode != 0:
        raise Exception("error auto-generating hook commands: " + str(result.stderr))

    # Remove 'help' and 'documentation' files as they are not needed.
    if os.path.exists(generated_hook_commands_dir + 'help.md'):
        os.remove(generated_hook_commands_dir + 'help.md')
    if os.path.exists(generated_hook_commands_dir + 'documentation.md'):
        os.remove(generated_hook_commands_dir + 'documentation.md')

    for page in os.listdir(generated_hook_commands_dir):
        title = "`" + page[:-3] + "`"
        anchor = "hook-command-" + page[:-3]
        # Add sphinx names to each file.
        with open(os.path.join(generated_hook_commands_dir, page), 'r+') as mdfile:
            content = mdfile.read()
            # Remove trailing seperated (e.g. ----)
            content = content.rstrip(" -\n")
            mdfile.seek(0, 0)
            mdfile.write('(' + anchor + ')=\n' +
                         '# ' + title + '\n' +
                         content)

    # Add in the index file containing the command list.
    subprocess.run(['cp', hook_index_header, generated_hook_commands_dir + 'index.md'])

    print("generated hook command list")

generate_cli_docs()
generate_controller_config_docs()
generate_model_config_docs()
generate_hook_command_docs()
