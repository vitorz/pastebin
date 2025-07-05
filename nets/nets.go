package nets

import (
	"fmt"
	"log"
	"net"
	"strings"
)

func CalculateAllIpv4() []net.IP {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		log.Fatalln("Error getting network interfaces:", err)
	}
	result := make([]net.IP, 0, len(addrs))
	k := 0
	for _, addr := range addrs {
		// Check the address type and skip loopback
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			// Only want IPv4
			if ip4 := ipNet.IP.To4(); ip4 != nil {
				result[k] = ip4
				k += 1
			}
		}
	}
	return result
}

func isVirtualInterface(name string) bool {
	virtualPrefixes := []string{
		"docker", "br-", "vmnet", "vboxnet", "lo", "tun", "tap", "zt", "wg", "vir",
	}
	for _, prefix := range virtualPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

type Netinf struct {
	Name string
	IP   net.IP
	Net  string
	Cidr int
}

func (ni Netinf) String() string {
	return fmt.Sprintf("Interface: %10s | IP: %-15s | Network: %s/%d", ni.Name, ni.IP.String(), ni.Net, ni.Cidr)
}

func GetRealInterfaces() ([]Netinf, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		fmt.Println("Error fetching interfaces:", err)
		return nil, err
	}

	result := make([]Netinf, 0, len(ifaces))

	for _, iface := range ifaces {
		// Skip interfaces that are down, loopback, or virtual
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || isVirtualInterface(iface.Name) {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			if ipNet, ok := addr.(*net.IPNet); ok {
				ip := ipNet.IP

				if ip.To4() != nil {
					cidr, _ := ipNet.Mask.Size()
					result = append(result, Netinf{iface.Name, ip, ip.Mask(ipNet.Mask).String(), cidr})
				}
			}
		}
	}
	return result, nil
}
