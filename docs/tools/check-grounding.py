#!/usr/bin/env python3
"""Check that every grounded fact in a .ggarch file resolves to the
codebase.

One-directional by design: everything DRAWN must exist in the schema;
not everything existing must be drawn. Curation is legitimate; undeclared
curation is not.

Ground formats:
  Record nodes:   ground: "db:table"            -> table exists in db's DDL
                 ground: "db:tableA+tableB"     -> declared flattening; each exists
                 ground: "path/to/file.go:NN"   -> repo-relative file exists
  Data edges:     ground: "db:table"            -> table exists
                 ground: "db:table.column"       -> column exists in table

Databases: model    -> domain/schema/model/<latest>.ddl
           controller -> domain/schema/controller/<latest>.ddl

Exits 1 listing every problem (ungrounded, unresolvable, or malformed).
"""
from __future__ import annotations

import re
import sys
from pathlib import Path

from ggarch import parse, validate


REPO = Path(__file__).resolve().parents[2]
DDL = {
    "model": sorted((REPO / "domain/schema/model").glob("*-model-release.ddl"))[-1],
    "controller": sorted((REPO / "domain/schema/controller").glob("*-controller-release.ddl"))[-1],
}

_CREATE = re.compile(r"CREATE TABLE (\w+) \((.*?)\n\);", re.S)


def load_schema(ddl: Path) -> dict[str, set[str]]:
    """Return {table: {column, ...}} from a DDL file."""
    out: dict[str, set[str]] = {}
    for m in _CREATE.finditer(ddl.read_text()):
        table, body = m.group(1), m.group(2)
        cols = set()
        for line in body.splitlines():
            line = line.strip()
            if not line or line.startswith("--"):
                continue
            cm = re.match(r"^(\w+)\s", line)
            if cm and cm.group(1) not in (
                "FOREIGN", "CONSTRAINT", "PRIMARY", "UNIQUE", "CHECK",
            ):
                cols.add(cm.group(1))
        out[table] = cols
    return out


SCHEMAS = {db: load_schema(ddl) for db, ddl in DDL.items()}


def _code_ground_resolves(node_id: str, ground: str, problems: list) -> None:
    """Code-path ground: "path/to/file.go:NN" (line) or
    "path/to/file.go:Symbol" (2026-09-20, TODOS F2: symbols survive
    code movement where line numbers rot — a line-only ground is
    existence-verified and gross-drift-checked; a symbol ground must
    appear in the file as a func declaration or a manifold-style
    entry)."""
    path_str, _, token = ground.partition(":")
    path = REPO / path_str
    if not path.exists():
        problems.append(f"{node_id}: code path not found: {ground}")
        return
    if token.isdigit():
        # Line ground: existence (above) plus a gross-drift bound —
        # a ground past the end of file is rotted beyond usefulness.
        if int(token) < 1 or int(token) > len(
                path.read_text(errors="ignore").splitlines()):
            problems.append(f"{node_id}: line {token} beyond end of "
                            f"{path_str}")
        return
    text = path.read_text(errors="ignore")
    pat = re.compile(rf"func [^{{]*\b{re.escape(token)}\(|^\s*{re.escape(token)}:",
                     re.M)
    if not pat.search(text):
        problems.append(f"{node_id}: symbol {token!r} not found in "
                        f"{path_str} (moved or renamed?)")


def resolve_node_ground(node_id: str, ground: str, problems: list) -> None:
    if "/" in ground.split(":")[0]:
        _code_ground_resolves(node_id, ground, problems)
        return
    if ":" not in ground or ground.split(":", 1)[0] not in SCHEMAS:
        problems.append(f"{node_id}: malformed ground: {ground}")
        return
    db, tables = ground.split(":", 1)
    for t in tables.split("+"):
        if t.strip() not in SCHEMAS[db]:
            problems.append(f"{node_id}: table {db}:{t} not in DDL")


def resolve_edge_ground(src: str, tgt: str, ground: str, problems: list) -> None:
    if ":" not in ground or ground.split(":", 1)[0] not in SCHEMAS:
        problems.append(f"edge {src}->{tgt}: malformed ground: {ground}")
        return
    db, rest = ground.split(":", 1)
    table, _, column = rest.partition(".")
    if table not in SCHEMAS[db]:
        problems.append(f"edge {src}->{tgt}: table {db}:{table} not in DDL")
        return
    if column and column not in SCHEMAS[db][table]:
        problems.append(f"edge {src}->{tgt}: column {db}:{table}.{column} not in DDL")


def walk_nodes(nodes, problems: list) -> None:
    for n in nodes:
        ground = n.properties.get("ground", "")
        if ground:
            resolve_node_ground(n.id, ground, problems)
        walk_nodes(n.children, problems)


def main(path: str) -> int:
    f = parse(Path(path).read_text())
    validate(f)
    problems: list[str] = []
    for model in f.models:
        walk_nodes(model.nodes, problems)
        for e in model.edges:
            if e.type == "data":
                ground = e.properties.get("ground", "")
                if not ground:
                    problems.append(
                        f"edge {e.source}->{e.target}: data edge without ground:")
                else:
                    resolve_edge_ground(e.source, e.target, ground, problems)
    if problems:
        print(f"{path}: {len(problems)} grounding problem(s):")
        for p in problems:
            print(f"  - {p}")
        return 1
    print(f"{path}: all grounds resolve.")
    return 0




def _symbol_at(path: Path, line_no: int) -> str | None:
    """The symbol a ground line names: a manifold-style entry
    ("agentName: agent.Manifold(...)" -> agentName) or the enclosing
    func declaration. None when neither applies."""
    lines = path.read_text(errors="ignore").splitlines()
    if 1 <= line_no <= len(lines):
        entry = re.match(r"^\s+(\w+):\s", lines[line_no - 1])
        if entry:
            return entry.group(1)
    for i in range(min(line_no, len(lines)) - 1, -1, -1):
        fn = re.match(r"^func [^({]*[ (\w\*\)\.]*\b(\w+)\(", lines[i])
        if fn:
            return fn.group(1)
    return None


def migrate(path_str: str, problems: list) -> None:
    """Rewrite line grounds ("path.go:NN") to symbol grounds
    ("path.go:Symbol") in the .ggarch file, in place. Symbols survive
    code movement where line numbers rot (TODOS F2)."""
    import ggarch  # noqa: F401  (the file is parsed by the caller)

    text = Path(path_str).read_text()
    path_str_repo = text  # placeholder to keep the closure readable
    for m in re.finditer(r'ground: "([/\w\.\-]+\.go):(\d+)"', text):
        go_path, line_no = m.group(1), int(m.group(2))
        path = REPO / go_path
        if not path.exists():
            problems.append(f"{go_path}:{line_no}: file not found — "
                            f"left as-is")
            continue
        symbol = _symbol_at(path, line_no)
        if symbol is None:
            problems.append(f"{go_path}:{line_no}: no symbol found — "
                            f"left as-is")
            continue
        text = text.replace(f'ground: "{go_path}:{line_no}"',
                            f'ground: "{go_path}:{symbol}"')
    Path(path_str).write_text(text)
if __name__ == "__main__":
    args = [a for a in sys.argv[1:]]
    if "--migrate" in args:
        problems: list[str] = []
        for f in [a for a in args if a != "--migrate"]:
            migrate(f, problems)
        for p in problems:
            print(p)
        sys.exit(1 if problems else 0)
    main(args[0] if args else "juju3.ggarch")
