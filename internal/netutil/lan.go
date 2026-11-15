// Package netutil finds addresses other devices on the LAN can reach.
package netutil

import (
	"fmt"
	"net"
)

// LANURLs returns http URLs for each private IPv4 address of this machine.
func LANURLs(port int, fragment string) []string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	var urls []string
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipnet.IP.To4()
		if ip == nil || ip.IsLoopback() || !ip.IsPrivate() {
			continue
		}
		urls = append(urls, fmt.Sprintf("http://%s:%d/%s", ip, port, fragment))
	}
	return urls
}
