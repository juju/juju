from docutils import nodes
from docutils.parsers.rst import Directive


class IbNoteDirective(Directive):
    has_content = True

    def run(self):
        text = "\n".join(self.content)
        node = nodes.container(classes=["ibnote"])
        self.state.nested_parse(self.content, self.content_offset, node)
        return [node]


def setup(app):
    app.add_directive("ibnote", IbNoteDirective)
    return {"version": "0.1", "parallel_read_safe": True, "parallel_write_safe": True}
