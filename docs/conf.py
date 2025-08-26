import datetime
import os
import yaml
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

ogp_image = "https://assets.ubuntu.com/v1/253da317-image-document-ubuntudocs.svg"


# Product favicon; shown in bookmarks, browser tabs, etc.

# TODO: To customise the favicon, uncomment and update as needed.

# html_favicon = '.sphinx/_static/favicon.png'

# Add any extra paths that contain custom files (such as robots.txt or
# .htaccess) here, relative to this directory. These files are copied
# directly to the root of the documentation.
# TODO: Check if this is still needed.
html_extra_path = ['.sphinx/_extra']

# Dictionary of values to pass into the Sphinx context for all pages:
# https://www.sphinx-doc.org/en/master/usage/configuration.html#confval-html_context

html_context = {
    # Product page URL; can be different from product docs URL
    #
    # TODO: Change to your product website URL,
    #       dropping the 'https://' prefix, e.g. 'ubuntu.com/lxd'.
    #
    # TODO: If there's no such website,
    #       remove the {{ product_page }} link from the page header template
    #       (usually .sphinx/_templates/header.html; also, see README.rst).
    "product_page": "juju.is/docs",
    # Product tag image; the orange part of your logo, shown in the page header
    #
    # TODO: To add a tag image, uncomment and update as needed.
    'product_tag': '_static/logos/juju-logo-no-text.png',
    # Your Discourse instance URL
    #
    # TODO: Change to your Discourse instance URL or leave empty.
    #
    # NOTE: If set, adding ':discourse: 123' to an .rst file
    #       will add a link to Discourse topic 123 at the bottom of the page.
    "discourse": "https://discourse.charmhub.io",
    # Your Mattermost channel URL
    #
    # TODO: Change to your Mattermost channel URL or leave empty.
    "mattermost": "",
    # Your Matrix channel URL
    #
    # TODO: Change to your Matrix channel URL or leave empty.
    "matrix": "https://matrix.to/#/#charmhub-juju:ubuntu.com",
    # Your documentation GitHub repository URL
    #
    # TODO: Change to your documentation GitHub repository URL or leave empty.
    #
    # NOTE: If set, links for viewing the documentation source files
    #       and creating GitHub issues are added at the bottom of each page.
    "github_url": "https://github.com/juju/juju",
    # Docs branch in the repo; used in links for viewing the source files
    #
    # TODO: To customise the branch, uncomment and update as needed.
    'repo_default_branch': 'main',
    # Docs location in the repo; used in links for viewing the source files
    #


    # TODO: To customise the directory, uncomment and update as needed.
    "repo_folder": "/docs/",
    # TODO: To enable or disable the Previous / Next buttons at the bottom of pages
    # Valid options: none, prev, next, both
    "sequential_nav": "both",
    # TODO: To enable listing contributors on individual pages, set to True
    "display_contributors": False,

    # Required for feedback button
    'github_issues': 'enabled',
}

# TODO: To enable the edit button on pages, uncomment and change the link to a
# public repository on GitHub or Launchpad. Any of the following link domains
# are accepted:
# - https://github.com/example-org/example"
# - https://launchpad.net/example
# - https://git.launchpad.net/example
#
# html_theme_options = {
# 'source_edit_link': 'https://github.com/canonical/sphinx-docs-starter-pack',
# }

# Project slug; see https://meta.discourse.org/t/what-is-category-slug/87897
#
# TODO: If your documentation is hosted on https://docs.ubuntu.com/,
#       uncomment and update as needed.

slug = 'juju'

#######################
# Sitemap configuration: https://sphinx-sitemap.readthedocs.io/
#######################

# Base URL of RTD hosted project

html_baseurl = 'https://documentation.ubuntu.com/juju/'

# URL scheme. Add language and version scheme elements.
# When configured with RTD variables, check for RTD environment so manual runs succeed:

if 'READTHEDOCS_VERSION' in os.environ:
    version = os.environ["READTHEDOCS_VERSION"]
    sitemap_url_scheme = '{version}{link}'
else:
    sitemap_url_scheme = 'MANUAL/{link}'

# Include `lastmod` dates in the sitemap:

sitemap_show_lastmod = True

# Exclude generated pages from the sitemap:

sitemap_excludes = [
    '404/',
    'genindex/',
    'search/',
]

# TODO: Add more pages to sitemap_excludes if needed. Wildcards are supported.
#       For example, to exclude module pages generated by autodoc, add '_modules/*'.

#######################
# Template and asset locations
#######################

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

linkcheck_anchors_ignore_for_url = [
    r"https://github\.com/.*",
    r"https://charmhub\.io/.*",
    r"https://launchpad\.net/.*",
    r"https://matrix\.to/.*",
    r"https://ghcr\.io/.*",
]

# give linkcheck multiple tries on failure
# linkcheck_timeout = 30
linkcheck_retries = 3


######################## old below

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
    # from upstream:
    "canonical_sphinx",
    "sphinxcontrib.cairosvgconverter",
    "sphinx_last_updated_by_git",
    "sphinx.ext.intersphinx",
    "sphinx_sitemap",
    # our own:
    'sphinx_design',
    # Make it possible to link to related RTD projects using their internal anchors
    # with, e.g., {external+ops:ref}`manage-configurations`:
    'sphinxext.rediraffe',
    # Display an external link icon and open link in new tab:
    # new_tab_link_show_external_link_icon must also be set to True
    'sphinx_new_tab_link',
    'sphinxcontrib.lightbox2',
    ]

# Extension configs:
# - sphinx.ext.intersphinx:
intersphinx_mapping = {
    # 'juju': ('https://canonical-juju.readthedocs-hosted.com/en/latest/', None),
    'tfjuju': ('https://documentation.ubuntu.com/terraform-provider-juju/latest/', None),
    'pyjuju': ('https://pythonlibjuju.readthedocs.io/en/latest/', None),
    'jaas': ('https://documentation.ubuntu.com/jaas/latest/', None),
    'charmcraft': ('https://documentation.ubuntu.com/charmcraft/stable/', None),
    'ops': ('https://documentation.ubuntu.com/ops/latest/', None),
}
# - sphinx_new_tab_link:
new_tab_link_show_external_link_icon = True
# - sphinxext.rediraffe:
# rediraffe_branch = "3.6"
rediraffe_redirects = "redirects.txt"

# Excludes files or directories from processing

exclude_patterns = [
    "doc-cheat-sheet*",
]

# Adds custom CSS files, located under 'html_static_path'

html_css_files = [
    "css/pdf.css",
    "css/cookie-banner.css",
]


# Adds custom JavaScript files, located under 'html_static_path'

html_js_files = [
    "js/bundle.js",
]


# Specifies a reST snippet to be appended to each .rst file

rst_epilog = """
.. include:: /reuse/links.txt
.. include:: /reuse/substitutions.txt
"""

# Feedback button at the top; enabled by default
#
# TODO: To disable the button, uncomment this.

# disable_feedback_button = True


# Your manpage URL
#
# TODO: To enable manpage links, uncomment and replace {codename} with required
#       release, preferably an LTS release (e.g. noble). Do *not* substitute
#       {section} or {page}; these will be replaced by sphinx at build time
#
# NOTE: If set, adding ':manpage:' to an .rst file
#       adds a link to the corresponding man section at the bottom of the page.

stable_distro = "plucky"

manpages_url = (
    "https://manpages.ubuntu.com/manpages/"
    + stable_distro
    + "/en/man{section}/{page}.{section}.html"
)

# Specifies a reST snippet to be prepended to each .rst file
# This defines a :center: role that centers table cell content.
# This defines a :h2: role that styles content for use with PDF generation.

rst_prolog = """
.. role:: center
   :class: align-center
.. role:: h2
    :class: hclass2
.. role:: woke-ignore
    :class: woke-ignore
.. role:: vale-ignore
    :class: vale-ignore
"""

# Workaround for https://github.com/canonical/canonical-sphinx/issues/34

if "discourse_prefix" not in html_context and "discourse" in html_context:
    html_context["discourse_prefix"] = html_context["discourse"] + "/t/"

# Workaround for substitutions.yaml

if os.path.exists('./reuse/substitutions.yaml'):
    with open('./reuse/substitutions.yaml', 'r') as fd:
        myst_substitutions = yaml.safe_load(fd.read())

