// Package world touches the network.
package world

import "net"

// IP parses an address.
func IP() net.IP { return net.ParseIP("::1") }
