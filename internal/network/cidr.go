// Package network provides helpers for IP/CIDR manipulation.
//
// The main use-case is computing the kubernetes.default ClusterIP
// from the cluster's service CIDR. In any Kubernetes cluster,
// kubernetes.default always gets the first assignable IP from
// the service CIDR (e.g. 10.96.0.0/12 → 10.96.0.1).
package network

import (
	"fmt"
	"net"
)

// FirstUsableIP returns the first assignable host IP in a CIDR block.
//
// In Kubernetes, the service CIDR's first usable IP is always assigned
// to the "kubernetes" service in the "default" namespace.
//
// Examples:
//
//	FirstUsableIP("10.96.0.0/12")   → "10.96.0.1"
//	FirstUsableIP("10.0.0.0/24")    → "10.0.0.1"
//	FirstUsableIP("172.16.0.0/16")  → "172.16.0.1"
func FirstUsableIP(cidr string) (string, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return "", fmt.Errorf("parsing CIDR %q: %w", cidr, err)
	}

	// ipNet.IP is the network address (e.g. 10.96.0.0).
	// We need to convert to a 4-byte representation for IPv4.
	ip := ipNet.IP.To4()
	if ip == nil {
		return "", fmt.Errorf("not an IPv4 CIDR: %s", cidr)
	}

	// The first usable IP is the network address + 1.
	// We increment the last octet. This is safe because
	// any valid service CIDR has at least a /30 mask,
	// so incrementing by 1 won't overflow the last byte
	// in practice. But let's handle it properly anyway.
	ip = incrementIP(ip)

	return ip.String(), nil
}

// incrementIP adds 1 to an IPv4 address, handling carry across octets.
func incrementIP(ip net.IP) net.IP {
	result := make(net.IP, len(ip))
	copy(result, ip)

	// Start from the last octet and carry over.
	for i := len(result) - 1; i >= 0; i-- {
		result[i]++
		if result[i] != 0 {
			// No overflow, stop.
			break
		}
		// Overflow (255 → 0), carry to next octet.
	}

	return result
}
