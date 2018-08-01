package cidrman

import (
	"fmt"
	"math/big"
	"net"
)

// ipv6ToUInt128 converts an IPv6 address to an unsigned 128-bit integer.
func ipv6ToUInt128(ip net.IP) *big.Int {
	return big.NewInt(0).SetBytes(ip)
}

// uint128ToIPV6 converts an unsigned 128-bit integer to an IPv6 address.
func uint128ToIPV6(addr *big.Int) net.IP {
	return net.IP(addr.Bytes()).To16()
}

// copyUInt128 copies an unsigned 128-bit integer.
func copyUInt128(x *big.Int) *big.Int {
	return big.NewInt(0).Set(x)
}

// broadcast6 returns the broadcast address for the given address and prefix.
func broadcast6(addr *big.Int, prefix uint) *big.Int {
	z := copyUInt128(addr)

	if prefix == 0 {
		z, _ = z.SetString("340282366920938463463374607431768211455", 10)
		return z
	}

	for i := int(prefix); i < 8*net.IPv6len; i++ {
		z = z.SetBit(z, i, 1)
	}
	return z
}

// network6 returns the network address for the given address and prefix.
func network6(addr *big.Int, prefix uint) *big.Int {
	z := copyUInt128(addr)

	if prefix == 0 {
		return z
	}

	for i := int(prefix); i < 8*net.IPv6len; i++ {
		z = z.SetBit(z, i, 0)
	}
	return z
}

// splitRange6 recursively computes the CIDR blocks to cover the range lo to hi.
func splitRange6(addr *big.Int, prefix uint, lo, hi *big.Int, cidrs *[]*net.IPNet) error {
	if prefix > 128 {
		return fmt.Errorf("Invalid mask size: %d", prefix)
	}

	bc := broadcast6(addr, prefix)
	fmt.Printf("%v/%v/%v/%v/%v\n", addr, prefix, lo, hi, bc)
	if (lo.Cmp(addr) < 0) || (hi.Cmp(bc) > 0) {
		return fmt.Errorf("%v, %v out of range for network %v/%d, broadcast %v", lo, hi, addr, prefix, bc)
	}

	if (lo.Cmp(addr) == 0) && (hi.Cmp(bc) == 0) {
		cidr := net.IPNet{IP: uint128ToIPV6(addr), Mask: net.CIDRMask(int(prefix), 8*net.IPv6len)}
		*cidrs = append(*cidrs, &cidr)
		return nil
	}

	prefix++
	lowerHalf := copyUInt128(addr)
	upperHalf := copyUInt128(addr)
	upperHalf = upperHalf.SetBit(upperHalf, int(prefix), 1)
	if hi.Cmp(upperHalf) < 0 {
		return splitRange6(lowerHalf, prefix, lo, hi, cidrs)
	} else if lo.Cmp(upperHalf) >= 0 {
		return splitRange6(upperHalf, prefix, lo, hi, cidrs)
	} else {
		err := splitRange6(lowerHalf, prefix, lo, broadcast6(lowerHalf, prefix), cidrs)
		if err != nil {
			return err
		}
		return splitRange6(upperHalf, prefix, upperHalf, hi, cidrs)
	}
}
