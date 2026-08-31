# Copyright 2025 Canonical Ltd.
# SPDX-License-Identifier: Apache-2.0
#
# Sphinx extension for D2 diagrams (https://d2lang.com).
# Follows the same pattern as sphinxcontrib.mermaid.
#
# Usage in Markdown:
#
#   ```{d2}
#   direction: right
#   client -> controller: "Juju API"
#   ```
#
# Configuration in conf.py:
#   d2_cmd        -- path to the d2 binary (default: "d2")
#   d2_layout     -- layout engine: "dagre" or "elk" (default: "elk")
#   d2_theme      -- D2 theme ID (default: 0)

from __future__ import annotations

import hashlib
import os
import posixpath
import re
import shlex
from subprocess import PIPE, Popen
from tempfile import TemporaryDirectory

from docutils import nodes
from docutils.parsers.rst import directives
from sphinx.util import logging
from sphinx.util.docutils import SphinxDirective
from sphinx.util.osutil import ensuredir

logger = logging.getLogger(__name__)


class D2Error(Exception):
    pass


class d2(nodes.General, nodes.Inline, nodes.Element):
    pass


class D2Directive(SphinxDirective):
    """Directive to embed a D2 diagram."""

    has_content = True
    required_arguments = 0
    optional_arguments = 0
    option_spec = {
        "layout": directives.unchanged,
        "theme": directives.unchanged,
        "alt": directives.unchanged,
    }

    def run(self) -> list[nodes.Node]:
        code = "\n".join(self.content)
        node = d2()
        node["code"] = code
        node["layout"] = self.options.get("layout", "")
        node["theme"] = self.options.get("theme", "")
        node["alt"] = self.options.get("alt", "")
        self.set_source_info(node)
        return [node]


def render_d2(self: object, code: str, options: dict, prefix: str = "d2") -> tuple[str | None, str | None]:
    """Compile D2 source to SVG and return (relative URL, absolute path)."""
    d2_cmd = self.builder.config.d2_cmd
    layout = options.get("layout") or self.builder.config.d2_layout
    theme = options.get("theme") or str(self.builder.config.d2_theme)

    hashkey = (code + layout + theme).encode("utf-8")
    basename = f"{prefix}-{hashlib.sha1(hashkey).hexdigest()}"  # noqa: S324
    fname = f"{basename}.svg"
    relfn = posixpath.join(self.builder.imgpath, fname)
    outdir = os.path.join(self.builder.outdir, self.builder.imagedir)
    outfn = os.path.join(outdir, fname)

    if os.path.isfile(outfn):
        return relfn, outfn

    ensuredir(outdir)

    if isinstance(d2_cmd, str):
        cmd_args = shlex.split(d2_cmd)
    else:
        cmd_args = list(d2_cmd)

    cmd_args += ["--layout", layout, "--theme", theme]

    with TemporaryDirectory() as tmpdir:
        infn = os.path.join(tmpdir, f"{basename}.d2")
        with open(infn, "w", encoding="utf-8") as f:
            f.write(code)
        cmd_args += [infn, outfn]

        try:
            p = Popen(cmd_args, stdout=PIPE, stdin=PIPE, stderr=PIPE, text=True)
        except FileNotFoundError:
            logger.warning(
                f"d2 command {d2_cmd!r} not found -- install d2 from https://d2lang.com "
                "or set d2_cmd in conf.py"
            )
            return None, None

        stdout, stderr = p.communicate()
        if p.returncode != 0:
            raise D2Error(f"d2 exited with error:\n{stderr}\n{stdout}")
        if not os.path.isfile(outfn):
            raise D2Error(f"d2 produced no output file:\n{stderr}\n{stdout}")

    # Remove fixed pixel width/height from the SVG so it scales to its
    # container. The viewBox is preserved so aspect ratio is maintained.
    with open(outfn, encoding="utf-8") as f:
        svg = f.read()
    svg = re.sub(
        r'(<svg[^>]*)\s+width="[^"]*"\s+height="[^"]*"',
        r'\1 width="100%" height="auto"',
        svg,
        count=1,
    )
    with open(outfn, "w", encoding="utf-8") as f:
        f.write(svg)

    return relfn, outfn


def html_visit_d2(self: object, node: d2) -> None:
    code = node["code"]
    options = {"layout": node.get("layout", ""), "theme": node.get("theme", "")}
    alt = node.get("alt", "") or "D2 diagram"

    try:
        fname, outfn = render_d2(self, code, options)
    except D2Error as exc:
        logger.warning(f"d2 diagram error: {exc}")
        raise nodes.SkipNode from exc

    if fname is None:
        # d2 not found -- render source as a code block so docs still build
        self.body.append(f'<pre class="d2-source">{self.encode(code)}</pre>\n')
        raise nodes.SkipNode

    # Register the image with the builder so Sphinx copies it to _images/
    # and lightbox2 can rewrite the URI correctly.
    imgnode = nodes.image()
    imgnode["uri"] = outfn
    imgnode["alt"] = alt
    imgnode["candidates"] = {"*": outfn}
    # width: 100% makes it fill the content column; lightbox opens full-size SVG
    imgnode["width"] = "100%"

    # Register with builder image tracking
    self.builder.images[outfn] = os.path.basename(outfn)

    # Delegate to lightbox2-wrapped image visitor
    self.visit_image(imgnode)
    self.depart_image(imgnode)

    raise nodes.SkipNode


def setup(app: object) -> dict:
    app.add_node(d2, html=(html_visit_d2, None))
    app.add_directive("d2", D2Directive)
    app.add_config_value("d2_cmd", "d2", "html")
    app.add_config_value("d2_layout", "elk", "html")
    app.add_config_value("d2_theme", 0, "html")
    return {"version": "0.1.0", "parallel_read_safe": True, "parallel_write_safe": True}
