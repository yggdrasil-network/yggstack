package types

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

func parseMappingString(value string) (firstAddress string, firstPort int, secondAddress string, secondPort int, err error) {
	var firstPortStr string = ""
	var secondPortStr string = ""

	tokens := strings.Split(value, ":")
	tokensLen := len(tokens)

	// If token count is 1, then it is first and second port the same

	if tokensLen == 1 {
		firstPort, err = strconv.Atoi(tokens[0])
		if err != nil {
			return "", 0, "", 0, fmt.Errorf("Malformed mapping spec '%s'", value)
		}
		secondPort = firstPort
	}

	// If token count is 2, then it is <first-port>:<second-port>

	if tokensLen == 2 {
		firstPort, err = strconv.Atoi(tokens[0])
		if err != nil {
			return "", 0, "", 0, fmt.Errorf("Malformed mapping spec '%s'", value)
		}
		secondPort, err = strconv.Atoi(tokens[1])
		if err != nil {
			return "", 0, "", 0, fmt.Errorf("Malformed mapping spec '%s'", value)
		}
	}

	// If token count is 3, parse it as
	// <first-port>:<second-address>:<second-port>

	if tokensLen == 3 {
		firstPort, err = strconv.Atoi(tokens[0])
		if err != nil {
			return "", 0, "", 0, fmt.Errorf("Malformed mapping spec '%s'", value)
		}
		secondAddress, secondPortStr, err = net.SplitHostPort(
			tokens[1] + ":" + tokens[2])
		if err != nil {
			return "", 0, "", 0, fmt.Errorf("Malformed mapping spec '%s'", value)
		}
		secondPort, err = strconv.Atoi(secondPortStr)
		if err != nil {
			return "", 0, "", 0, fmt.Errorf("Malformed mapping spec '%s'", value)
		}
	}

	// If token count is 4, parse it as
	// <first-address>:<first-port>:<second-address>:<second-port>

	if tokensLen == 4 {
		firstAddress, firstPortStr, err = net.SplitHostPort(
			tokens[0] + ":" + tokens[1])
		if err != nil {
			return "", 0, "", 0, fmt.Errorf("Malformed mapping spec '%s'", value)
		}
		secondAddress, secondPortStr, err = net.SplitHostPort(
			tokens[2] + ":" + tokens[3])
		if err != nil {
			return "", 0, "", 0, fmt.Errorf("Malformed mapping spec '%s'", value)
		}
		firstPort, err = strconv.Atoi(firstPortStr)
		if err != nil {
			return "", 0, "", 0, fmt.Errorf("Malformed mapping spec '%s'", value)
		}
		secondPort, err = strconv.Atoi(secondPortStr)
		if err != nil {
			return "", 0, "", 0, fmt.Errorf("Malformed mapping spec '%s'", value)
		}
	}

	if tokensLen > 4 {
		// Last token needs to be the secondPort

		secondPort, err = strconv.Atoi(tokens[tokensLen-1])
		if err != nil {
			return "", 0, "", 0, fmt.Errorf("Malformed mapping spec '%s'", value)
		}

		// Cut seen tokens

		tokens = tokens[:tokensLen-1]
		tokensLen = len(tokens)

		if strings.HasSuffix(tokens[tokensLen-1], "]") {
			// Reverse-walk over tokens to find the end of
			// numeric ipv6 address

			for i := tokensLen - 1; i >= 0; i-- {
				if strings.HasPrefix(tokens[i], "[") {
					// Store second address
					secondAddress = strings.Join(tokens[i:], ":")
					secondAddress, _ = strings.CutPrefix(secondAddress, "[")
					secondAddress, _ = strings.CutSuffix(secondAddress, "]")
					// Cut seen tokens
					tokens = tokens[:i]
					// break from loop
					break
				}
			}
		} else {
			// next is second address in non-numerical-ipv6 form
			secondAddress = tokens[tokensLen-1]
			tokens = tokens[:tokensLen-1]
		}

		tokensLen = len(tokens)

		if tokensLen < 1 {
			return "", 0, "", 0, fmt.Errorf("Malformed mapping spec '%s'", value)
		}

		// Last token needs to be the firstPort

		firstPort, err = strconv.Atoi(tokens[tokensLen-1])
		if err != nil {
			return "", 0, "", 0, fmt.Errorf("Malformed mapping spec '%s'", value)
		}

		// Cut seen tokens

		tokens = tokens[:tokensLen-1]
		tokensLen = len(tokens)

		if tokensLen > 0 {
			if strings.HasSuffix(tokens[tokensLen-1], "]") {
				// Reverse-walk over tokens to find the end of
				// numeric ipv6 address

				for i := tokensLen - 1; i >= 0; i-- {
					if strings.HasPrefix(tokens[i], "[") {
						// Store first address
						firstAddress = strings.Join(tokens[i:], ":")
						firstAddress, _ = strings.CutPrefix(firstAddress, "[")
						firstAddress, _ = strings.CutSuffix(firstAddress, "]")
						// break from loop
						break
					}
				}
			} else {
				// next is first address in non-numerical-ipv6 form
				firstAddress = tokens[tokensLen-1]
			}
		}
	}

	if firstPort < 1 || firstPort > 65535 || secondPort < 1 || secondPort > 65535 {
		return "", 0, "", 0, fmt.Errorf("ports must be in range 1-65535")
	}

	return firstAddress, firstPort, secondAddress, secondPort, nil
}

// Validate addresses and ports after parsing the mapping string
func buildMapping(value string, isLocal bool) (listenIP net.IP, listenPort int, mappedIP net.IP, mappedPort int, err error) {
	firstAddr, firstPort, secondAddr, secondPort, err := parseMappingString(value)
	if err != nil {
		return nil, 0, nil, 0, err
	}

	if isLocal {
		// Local mappings must have an explicit Yggdrasil destination.
		// Reject empty, IPv4, and IPv4-mapped-IPv6 addresses.
		if secondAddr == "" {
			return nil, 0, nil, 0, fmt.Errorf("local mapping requires a Yggdrasil IPv6 destination address")
		}
		ip := net.ParseIP(secondAddr)
		if ip == nil || ip.To4() != nil {
			return nil, 0, nil, 0, fmt.Errorf("Yggdrasil mapped address must be a valid IPv6 address, got %q", secondAddr)
		}
	} else {
		if firstAddr != "" {
			return nil, 0, nil, 0, fmt.Errorf("Yggdrasil listening must be empty")
		}
	}

	//

	mappedIP = net.IPv6loopback

	if firstAddr != "" {
		listenIP = net.ParseIP(firstAddr)
		if listenIP == nil {
			return nil, 0, nil, 0, fmt.Errorf("invalid listen address %q", firstAddr)
		}
	}

	if secondAddr != "" {
		// TODO: Filter Yggdrasil IPs here (for Local mappings)
		mappedIP = net.ParseIP(secondAddr)
		if mappedIP == nil {
			return nil, 0, nil, 0, fmt.Errorf("invalid mapped address %q", secondAddr)
		}
	}

	return listenIP, firstPort, mappedIP, secondPort, nil
}

// // // // //

type TCPMapping struct {
	Listen *net.TCPAddr
	Mapped *net.TCPAddr
}

type TCPLocalMappings []TCPMapping

func (m *TCPLocalMappings) String() string { return "" }

func (m *TCPLocalMappings) Set(value string) error {
	listenIP, listenPort, mappedIP, mappedPort, err := buildMapping(value, true)
	if err != nil {
		return err
	}
	*m = append(*m, TCPMapping{
		Listen: &net.TCPAddr{IP: listenIP, Port: listenPort},
		Mapped: &net.TCPAddr{IP: mappedIP, Port: mappedPort},
	})
	return nil
}

// //

type TCPRemoteMappings []TCPMapping

func (m *TCPRemoteMappings) String() string { return "" }

func (m *TCPRemoteMappings) Set(value string) error {
	listenIP, listenPort, mappedIP, mappedPort, err := buildMapping(value, false)
	if err != nil {
		return err
	}
	*m = append(*m, TCPMapping{
		Listen: &net.TCPAddr{IP: listenIP, Port: listenPort},
		Mapped: &net.TCPAddr{IP: mappedIP, Port: mappedPort},
	})
	return nil
}

// // // // //

type UDPMapping struct {
	Listen *net.UDPAddr
	Mapped *net.UDPAddr
}

type UDPLocalMappings []UDPMapping

func (m *UDPLocalMappings) String() string { return "" }

func (m *UDPLocalMappings) Set(value string) error {
	listenIP, listenPort, mappedIP, mappedPort, err := buildMapping(value, true)
	if err != nil {
		return err
	}
	*m = append(*m, UDPMapping{
		Listen: &net.UDPAddr{IP: listenIP, Port: listenPort},
		Mapped: &net.UDPAddr{IP: mappedIP, Port: mappedPort},
	})
	return nil
}

// //

type UDPRemoteMappings []UDPMapping

func (m *UDPRemoteMappings) String() string { return "" }

func (m *UDPRemoteMappings) Set(value string) error {
	listenIP, listenPort, mappedIP, mappedPort, err := buildMapping(value, false)
	if err != nil {
		return err
	}
	*m = append(*m, UDPMapping{
		Listen: &net.UDPAddr{IP: listenIP, Port: listenPort},
		Mapped: &net.UDPAddr{IP: mappedIP, Port: mappedPort},
	})
	return nil
}
