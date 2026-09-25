# Copyright 2025 Canonical Ltd.
# SPDX-License-Identifier: Apache-2.0
#
# Sphinx extension for section-level audience tags.
#
# Usage in Markdown (first line of the section body):
#
#   ```{audience} user, charm-dev
#   ```
#
# The argument is a comma-separated SET of tokens. Tokens are the stable
# API; AUDIENCE_LABELS is the single render authority for BOTH the HTML
# line and the llms markdown line, so a relabel is one edit + rebuild,
# never a content sweep.
#
# Why a custom directive and not a sphinx-design badge role: a role
# gives no control over the llms output (front matter never reaches it
# either); the directive's markdown visitor is the machine-readable
# carrier -- one plain "For: ..." line per tagged section.

from docutils import nodes
from docutils.parsers.rst import Directive


class audience(nodes.General, nodes.Element):
    pass


# The central token->label map (reviewer ruling, session 20): the user
# label is "Juju-and-charm users", never "operators".
AUDIENCE_LABELS = {
    "user": "Juju-and-charm users",
    "charm-dev": "Charm developers",
    "juju-dev": "Juju developers",
}


def _labels(tokens):
    return [AUDIENCE_LABELS[token] for token in tokens]


class AudienceDirective(Directive):
    """Tag a section with the audience(s) it is for."""

    required_arguments = 1
    final_argument_whitespace = True
    has_content = False

    def run(self):
        tokens = []
        for token in (t.strip() for t in self.arguments[0].split(",")):
            if token and token not in tokens:
                tokens.append(token)
        if not tokens:
            raise self.error("audience directive needs at least one token")
        unknown = [t for t in tokens if t not in AUDIENCE_LABELS]
        if unknown:
            raise self.error(
                "unknown audience token(s): %s (known: %s)"
                % (", ".join(unknown), ", ".join(AUDIENCE_LABELS))
            )
        node = audience()
        node["tokens"] = tokens
        return [node]


def _line(tokens):
    """The one render authority: the HTML and the llms output carry
    the same line."""
    return "For: %s." % ", ".join(_labels(tokens))


def html_visit_audience(self, node):
    """Render the tag as a backgrounded line, ibnote-style: always
    there, non-intrusive (small, muted, no box)."""
    self.body.append('<p class="audience">%s</p>\n' % _line(node["tokens"]))
    raise nodes.SkipNode


def markdown_visit_audience(self, node):
    """The llms carrier: one plain line per tagged section."""
    self.add(_line(node["tokens"]), prefix_eol=1, suffix_eol=1)
    raise nodes.SkipNode


def text_visit_audience(self, node):
    """Plain-text output (man pages etc.)."""
    self.add_text("[For: %s]" % ", ".join(_labels(node["tokens"])))
    raise nodes.SkipNode


def setup(app):
    app.add_node(
        audience,
        html=(html_visit_audience, None),
        markdown=(markdown_visit_audience, None),
        text=(text_visit_audience, None),
        man=(text_visit_audience, None),
    )
    app.add_directive("audience", AudienceDirective)
    return {"version": "0.1", "parallel_read_safe": True, "parallel_write_safe": True}