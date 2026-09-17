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


def resolve_node_ground(node_id: str, ground: str, problems: list) -> None:
    if "/" in ground and ":" not in ground.split("/")[0]:
        # Code-path ground: "path/to/file.go:NN"
        path = REPO / ground.split(":")[0]
        if not path.exists():
            problems.append(f"{node_id}: code path not found: {ground}")
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
        if n.type == "record":
            ground = n.properties.get("ground", "")
            if not ground:
                problems.append(f"{n.id}: record node without ground:")
            else:
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


if __name__ == "__main__":
    sys.exit(main(sys.argv[1] if len(sys.argv) > 1 else "juju3.ggarch"))
