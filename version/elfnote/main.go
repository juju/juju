// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"github.com/juju/juju/version"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintf(os.Stderr, "%s <note section file> <GOARCH>\n", os.Args[0])
		os.Exit(1)
		return
	}

	var order binary.ByteOrder
	switch os.Args[2] {
	case "amd64", "arm64", "ppc64el", "ppc64le":
		order = binary.LittleEndian
	case "s390x":
		order = binary.BigEndian
	default:
		fmt.Fprintf(os.Stderr, "unexpected arch string %s", os.Args[2])
		os.Exit(1)
		return
	}

	f, err := os.Create(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to open file: %s", err.Error())
		os.Exit(1)
		return
	}

	err = writeElfNote(f, order)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to write elf note: %s", err.Error())
		os.Exit(1)
		return
	}

	err = f.Close()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to close file: %s", err.Error())
		os.Exit(1)
		return
	}

	os.Exit(0)
}

const (
	ELF_NOTE_TAG = 0
)

var (
	ELF_NOTE_NAME = []byte("Juju Version")
)

func round4(sz int) (int, int) {
	if sz%4 == 0 {
		return sz, 0
	}
	padding := 4 - (sz % 4)
	return sz + padding, padding
}

func writeElfNote(w io.Writer, order binary.ByteOrder) error {
	verStr := version.Current.String()
	nameSize, namePadding := round4(len(ELF_NOTE_NAME))
	descSize, descPadding := round4(len(verStr))
	if err := binary.Write(w, order, uint32(nameSize)); err != nil {
		return err
	}
	if err := binary.Write(w, order, uint32(descSize)); err != nil {
		return err
	}
	if err := binary.Write(w, order, uint32(ELF_NOTE_TAG)); err != nil {
		return err
	}
	if _, err := w.Write(ELF_NOTE_NAME); err != nil {
		return err
	}
	if _, err := w.Write([]byte{0, 0, 0, 0}[:namePadding]); err != nil {
		return err
	}
	if _, err := w.Write([]byte(verStr)); err != nil {
		return err
	}
	if _, err := w.Write([]byte{0, 0, 0, 0}[:descPadding]); err != nil {
		return err
	}
	return nil
}
