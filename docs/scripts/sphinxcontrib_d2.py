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
#   d2_cmd          -- path to the d2 binary (default: "d2")
#   d2_layout       -- layout engine: "dagre" or "elk" (default: "elk")
#   d2_light_theme  -- D2 theme ID for light mode (default: 0)
#   d2_dark_theme   -- D2 theme ID for dark mode (default: 200)

from __future__ import annotations

import hashlib
import os
import posixpath
import re
import shlex
import urllib.parse
from subprocess import PIPE, Popen
from tempfile import TemporaryDirectory

from docutils import nodes
from docutils.parsers.rst import directives
from sphinx.util import logging
from sphinx.util.docutils import SphinxDirective
from sphinx.util.osutil import ensuredir

logger = logging.getLogger(__name__)

# D2 dark theme ID
D2_DARK_THEME = 200


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
        "alt": directives.unchanged,
    }

    def run(self) -> list[nodes.Node]:
        code = "\n".join(self.content)
        node = d2()
        node["code"] = code
        node["layout"] = self.options.get("layout", "")
        node["alt"] = self.options.get("alt", "")
        self.set_source_info(node)
        return [node]


def _render_one(
    d2_cmd: str | list,
    code: str,
    layout: str,
    theme: int,
    outfn: str,
    pad: int = 20,
) -> None:
    """Compile D2 source to a single SVG file."""
    if isinstance(d2_cmd, str):
        cmd_args = shlex.split(d2_cmd)
    else:
        cmd_args = list(d2_cmd)

    cmd_args += ["--layout", layout, "--theme", str(theme), "--pad", str(pad)]

    with TemporaryDirectory() as tmpdir:
        basename = os.path.splitext(os.path.basename(outfn))[0]
        infn = os.path.join(tmpdir, f"{basename}.d2")
        with open(infn, "w", encoding="utf-8") as f:
            f.write(code)
        cmd_args += [infn, outfn]

        try:
            p = Popen(cmd_args, stdout=PIPE, stdin=PIPE, stderr=PIPE, text=True)
        except FileNotFoundError:
            raise D2Error(
                f"d2 command {d2_cmd!r} not found -- "
                "install d2 from https://d2lang.com or set d2_cmd in conf.py"
            ) from None

        stdout, stderr = p.communicate()
        if p.returncode != 0:
            raise D2Error(f"d2 exited with error:\n{stderr}\n{stdout}")
        if not os.path.isfile(outfn):
            raise D2Error(f"d2 produced no output file:\n{stderr}\n{stdout}")

    # Remove fixed pixel dimensions so the SVG scales to its container.
    # The viewBox is preserved so aspect ratio is maintained.
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


def render_d2_pair(
    self: object,
    code: str,
    layout: str,
    prefix: str = "d2",
) -> tuple[tuple[str, str] | None, tuple[str, str] | None]:
    """Compile D2 to light and dark SVGs. Returns ((relfn, outfn), (relfn, outfn))."""
    d2_cmd = self.builder.config.d2_cmd
    light_theme = self.builder.config.d2_light_theme
    dark_theme = self.builder.config.d2_dark_theme
    pad = self.builder.config.d2_pad

    outdir = os.path.join(self.builder.outdir, self.builder.imagedir)
    ensuredir(outdir)

    results = []
    for suffix, theme in (("light", light_theme), ("dark", dark_theme)):
        hashkey = (code + layout + str(theme) + str(pad)).encode("utf-8")
        basename = f"{prefix}-{hashlib.sha1(hashkey).hexdigest()}"  # noqa: S324
        fname = f"{basename}.svg"
        relfn = posixpath.join(self.builder.imgpath, fname)
        outfn = os.path.join(outdir, fname)

        if not os.path.isfile(outfn):
            try:
                _render_one(d2_cmd, code, layout, theme, outfn, pad=pad)
            except D2Error as exc:
                logger.warning(f"d2 diagram error ({suffix}): {exc}")
                results.append(None)
                continue

        results.append((relfn, outfn))

    return tuple(results)  # type: ignore[return-value]


def _emit_image(
    self: object,
    relfn: str,
    outfn: str,
    alt: str,
    css_class: str,
    lightbox_group: str,
) -> None:
    """Emit a lightbox-wrapped <img> inside an only-light/only-dark div."""
    self.builder.images[outfn] = os.path.basename(outfn)
    # Rewrite URI the same way lightbox2 does
    uri = posixpath.join(
        self.builder.imgpath,
        urllib.parse.quote(self.builder.images[outfn]),
    )
    self.body.append(
        f'<div class="{css_class}">'
        f'<a href="{uri}" data-lightbox="{lightbox_group}">'
        f'<img src="{uri}" alt="{alt}" style="width:100%;" />'
        f'</a>'
        f'</div>\n'
    )


def html_visit_d2(self: object, node: d2) -> None:
    code = node["code"]
    layout = node.get("layout", "") or self.builder.config.d2_layout
    alt = node.get("alt", "") or "D2 diagram"

    light, dark = render_d2_pair(self, code, layout)

    if light is None and dark is None:
        self.body.append(f'<pre class="d2-source">{self.encode(code)}</pre>\n')
        raise nodes.SkipNode

    # Unique group per diagram so lightbox > doesn't cross diagrams or modes
    group = "d2-" + hashlib.sha1(code.encode()).hexdigest()[:8]  # noqa: S324

    if light is not None:
        _emit_image(self, light[0], light[1], alt, "only-light", group + "-light")
    if dark is not None:
        _emit_image(self, dark[0], dark[1], alt, "only-dark", group + "-dark")

    raise nodes.SkipNode


def setup(app: object) -> dict:
    app.add_node(d2, html=(html_visit_d2, None))
    app.add_directive("d2", D2Directive)
    app.add_config_value("d2_cmd", "d2", "html")
    app.add_config_value("d2_layout", "elk", "html")
    app.add_config_value("d2_light_theme", 0, "html")
    app.add_config_value("d2_dark_theme", D2_DARK_THEME, "html")
    app.add_config_value("d2_pad", 20, "html")
    return {"version": "0.3.0", "parallel_read_safe": True, "parallel_write_safe": True}
