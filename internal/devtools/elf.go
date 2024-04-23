// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package devtools

import (
	"bytes"
	"debug/elf"
	"encoding/binary"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/version/v2"
)

const (
	ErrNoVersion   errors.ConstError = "no version note on ELF"
	ErrInvalidNote errors.ConstError = "invalid version note"
)

// ELFExtractVersion reads an ELF file, extracting the ".note.juju.version" section
// and decoding the note which has the juju version in the description.
func ELFExtractVersion(path string) (version.Binary, error) {
	bin, err := elf.Open(path)
	if err != nil {
		return version.Binary{}, err
	}

	res := version.Binary{
		Release: "ubuntu",
	}
	// This is only for dev builds, so just assume the binaries are right for
	// the architecture based on just the ELF machine.
	switch bin.Machine {
	case elf.EM_X86_64:
		res.Arch = "amd64"
	case elf.EM_AARCH64:
		res.Arch = "arm64"
	case elf.EM_S390:
		if bin.Class == elf.ELFCLASS32 {
			return version.Binary{}, fmt.Errorf("s390: 32 bit not supported")
		}
		res.Arch = "s390x"
	case elf.EM_PPC64:
		if bin.ByteOrder == binary.BigEndian {
			return version.Binary{}, fmt.Errorf("ppc64: big endian not supported")
		}
		res.Arch = "ppc64el"
	}

	versionNote := bin.Section(".note.juju.version")
	if versionNote == nil {
		return res, ErrNoVersion
	}

	note, err := readELFNote(versionNote, bin.ByteOrder)
	if err != nil {
		return res, err
	}

	versionStr := string(bytes.TrimRight(note.Description, "\x00"))
	res.Number, err = version.Parse(versionStr)
	if err != nil {
		return res, err
	}

	return res, nil
}

type elfNote struct {
	Tag         uint32
	Name        []byte
	Description []byte
}

func readELFNote(section *elf.Section, byteOrder binary.ByteOrder) (*elfNote, error) {
	if section.Type != elf.SHT_NOTE {
		return nil, ErrInvalidNote
	}

	note := &elfNote{}
	reader := section.Open()

	nameSize := uint32(0)
	err := binary.Read(reader, byteOrder, &nameSize)
	if err != nil {
		return nil, err
	}

	descSize := uint32(0)
	err = binary.Read(reader, byteOrder, &descSize)
	if err != nil {
		return nil, err
	}

	tag := uint32(0)
	err = binary.Read(reader, byteOrder, &tag)
	if err != nil {
		return nil, err
	}
	note.Tag = tag

	note.Name = make([]byte, nameSize)
	read, err := reader.Read(note.Name)
	if err != nil {
		return nil, err
	} else if read != int(nameSize) {
		return nil, ErrInvalidNote
	}

	note.Description = make([]byte, descSize)
	read, err = reader.Read(note.Description)
	if err != nil {
		return nil, err
	} else if read != int(descSize) {
		return nil, ErrInvalidNote
	}

	return note, nil
}
