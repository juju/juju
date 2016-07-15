// Copied from https://github.com/ricochet2200/go-disk-usage
// Copyright 2011 Rick Smith.
// Use of this source code is governed by a public domain
// license that can be found in the LICENSE.ricochet2200 file.
//

package du

import (
	"syscall"
	"unsafe"
)

type DiskUsage struct {
	freeBytes  int64
	totalBytes int64
	availBytes int64
}

// Returns an object holding the disk usage of volumePath
// This function assumes volumePath is a valid path
func NewDiskUsage(volumePath string) *DiskUsage {

	h := syscall.MustLoadDLL("kernel32.dll")
	c := h.MustFindProc("GetDiskFreeSpaceExW")

	du := &DiskUsage{}

	c.Call(
		uintptr(unsafe.Pointer(syscall.StringToUTF16Ptr(volumePath))),
		uintptr(unsafe.Pointer(&du.freeBytes)),
		uintptr(unsafe.Pointer(&du.totalBytes)),
		uintptr(unsafe.Pointer(&du.availBytes)))

	return du
}

// Total free bytes on file system
func (this *DiskUsage) Free() uint64 {
	return uint64(this.freeBytes)
}

// Total available bytes on file system to an unpriveleged user
func (this *DiskUsage) Available() uint64 {
	return uint64(this.availBytes)
}

// Total size of the file system
func (this *DiskUsage) Size() uint64 {
	return uint64(this.totalBytes)
}

// Total bytes used in file system
func (this *DiskUsage) Used() uint64 {
	return this.Size() - this.Free()
}

// Percentage of use on the file system
func (this *DiskUsage) Usage() float32 {
	return float32(this.Used()) / float32(this.Size())
}
