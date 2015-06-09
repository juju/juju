// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.
//
// +build windows

package securestring

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"syscall"
	"unsafe"

	"github.com/juju/errors"
)

//sys protectData(input uintptr, szDataDescr uint32, entropy uintptr, reserved uint32, prompt uint32, flags uint, output uintptr) (err error) [failretval==0] = crypt32.CryptProtectData
//sys unprotectData(input uintptr, szDataDescr uint32, entropy uintptr, reserved uint32, prompt uint32, flags uint, output uintptr) (err error) [failretval==0] = crypt32.CryptUnprotectData

// blob is the struct type we shall be making the syscalls on, it contains a
// pointer to the start of the actual data and its respective length in bytes
type blob struct {
	length uint32
	data   *byte
}

// getData returns an uint16 array that contains the data pointed to by blob.data
// The return value of this function will be passed to syscall.UTF16ToString
// to return the plain text result
func (b *blob) getData() []uint16 {
	data := (*[1 << 16]uint16)(unsafe.Pointer(b.data))[:b.length/2]
	return data
}

// getDataAsBytes returns a byte array with the data pointed to by blob.data
func (b *blob) getDataAsBytes() []byte {
	data := (*[1 << 30]byte)(unsafe.Pointer(b.data))[:b.length]
	return data
}

// convertToUTF16 converts the utf8 string to utf16
func convertToUTF16(a string) ([]byte, error) {
	u16, err := syscall.UTF16FromString(a)
	if err != nil {
		return nil, errors.Annotate(err, "Failed to convert string to UTF16")
	}
	buf := &bytes.Buffer{}
	if err := binary.Write(buf, binary.LittleEndian, u16); err != nil {
		return nil, errors.Annotate(err, "Failed to convert UTF16 to bytes")
	}
	return buf.Bytes(), nil
}

// Encrypt encrypts a string provided as input into a hexadecimal string
// the output corresponds to the output of ConvertFrom-SecureString:
func Encrypt(input string) (string, error) {
	// we need to convert UTF8 to UTF16 before sending it into CryptProtectData
	// to be compatible with the way powershell does it
	data, err := convertToUTF16(input)
	if err != nil {
		return "", err
	}
	inputBlob := blob{uint32(len(data)), &data[0]}
	outputBlob := blob{}

	err = protectData(uintptr(unsafe.Pointer(&inputBlob)), 0, 0, 0, 0, 0, uintptr(unsafe.Pointer(&outputBlob)))
	if err != nil {
		return "", fmt.Errorf("Failed to encrypt %s, error: %s", input, err)
	}
	defer syscall.LocalFree((syscall.Handle)(unsafe.Pointer(outputBlob.data)))
	output := outputBlob.getDataAsBytes()
	// the result is a slice of bytes, which we must encode into hexa
	// to match ConvertFrom-SecureString's output before returning it
	h := hex.EncodeToString([]byte(output))
	return h, nil
}

// Decrypt converts the output from a call to ConvertFrom-SecureString
// back to the original input string and returns it
func Decrypt(input string) (string, error) {
	// first we decode the hexadecimal string into a raw slice of bytes
	data, err := hex.DecodeString(input)
	if err != nil {
		return "", err
	}
	inputBlob := blob{uint32(len(data)), &data[0]}
	outputBlob := blob{}

	err = unprotectData(uintptr(unsafe.Pointer(&inputBlob)), 0, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(&outputBlob)))
	if err != nil {
		return "", fmt.Errorf("Failed to decrypt %s, error: &s", input, err)
	}
	defer syscall.LocalFree((syscall.Handle)(unsafe.Pointer(outputBlob.data)))

	a := outputBlob.getData()
	return syscall.UTF16ToString(a), nil
}
