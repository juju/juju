package cidrman

import (
	"encoding/binary"
	"fmt"
	"net"
	"sort"
)

// ipv4ToUInt32 converts an IPv4 address to an unsigned 32-bit integer.
func ipv4ToUInt32(ip net.IP) uint32 {
	return binary.BigEndian.Uint32(ip)
}

// uint32ToIPV4 converts an unsigned 32-bit integer to an IPv4 address.
func uint32ToIPV4(addr uint32) net.IP {
	ip := make([]byte, net.IPv4len)
	binary.BigEndian.PutUint32(ip, addr)
	return ip
}

// The following functions are inspired by http://www.cs.colostate.edu/~somlo/iprange.c.

// setBit sets the specified bit in an address to 0 or 1.
func setBit(addr uint32, bit uint, val uint) uint32 {
	if bit < 0 {
		panic("negative bit index")
	}

	if val == 0 {
		return addr & ^(1 << (32 - bit))
	} else if val == 1 {
		return addr | (1 << (32 - bit))
	} else {
		panic("set bit is not 0 or 1")
	}
}

// netmask returns the netmask for the specified prefix.
func netmask(prefix uint) uint32 {
	if prefix == 0 {
		return 0
	}
	return ^uint32((1 << (32 - prefix)) - 1)
}

// broadcast4 returns the broadcast address for the given address and prefix.
func broadcast4(addr uint32, prefix uint) uint32 {
	return addr | ^netmask(prefix)
}

// network4 returns the network address for the given address and prefix.
func network4(addr uint32, prefix uint) uint32 {
	return addr & netmask(prefix)
}

// splitRange4 recursively computes the CIDR blocks to cover the range lo to hi.
func splitRange4(addr uint32, prefix uint, lo, hi uint32, cidrs *[]*net.IPNet) error {
	if prefix > 32 {
		return fmt.Errorf("Invalid mask size: %d", prefix)
	}

	bc := broadcast4(addr, prefix)
	if (lo < addr) || (hi > bc) {
		return fmt.Errorf("%d, %d out of range for network %d/%d, broadcast %d", lo, hi, addr, prefix, bc)
	}

	if (lo == addr) && (hi == bc) {
		cidr := net.IPNet{IP: uint32ToIPV4(addr), Mask: net.CIDRMask(int(prefix), 8*net.IPv4len)}
		*cidrs = append(*cidrs, &cidr)
		return nil
	}

	prefix++
	lowerHalf := addr
	upperHalf := setBit(addr, prefix, 1)
	if hi < upperHalf {
		return splitRange4(lowerHalf, prefix, lo, hi, cidrs)
	} else if lo >= upperHalf {
		return splitRange4(upperHalf, prefix, lo, hi, cidrs)
	} else {
		err := splitRange4(lowerHalf, prefix, lo, broadcast4(lowerHalf, prefix), cidrs)
		if err != nil {
			return err
		}
		return splitRange4(upperHalf, prefix, upperHalf, hi, cidrs)
	}
}

// IPv4 CIDR block.

type cidrBlock4 struct {
	first uint32
	last  uint32
}

type cidrBlock4s []*cidrBlock4

// newBlock4 returns a new IPv4 CIDR block.
func newBlock4(ip net.IP, mask net.IPMask) *cidrBlock4 {
	var block cidrBlock4

	block.first = ipv4ToUInt32(ip)
	prefix, _ := mask.Size()
	block.last = broadcast4(block.first, uint(prefix))

	return &block
}

// Sort interface.

func (c cidrBlock4s) Len() int {
	return len(c)
}

func (c cidrBlock4s) Less(i, j int) bool {
	lhs := c[i]
	rhs := c[j]

	// By last IP in the range.
	if lhs.last < rhs.last {
		return true
	} else if lhs.last > rhs.last {
		return false
	}

	// Then by first IP in the range.
	if lhs.first < rhs.first {
		return true
	} else if lhs.first > rhs.first {
		return false
	}

	return false
}

func (c cidrBlock4s) Swap(i, j int) {
	c[i], c[j] = c[j], c[i]
}

// merge4 accepts a list of IPv4 networks and merges them into the smallest possible list of IPNets.
// It merges adjacent subnets where possible, those contained within others and removes any duplicates.
func merge4(blocks cidrBlock4s) ([]*net.IPNet, error) {
	sort.Sort(blocks)

	// Coalesce overlapping blocks.
	for i := len(blocks) - 1; i > 0; i-- {
		if blocks[i].first <= blocks[i-1].last+1 {
			blocks[i-1].last = blocks[i].last
			if blocks[i].first < blocks[i-1].first {
				blocks[i-1].first = blocks[i].first
			}
			blocks[i] = nil
		}
	}

	var merged []*net.IPNet
	for _, block := range blocks {
		if block == nil {
			continue
		}

		if err := splitRange4(0, 0, block.first, block.last, &merged); err != nil {
			return nil, err
		}
	}

	return merged, nil
}
