package types

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

func parseMappingString(value string) (first_address string, first_port int, second_address string, second_port int, err error) {
	var first_port_string string = ""
	var second_port_string string = ""

	tokens := strings.Split(value, ":")
	tokens_len := len(tokens)

	// If token count is 1, then it is first and second port the same

	if tokens_len == 1 {
		first_port, err = strconv.Atoi(tokens[0])
		if err != nil {
			return "", 0, "", 0, fmt.Errorf("Malformed mapping spec '%s'", value)
		}
		second_port = first_port
	}

	// If token count is 2, then it is <first-port>:<second-port>

	if tokens_len == 2 {
		first_port, err = strconv.Atoi(tokens[0])
		if err != nil {
			return "", 0, "", 0, fmt.Errorf("Malformed mapping spec '%s'", value)
		}
		second_port, err = strconv.Atoi(tokens[1])
		if err != nil {
			return "", 0, "", 0, fmt.Errorf("Malformed mapping spec '%s'", value)
		}
	}

	// If token count is 3, parse it as
	// <first-port>:<second-address>:<second-port>

	if tokens_len == 3 {
		first_port, err = strconv.Atoi(tokens[0])
		if err != nil {
			return "", 0, "", 0, fmt.Errorf("Malformed mapping spec '%s'", value)
		}
		second_address, second_port_string, err = net.SplitHostPort(
			tokens[1] + ":" + tokens[2])
		if err != nil {
			return "", 0, "", 0, fmt.Errorf("Malformed mapping spec '%s'", value)
		}
		second_port, err = strconv.Atoi(second_port_string)
		if err != nil {
			return "", 0, "", 0, fmt.Errorf("Malformed mapping spec '%s'", value)
		}
	}

	// If token count is 4, parse it as
	// <first-address>:<first-port>:<second-address>:<second-port>

	if tokens_len == 4 {
		first_address, first_port_string, err = net.SplitHostPort(
			tokens[0] + ":" + tokens[1])
		if err != nil {
			return "", 0, "", 0, fmt.Errorf("Malformed mapping spec '%s'", value)
		}
		second_address, second_port_string, err = net.SplitHostPort(
			tokens[0] + ":" + tokens[1])
		if err != nil {
			return "", 0, "", 0, fmt.Errorf("Malformed mapping spec '%s'", value)
		}
		first_port, err = strconv.Atoi(first_port_string)
		if err != nil {
			return "", 0, "", 0, fmt.Errorf("Malformed mapping spec '%s'", value)
		}
		second_port, err = strconv.Atoi(second_port_string)
		if err != nil {
			return "", 0, "", 0, fmt.Errorf("Malformed mapping spec '%s'", value)
		}
	}

	if tokens_len > 4 {
		// Last token needs to be the second_port

		second_port, err = strconv.Atoi(tokens[tokens_len-1])
		if err != nil {
			return "", 0, "", 0, fmt.Errorf("Malformed mapping spec '%s'", value)
		}

		// Cut seen tokens

		tokens = tokens[:tokens_len-1]
		tokens_len = len(tokens)

		if strings.HasSuffix(tokens[tokens_len-1], "]") {
			// Reverse-walk over tokens to find the end of
			// numeric ipv6 address

			for i := tokens_len - 1; i >= 0; i-- {
				if strings.HasPrefix(tokens[i], "[") {
					// Store second address
					second_address = strings.Join(tokens[i:], ":")
					second_address, _ = strings.CutPrefix(second_address, "[")
					second_address, _ = strings.CutSuffix(second_address, "]")
					// Cut seen tokens
					tokens = tokens[:i]
					// break from loop
					break
				}
			}
		} else {
			// next is second address in non-numerical-ipv6 form
			second_address = tokens[tokens_len-1]
			tokens = tokens[:tokens_len-1]
		}

		tokens_len = len(tokens)

		if tokens_len < 1 {
			return "", 0, "", 0, fmt.Errorf("Malformed mapping spec '%s'", value)
		}

		// Last token needs to be the first_port

		first_port, err = strconv.Atoi(tokens[tokens_len-1])
		if err != nil {
			return "", 0, "", 0, fmt.Errorf("Malformed mapping spec '%s'", value)
		}

		// Cut seen tokens

		tokens = tokens[:tokens_len-1]
		tokens_len = len(tokens)

		if tokens_len > 0 {
			if strings.HasSuffix(tokens[tokens_len-1], "]") {
				// Reverse-walk over tokens to find the end of
				// numeric ipv6 address

				for i := tokens_len - 1; i >= 0; i-- {
					if strings.HasPrefix(tokens[i], "[") {
						// Store first address
						first_address = strings.Join(tokens[i:], ":")
						first_address, _ = strings.CutPrefix(first_address, "[")
						first_address, _ = strings.CutSuffix(first_address, "]")
						// break from loop
						break
					}
				}
			} else {
				// next is first address in non-numerical-ipv6 form
				first_address = tokens[tokens_len-1]
			}
		}
	}

	if first_port == 0 || second_port == 0 {
		return "", 0, "", 0, fmt.Errorf("Ports must not be zero")
	}

	return first_address, first_port, second_address, second_port, nil
}

type TCPMapping struct {
	Listen *net.TCPAddr
	Mapped *net.TCPAddr
}

type TCPLocalMappings []TCPMapping

func (m *TCPLocalMappings) String() string {
	return ""
}

func (m *TCPLocalMappings) Set(value string) error {
	first_address, first_port, second_address, second_port, err :=
		parseMappingString(value)

	if err != nil {
		return err
	}

	// First address can be ipv4/ipv6
	// Second address can be only Yggdrasil ipv6

	if !strings.Contains(second_address, ":") {
		return fmt.Errorf("Yggdrasil listening address can be only IPv6")
	}

	// Create mapping

	mapping := TCPMapping{
		Listen: &net.TCPAddr{
			Port: first_port,
		},
		Mapped: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: second_port,
		},
	}

	if first_address != "" {
		listenaddr := net.ParseIP(first_address)
		if listenaddr == nil {
			return fmt.Errorf("invalid listen address %q", first_address)
		}
		mapping.Listen.IP = listenaddr
	}

	if second_address != "" {
		mappedaddr := net.ParseIP(second_address)
		if mappedaddr == nil {
			return fmt.Errorf("invalid mapped address %q", second_address)
		}
		// TODO: Filter Yggdrasil IPs here
		mapping.Mapped.IP = mappedaddr
	}

	*m = append(*m, mapping)
	return nil
}

type TCPRemoteMappings []TCPMapping

func (m *TCPRemoteMappings) String() string {
	return ""
}

func (m *TCPRemoteMappings) Set(value string) error {
	first_address, first_port, second_address, second_port, err :=
		parseMappingString(value)

	if err != nil {
		return err
	}

	// First address must be empty
	// Second address can be ipv4/ipv6

	if first_address != "" {
		return fmt.Errorf("Yggdrasil listening must be empty")
	}

	// Create mapping

	mapping := TCPMapping{
		Listen: &net.TCPAddr{
			Port: first_port,
		},
		Mapped: &net.TCPAddr{
			IP:   net.IPv6loopback,
			Port: second_port,
		},
	}

	if first_address != "" {
		listenaddr := net.ParseIP(first_address)
		if listenaddr == nil {
			return fmt.Errorf("invalid listen address %q", first_address)
		}
		mapping.Listen.IP = listenaddr
	}

	if second_address != "" {
		mappedaddr := net.ParseIP(second_address)
		if mappedaddr == nil {
			return fmt.Errorf("invalid mapped address %q", second_address)
		}
		mapping.Mapped.IP = mappedaddr
	}

	*m = append(*m, mapping)
	return nil
}

type UDPMapping struct {
	Listen *net.UDPAddr
	Mapped *net.UDPAddr
}

type UDPLocalMappings []UDPMapping

func (m *UDPLocalMappings) String() string {
	return ""
}

func (m *UDPLocalMappings) Set(value string) error {
	first_address, first_port, second_address, second_port, err :=
		parseMappingString(value)

	if err != nil {
		return err
	}

	// First address can be ipv4/ipv6
	// Second address can be only Yggdrasil ipv6

	if !strings.Contains(second_address, ":") {
		return fmt.Errorf("Yggdrasil listening address can be only IPv6")
	}

	// Create mapping

	mapping := UDPMapping{
		Listen: &net.UDPAddr{
			Port: first_port,
		},
		Mapped: &net.UDPAddr{
			IP:   net.IPv6loopback,
			Port: second_port,
		},
	}

	if first_address != "" {
		listenaddr := net.ParseIP(first_address)
		if listenaddr == nil {
			return fmt.Errorf("invalid listen address %q", first_address)
		}
		mapping.Listen.IP = listenaddr
	}

	if second_address != "" {
		mappedaddr := net.ParseIP(second_address)
		if mappedaddr == nil {
			return fmt.Errorf("invalid mapped address %q", second_address)
		}
		// TODO: Filter Yggdrasil IPs here
		mapping.Mapped.IP = mappedaddr
	}

	*m = append(*m, mapping)
	return nil
}

type UDPRemoteMappings []UDPMapping

func (m *UDPRemoteMappings) String() string {
	return ""
}

func (m *UDPRemoteMappings) Set(value string) error {
	first_address, first_port, second_address, second_port, err :=
		parseMappingString(value)

	if err != nil {
		return err
	}

	// First address must be empty
	// Second address can be ipv4/ipv6

	if first_address != "" {
		return fmt.Errorf("Yggdrasil listening must be empty")
	}

	// Create mapping

	mapping := UDPMapping{
		Listen: &net.UDPAddr{
			Port: first_port,
		},
		Mapped: &net.UDPAddr{
			IP:   net.IPv6loopback,
			Port: second_port,
		},
	}

	if first_address != "" {
		listenaddr := net.ParseIP(first_address)
		if listenaddr == nil {
			return fmt.Errorf("invalid listen address %q", first_address)
		}
		mapping.Listen.IP = listenaddr
	}

	if second_address != "" {
		mappedaddr := net.ParseIP(second_address)
		if mappedaddr == nil {
			return fmt.Errorf("invalid mapped address %q", second_address)
		}
		mapping.Mapped.IP = mappedaddr
	}

	*m = append(*m, mapping)
	return nil
}
