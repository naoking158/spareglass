// Package netutil finds addresses other devices on the LAN can reach.
package netutil

import (
	"fmt"
	"net"
)

// LANURLs returns http URLs for each private IPv4 address of this machine.
// Port 80 is left out of the URL.
func LANURLs(port int, path string) []string {
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
		host := ip.String()
		if port != 80 {
			host = fmt.Sprintf("%s:%d", ip, port)
		}
		urls = append(urls, "http://"+host+path)
	}
	return urls
}
