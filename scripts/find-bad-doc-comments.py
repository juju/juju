#!/usr/bin/python

"""
This quick-and-dirty tool locates Go doc comments in a source tree
that don't follow the convention of the first word of the comment
matching the function name. It highlights cases where doc comments
haven't been updated in step with function name changes or where doc
comments have been copied and pasted but not updated.

By default, all problems found are emitted but there is also an
interactive edit mode is available via --fix.
"""

import argparse
import fnmatch
import os
import re
import subprocess
from os import path

def find_go_files(root):
    for directory, _, files in os.walk(root):
        for filename in fnmatch.filter(files, '*.go'):
            yield path.join(directory, filename)

DOC_COMMENT_PATT = '\n\n//.+\n(//.+\n)*func.+\n'
FIRST_WORD_PATT = '// *(\w+)'
FUNC_NAME_PATT = 'func(?: \([^)]+\))? (\S+)\('

def extract_doc_comments(text):
    for match in re.finditer(DOC_COMMENT_PATT, text, re.MULTILINE):
        yield match.group(0).strip()

def find_bad_doc_comments(comments):
    for comment in comments:
        lines = comment.splitlines()
        first_word_match = re.match(FIRST_WORD_PATT, lines[0])
        if first_word_match:
            first_word = first_word_match.group(1)
            func_name = re.match(FUNC_NAME_PATT, lines[-1]).group(1)
            if first_word != func_name:
                yield func_name, comment

def cmdline():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--fix', default=False, action='store_true',
                        help='Interactive fix-up mode')
    parser.add_argument('root', nargs='?', default=os.getcwd())
    return parser.parse_args()

def emit(filename, comment):
    print
    print '%s: ' % filename
    print comment

def fix(filename, func_name, comment):
    emit(filename, comment)
    resp = raw_input('Fix? [Y/n] ').strip().lower()
    if resp in ('', 'y'):
        subprocess.check_call(['vim', '-c', '/func .*'+func_name+'(', filename])

def main():
    args = cmdline()

    count = 0
    for filename in find_go_files(args.root):
        with open(filename) as sourceFile:
            source = sourceFile.read()
        comments = extract_doc_comments(source)
        for func_name, bad_comment in find_bad_doc_comments(comments):
            if args.fix:
                fix(filename, func_name, bad_comment)
            else:
                emit(filename, bad_comment)
            count += 1

    print
    print "Problems found:", count

if __name__ == '__main__':
    main()
