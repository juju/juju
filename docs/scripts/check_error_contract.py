#!/usr/bin/env python3
"""Check the reference pages' documented error contract against the source.

Every documented error entry — a bold-backtick message in an Errors list of
docs/reference/*.md — must be raisable by the Go source of this repository,
its pinned `github.com/juju/*` dependencies included. The message is a
template on both sides: the pages write slots as `<name>` (and `"<name>"`
for quoted values), the source writes fmt verbs (`%s`, `%q`, `%w`, ...), and
the pages sometimes spell out what the source parameterises (a
`%s address(es)` is documented as the concrete `no public address(es)`;
the dependency's `%q is not a valid %s tag` as `<tag>" is not a valid unit
tag`). A documented message passes when a raise site's format string
renders it:

- verbatim: the message, with its slots marked, is fully matched by the
  template's regular expression — which covers the `NotFoundf`-style
  constructors (their message is `format + " " + suffix`, e.g.
  `secret %q not found`) and the concretised verbs;
- as a wrap chain: the source template ends in a verb (`%w`, `%s`) and the
  documented message spells the wrapped inner message out; each inner link
  must itself match a raise site, and a bare `<reason>` slot ends a chain
  (a message the page truncates with `...` ends one too);
- the documented error codes (the `(code `x`)` notes) must be codes the API
  server can answer with: the Code* strings of rpc/params/apierror.go (parsed
  live from the source), or an HTTP status.

Raise forms extracted: New, Newf, Errorf (github.com/juju/errors and
github.com/juju/juju/internal/errors under their import aliases), fmt.Errorf,
fmt.Sprintf (messages built for params.Error), the Message field of a
params.Error literal, the annotated *f constructors of github.com/juju/errors,
ConstError, Annotate and Annotatef (prefix + wrapped slot), ParamsErrorf of
apiserver/errors (the format string is its second argument), OperationBlocked-
Error of apiserver/errors (the message, and its default whenever it is called),
and the logger Errorf and Warningf calls the workers document (their first
argument is a context). Not extracted (recorded limitations): messages
composed from variables or non-literal formats, and codes stated only in
prose outside the entry's own note.

Usage: check_error_contract.py [--source DIR] [--docs DIR] [--modules DIR]
--source defaults to the repository root (two levels up from this script);
--docs defaults to the reference directory beside the page sources;
--modules defaults to the Go module cache ($GOMODCACHE, else `go env
GOMODCACHE`, else ~/go/pkg/mod).
Exit status 1 when any documented message or code is unverified.
"""
import argparse
import os
import re
import subprocess
import sys

# The suffix github.com/juju/errors appends to each *f constructor's message
# (errortypes.go: the ConstError family; separator " " when the wrapped error
# carries a message, which a ConstError always does).
FAMILY = {
    "NotFoundf": "not found",
    "UserNotFoundf": "user not found",
    "Unauthorizedf": "unauthorized",
    "NotImplementedf": "not implemented",
    "AlreadyExistsf": "already exists",
    "NotSupportedf": "not supported",
    "NotValidf": "not valid",
    "NotProvisionedf": "not provisioned",
    "NotAssignedf": "not assigned",
    "BadRequestf": "bad request",
    "MethodNotAllowedf": "method not allowed",
    "Forbiddenf": "forbidden",
    "QuotaLimitExceededf": "quota limit exceeded",
    "NotYetAvailablef": "not yet available",
    "Timeoutf": "timeout",
}
ERRORS_PACKAGES = (
    "github.com/juju/errors",
    "github.com/juju/juju/internal/errors",
)
APISERVER_ERRORS = "github.com/juju/juju/apiserver/errors"

MSG = re.compile(r"\*\*`([^`]+)`\*\*")
NON_ERRORS = {"file", "oci-image", "store", "upload", "available", "potential"}
VERB = r"%[-+ #0]*\d*(?:\.\d+)?[a-zA-Z]"
VERB_RE = re.compile(r"%%|%q|" + VERB)
WS = re.compile(r"\s+")

# One Go string literal (double-quoted or raw), and a run of literals joined
# by `+` (a format string split across lines).
LIT = r'(?:"(?:[^"\\]|\\.)*"|\x60[^\x60]*\x60)'
LITRUN = re.compile(LIT + r"(?:(?:\s*\+\s*)" + LIT + r")*")
LITPIECE = re.compile(r'"((?:[^"\\]|\\.)*)"|\x60([^\x60]*)\x60')

# Documented slots: <name> bare, "<name>" quoted, and a literal %q the page
# quotes verbatim from the source. A slot's content starts with a letter, so
# inequalities such as `scale < 0` stay literal text.
DOC_SLOT = re.compile(r"%q|\"<[A-Za-z][^\"<>]*>\"|<[A-Za-z][^<>]*>")

SLOT_RE = r".*?"
QSLOT_RE = r'"[^"]*"'
MARK = "\x01"        # a bare slot in a documented message
QMARK = '"\x02"'     # a quoted slot in a documented message
TRUNC = "\x03"       # the page truncates the message with `...`
TERMINALS = (MARK, QMARK)

# Documented messages the extractor cannot verify, each with its reason.
# Keep this list short and auditable; every entry names where the message
# comes from.
EXCLUSIONS = {
    # The page prints the API error code (params.CodeOperationBlocked), not a
    # message; the operator's block message is the message. The code check
    # verifies the code.
    "operation is blocked": "the page prints the error code, not a message",
    # The page prints Charmhub's own API error, which this tree does not raise.
    "<code>: <message>": "Charmhub's API error, passed through by the facade",
    # The page omits the suffix a github.com/juju/errors constructor renders.
    "charm \"<url>\" not available":
        "page omits the rendered suffix; raised as "
        "NotYetAvailablef(\"charm %q not available\") "
        "(apiserver/facades/agent/uniter/uniter.go:1140)",
    "current model is not the controller model":
        "page omits the rendered suffix; raised as BadRequestf "
        "(apiserver/facades/client/controller/destroy.go:40)",
    "a relation should have exactly two endpoints":
        "page omits the rendered suffix; raised as BadRequestf "
        "(apiserver/facades/client/application/application.go:1428)",
    "endpoints [\"<name 1>\" \"<name 2>\"] are ambiguous, "
    "specify relation endpoints or id":
        "page omits the rendered suffix and splits the %q slice; raised as "
        "BadRequestf(\"endpoints %q are ambiguous, specify relation "
        "endpoints or id\")",
    "cannot resolve all units and specific units":
        "page omits the rendered suffix; raised as BadRequestf "
        "(apiserver/facades/client/application/application.go:1889)",
    "unsupported method: \"<method>\"":
        "page omits the rendered suffix; raised as MethodNotAllowedf "
        "(apiserver/internal/handlers/resources/resources.go:128)",
    # The page writes the message the source does not carry verbatim.
    "\"<level>\" <type> access not valid":
        "page paraphrase; no verbatim raise site in the source",
    # The page's own shorthand for the wrap prefix of the previous entry.
    "...: machine has units":
        "page shorthand for the wrap prefix of the previous entry",
    # The page writes the caller's wrap prefix as a slot.
    "<prefix>: invalid hash prefix":
        "raised as ConstError \"invalid hash prefix\" "
        "(domain/objectstore/errors/errors.go:32) with a caller-supplied "
        "prefix the page writes as a slot",
    # Not an error message: a bolded tag kind.
    "user tag": "not a message; the page bolds a tag kind",
}


def unescape(s):
    """Unfold the escapes of a Go string literal's content."""
    return s.replace(r"\"", '"').replace(r"\n", "\n").replace(r"\t", "\t") \
            .replace(r"\\", "\\")


def content(match):
    """The format string of a literal run match."""
    out = []
    for m in LITPIECE.finditer(match.group(0)):
        out.append(m.group(1) if m.group(1) is not None else m.group(2))
    return unescape("".join(out))


def skel(template, quoted=True):
    """Turn a Go format string into a regular expression."""
    template = WS.sub(" ", template)
    out, i = [], 0
    for m in VERB_RE.finditer(template):
        out.append(re.escape(template[i:m.start()]))
        if m.group(0) == "%%":
            out.append("%")
        elif m.group(0) == "%q":
            out.append(QSLOT_RE if quoted else SLOT_RE)
        else:
            out.append(SLOT_RE)
        i = m.end()
    out.append(re.escape(template[i:]))
    return "".join(out)


def doc_marker(message):
    """Turn a documented message into a marked concrete string."""
    trunc = ""
    m = re.search(r"(?:\.\.\.|…)[\s]*$", message)
    if m:
        message, trunc = message[:m.start()], TRUNC
    message = WS.sub(" ", message)
    out, i = [], 0
    for m in DOC_SLOT.finditer(message):
        out.append(message[i:m.start()])
        out.append(QMARK if m.group(0) == "%q" or m.group(0)[0] == '"' else MARK)
        i = m.end()
    out.append(message[i:])
    return "".join(out) + trunc


def parse_imports(text):
    """Map each import's local name to its package path."""
    aliases = {}
    for m in re.finditer(r'^\s*import\s+(?:([\w./]+)\s+)?"([^"\s]+)"', text,
                         re.M):
        alias, path = m.group(1), m.group(2)
        if alias is None or alias == "_":
            alias = path.rsplit("/", 1)[-1]
        aliases[alias] = path
    for m in re.finditer(r'^\s*("([^"\s]+)")([\s]*//[^\n]*)?$', text, re.M):
        aliases[m.group(2).rsplit("/", 1)[-1]] = m.group(2)
    for m in re.finditer(r'^\s*([\w./]+)\s+("([^"\s]+)")', text, re.M):
        aliases[m.group(1)] = m.group(3)
    return aliases


def module_cache():
    """The Go module cache directory, or None."""
    cache = os.environ.get("GOMODCACHE")
    if not cache:
        try:
            cache = subprocess.run(["go", "env", "GOMODCACHE"],
                                    capture_output=True, text=True,
                                    timeout=30).stdout.strip()
        except (OSError, subprocess.SubprocessError):
            cache = ""
    return cache or os.path.expanduser("~/go/pkg/mod")


def juju_modules(source):
    """The github.com/juju/* dependency directories of this checkout."""
    try:
        text = open(os.path.join(source, "go.mod"), encoding="utf-8").read()
    except OSError:
        return []
    out, cache = [], module_cache()
    for m in re.finditer(r"(github.com/juju/[\w/.]+)\s+(v[\w.\-]+)", text):
        path = os.path.join(cache, f"{m.group(1)}@{m.group(2)}")
        if os.path.isdir(path):
            out.append(path)
    return out


def corpus(source):
    """Extract raise-site format strings from the Go source."""
    out = set()
    family_re = re.compile(r"\b(" + "|".join(FAMILY) +
                           r")\(\s*" + LITRUN.pattern)
    roots = [source] + juju_modules(source)
    for root in roots:
        walk_root(root, out, family_re)
    return out


def walk_root(source, out, family_re):
    """Add the raise-site format strings of one source tree."""
    for root, dirs, files in os.walk(source):
        dirs[:] = [d for d in dirs if d not in (".git", "vendor", "mocks",
                                                "testdata")]
        for name in files:
            if not name.endswith(".go") or name.endswith("_test.go"):
                continue
            path = os.path.join(root, name)
            try:
                text = open(path, encoding="utf-8",
                           errors="replace").read()
            except OSError:
                continue
            imports = parse_imports(text)
            for a, p in imports.items():
                if p == APISERVER_ERRORS:
                    # ParamsErrorf(code, format, ...) raises the format
                    # string with the code attached.
                    for m in re.finditer(
                            re.escape(a) + r"\.ParamsErrorf\(\s*[^,]+,\s*"
                            + LITRUN.pattern, text):
                        out.add(content(m))
                    # OperationBlockedError(msg) raises the message, and
                    # its default whenever it is called with none.
                    if re.search(re.escape(a) +
                                 r"\.OperationBlockedError\(", text):
                        out.add("the operation has been blocked")
                    for m in re.finditer(
                            re.escape(a) + r"\.OperationBlockedError\(\s*"
                            + LITRUN.pattern, text):
                        out.add(content(m))
                if p not in ERRORS_PACKAGES:
                    continue
                a = re.escape(a) + r"\."
                for m in re.finditer(
                        a + r"(?:New|Newf|Errorf)\(\s*" + LITRUN.pattern,
                        text):
                    out.add(content(m))
                for m in re.finditer(a + r"ConstError\(\s*" + LITRUN.pattern,
                                     text):
                    out.add(content(m))
                for m in re.finditer(
                        a + r"Annotate(?:f)?\([^,]+,\s*" + LITRUN.pattern,
                        text):
                    # Annotate renders `prefix + ": " + inner`; model the
                    # wrapped inner as a verb so the template heads a chain.
                    out.add(content(m) + ": %v")
                for m in re.finditer(
                        a + r"New(?:Timeout|NotFound|UserNotFound|"
                        r"Unauthorized|NotImplemented|AlreadyExists|"
                        r"NotSupported|NotValid|NotProvisioned|NotAssigned|"
                        r"BadRequest|MethodNotAllowed|Forbidden|"
                        r"QuotaLimitExceeded|NotYetAvailable)"
                        r"\([^,]+,\s*" + LITRUN.pattern, text):
                    # NewX(err, msg) renders `msg: inner`, or the bare msg
                    # when err is nil; both shapes are raisable.
                    out.add(content(m))
                    out.add(content(m) + ": %v")
            # HTTP handlers answer with the message literal directly.
            for m in re.finditer(r"\bhttp\.Error\(\s*[^,]+,\s*"
                                 + LITRUN.pattern, text):
                out.add(content(m))
            # The SSH server rejects channels and writes pre-banner errors
            # with the message literal directly.
            for m in re.finditer(
                    r"\w+\.rejectChannel\(\s*[^,]+,\s*[^,]+,\s*"
                    + LITRUN.pattern, text):
                out.add(content(m))
            for m in re.finditer(r"\w+\.WritePreBannerError\(\s*[^,]+,\s*"
                                 + LITRUN.pattern, text):
                out.add(content(m))
            # The migration master reports its status messages through a
            # setErrorStatus wrapper; its format strings are client-visible.
            for m in re.finditer(r"\w+\.setErrorStatus\(\s*[^,]+,\s*"
                                 + LITRUN.pattern, text):
                c = content(m)
                if "%" in c:
                    out.add(c)
            if "fmt" in imports:
                for m in re.finditer(r"\bfmt\.(?:Errorf|Sprintf)\(\s*"
                                     + LITRUN.pattern, text):
                    out.add(content(m))
            # A params.Error literal carries its message in the Message field.
            for m in re.finditer(r"\bMessage:\s*" + LITRUN.pattern, text):
                out.add(content(m))
            # The workers' pages document what the workers log. A logger call
            # passes a context first, so only a format string that carries a
            # verb is taken (an argument literal would carry none).
            for m in re.finditer(r"\w+\.(?:Errorf|Warningf)\([^,]+,\s*"
                                 + LITRUN.pattern, text):
                c = content(m)
                if "%" in c:
                    out.add(c)
            for m in family_re.finditer(text):
                out.add(content(m) + " " + FAMILY[m.group(1)])


def build(corp):
    """Compile the corpus into matchers, indexed by leading literal word.

    Templates with no literal word (a bare `%s`, a bare `%w`, ...) match any
    message and so evidence nothing; they are dropped. A second, loose set
    of matchers additionally treats a quoted source verb (`%q`) as a bare
    slot, for pages that write the slot without quotes.
    """
    full, chain, loose = {}, {}, {}
    for template in corp:
        lit = re.sub(VERB, "", template.replace("%%", "%"))
        if not re.search(r"[A-Za-z]{2}", lit):
            continue
        head = VERB_RE.search(template)
        key = ("" if head and head.start() == 0
               else WS.sub(" ",
                           template[:head.start() if head else None])
               ).split(" ")[0]
        full.setdefault(key, []).append(re.compile(skel(template)))
        if "%q" in template:
            loose.setdefault(key, []).append(
                re.compile(skel(template, quoted=False)))
        m = None
        for m in VERB_RE.finditer(template):
            pass
        if m is not None and m.end() == len(template) and m.group(0) != "%q":
            prefix = skel(template[:m.start()])
            if prefix:
                chain.setdefault(key, []).append(re.compile(prefix))
    return full, chain, loose


def candidates(D, index):
    """Corpus buckets whose leading literal word can start the message."""
    D = D.rstrip(TRUNC) if D.endswith(TRUNC) else D
    stops = [s for s in (D.find(MARK), D.find(QMARK)) if s != -1]
    word = D[:min(stops) if stops else len(D)].split(" ")[0].split("\n")[0]
    return index.get(word, []) + index.get("", [])


def matches(D, full, chain, loose, cache, depth=0):
    """Does a marked message match a raise-site template (or a chain)?"""
    if D in cache:
        return cache[D]
    cache[D] = False  # cycle guard
    trunc = D.endswith(TRUNC)
    ok = False
    if trunc:
        # A message the page truncates matches any template it starts.
        for index in (full, loose):
            for R in candidates(D, index):
                if R.match(D[:-len(TRUNC)]):
                    ok = True
                    break
            if ok:
                break
    if not ok:
        for index in (full, loose):
            for R in candidates(D, index):
                if R.fullmatch(D):
                    ok = True
                    break
            if ok:
                break
        if not ok and depth < 4:
            for R in candidates(D, chain):
                m = R.match(D)
                if m is None:
                    continue
                rest = D[m.end():]
                if rest in TERMINALS or rest == TRUNC or (rest and matches(
                        rest, full, chain, loose, cache, depth + 1)):
                    ok = True
                    break
    cache[D] = ok
    return ok


def parse_entries(lines):
    """The error-entry grammar of the pages (as tools/count_errors.py)."""
    entries, in_errors = [], False
    for n, line in enumerate(lines, 1):
        s = line.strip()
        if s.startswith("- *Errors:*") or s.startswith("- **Errors:**"):
            in_errors = True
            continue
        if re.match(r"#{2,3} ", s):
            in_errors = False
            continue
        found = MSG.findall(line)
        if not found:
            continue
        if in_errors:
            entries.extend((n, m) for m in found)
        elif re.match(r"\s*[-*] ", line):
            entries.extend((n, m) for m in found if m not in NON_ERRORS)
    return entries


def parse_codes(lines):
    """The error code each entry notes, where the note is machine-shaped."""
    codes = {}
    for n, line in enumerate(lines, 1):
        for m in MSG.finditer(line):
            cm = re.match(r"\s*(?:\(([0-9]{3})\)|\(code `([^`]+)`\))",
                          line[m.end():])
            if cm:
                codes[(n, m.group(1))] = cm.group(1) or cm.group(2)
    return codes


def params_codes(source):
    """The code strings the API server can answer with."""
    path = os.path.join(source, "rpc/params/apierror.go")
    try:
        text = open(path, encoding="utf-8").read()
    except OSError:
        return set()
    return set(re.findall(r'Code\w+\s*=\s*"([^"]+)"', text))


def main():
    here = os.path.dirname(os.path.abspath(__file__))
    ap = argparse.ArgumentParser()
    ap.add_argument("--source",
                    default=os.path.dirname(os.path.dirname(here)),
                    help="repository root holding the Go source")
    ap.add_argument("--docs",
                    default=os.path.join(os.path.dirname(here), "reference"),
                    help="directory holding the reference pages")
    ap.add_argument("--modules", default=None,
                    help="the Go module cache directory")
    args = ap.parse_args()
    if args.modules:
        os.environ["GOMODCACHE"] = args.modules

    corp = corpus(args.source)
    full, chain, loose = build(corp)
    print(f"raise-site templates: {len(corp)} "
          f"({sum(len(v) for v in chain.values())} wrap-chain heads)")
    params = params_codes(args.source)
    codes_vocab = params | {str(s) for s in range(400, 600)}
    print(f"error codes: {len(params)} params codes")

    cache = {}
    bad, bad_codes, excluded, total = [], [], [], 0
    for page in sorted(os.listdir(args.docs)):
        if not page.endswith(".md"):
            continue
        lines = open(os.path.join(args.docs, page),
                     encoding="utf-8").read().splitlines()
        entries = parse_entries(lines)
        noted = parse_codes(lines)
        total += len(entries)
        for n, message in entries:
            if message in EXCLUSIONS:
                excluded.append((page, n, message))
                continue
            if not matches(doc_marker(message), full, chain, loose, cache):
                bad.append((page, n, message))
            code = noted.get((n, message))
            if code is not None and code not in codes_vocab:
                bad_codes.append((page, n, code, message))
    print(f"entries: {total}, excluded: {len(excluded)}, "
          f"unmatched messages: {len(bad)}, unknown codes: {len(bad_codes)}")
    for page, n, message in bad:
        print(f"UNMATCHED {page}:{n}: {message}")
    for page, n, code, message in bad_codes:
        print(f"UNKNOWN-CODE {page}:{n}: ({code}) {message}")
    if bad or bad_codes:
        sys.exit(1)


if __name__ == "__main__":
    main()