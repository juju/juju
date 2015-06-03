// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.
//
// +build windows

package securestring

import (
	"encoding/hex"
	"fmt"
	"syscall"
	"unsafe"
)

//sys protectData(input uintptr, szDataDescr uint32, entropy uintptr, reserved uint32, prompt uint32, flags uint, output uintptr) (err error) [failretval==0] = crypt32.CryptProtectData
//sys unprotectData(input uintptr, szDataDescr uint32, entropy uintptr, reserved uint32, prompt uint32, flags uint, output uintptr) (err error) [failretval==0] = crypt32.CryptUnprotectData

// blob is the struct type we shall be making the syscalls on, it contains a
// pointer to the start of the actual data and its respective length in bytes
type blob struct {
	length uint32
	data   *byte
}

// getData fetches all the data pointed to by blob.data
func (b *blob) getData() []byte {
	fetched := make([]byte, b.length)
	// the in-built will copy the proper amount of data pointed to by blob.data
	// and put it in the new variable
	// 1 << 30 is the largest possible slice size; it's pretty overkill but it
	// ensures we can read as most of very large data as physically possible
	copy(fetched, (*[1 << 30]byte)(unsafe.Pointer(b.data))[:])
	return fetched
}

// Encrypt encrypts a string provided as input into a hexadecimal string
// the output corresponds to the output of ConvertFrom-SecureString:
func Encrypt(input string) (string, error) {
	data := []byte(input)

	// for some reason the cmdlet's calls automatically encrypts the bytes
	// with interwoven nulls, so we must account for this as follows:
	nulled := []byte{}
	for _, b := range data {
		nulled = append(nulled, b)
		nulled = append(nulled, 0)
	}

	inputBlob := blob{uint32(len(nulled)), &nulled[0]}
	entropyBlob := blob{}
	outputBlob := blob{}
	dwflags := 1

	err := protectData(uintptr(unsafe.Pointer(&inputBlob)), 0, uintptr(unsafe.Pointer(&entropyBlob)), 0, 0, uint(dwflags), uintptr(unsafe.Pointer(&outputBlob)))
	if err != nil {
		return "", fmt.Errorf("Failed to encrypt %s, error: %s", input, err)
	}
	defer syscall.LocalFree((syscall.Handle)(unsafe.Pointer(outputBlob.data)))

	output := outputBlob.getData()
	// the result is a slice of bytes, which we must encode into hexa
	// to match ConvertFrom-SecureString's output before returning it
	return hex.EncodeToString(output), nil
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
	entropyBlob := blob{}
	outputBlob := blob{}
	dwflags := 1

	err = unprotectData(uintptr(unsafe.Pointer(&inputBlob)), 0, uintptr(unsafe.Pointer(&entropyBlob)), 0, 0, uint(dwflags),
		uintptr(unsafe.Pointer(&outputBlob)))
	if err != nil {
		return "", fmt.Errorf("Failed to decrypt %s, error: &s", input, err)
	}
	defer syscall.LocalFree((syscall.Handle)(unsafe.Pointer(outputBlob.data)))

	output := outputBlob.getData()
	// as mentioned, the commandlet infers working with data with interwoven
	// nulls, for which we must account for by removing them now:
	clean := []byte{}
	for _, b := range output {
		if b != 0 {
			clean = append(clean, b)
		}
	}

	return string(clean), nil
}
