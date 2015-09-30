#!/usr/bin/env python3
from argparse import ArgumentParser
import json
import os.path
import re
import sys

from generate_simplestreams import json_dump


def main():
    parser = ArgumentParser()
    parser.add_argument('input')
    parser.add_argument('output')
    args = parser.parse_args()
    paths_hashes = {}
    with open(args.input) as input_file:
        stanzas = json.load(input_file)
    hashes = {}
    for stanza in stanzas:
        path = os.path.join('agent', os.path.basename(stanza['path']))
        path = re.sub('-win(2012(hv)?(r2)?|7|8|81)-', '-windows-', path)
        path_hash = stanza['sha256']
        paths_hashes.setdefault(path, stanza['sha256'])
        if paths_hashes[path] != path_hash:
            raise ValueError('Conflicting hash')
        stanza['path'] = path
        hashes[path] = path_hash
    ph_list = {}
    for path, path_hash in hashes.items():
        ph_list.setdefault(path_hash, set()).add(path)
    for path_hash, paths in ph_list.items():
        if len(paths) > 1:
            print(paths)
    json_dump(stanzas, args.output)

if __name__ == '__main__':
    sys.exit(main())
