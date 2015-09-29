#!/usr/bin/env python3
from argparse import ArgumentParser
import json

from utils import dump_json_pretty


def parse_args():
    parser = ArgumentParser(description='Convert sstream-query output to JSON')
    parser.add_argument('input')
    parser.add_argument('output')
    return parser.parse_args()


def main():
    args = parse_args()
    output = []
    with open(args.input) as in_file:
        for line in in_file:
            output.append(eval(line))
    with open(args.output, 'w') as out_file:
        dump_json_pretty(output, out_file)


if __name__ == '__main__':
    main()
