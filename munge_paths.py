#!/usr/bin/env python3
from argparse import ArgumentParser
import json
import sys

from utils import dump_json_pretty


def main():
    parser = ArgumentParser()
    parser.add_argument('input')
    parser.add_argument('output')
    args = parser.parse_args()
    with open(args.input) as input_file:
        stanzas = json.load(input_file)
    hashes = {}
    for stanza in stanzas:
        hashes[stanza['sha256']] = stanza['path']
    for stanza in stanzas:
        stanza['path'] = hashes[stanza['sha256']]
    with open(args.output, 'w') as output_file:
        dump_json_pretty(stanzas, output_file)

if __name__ == '__main__':
    sys.exit(main())
