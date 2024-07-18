package types

import "testing"

func TestEndpointMappings(t *testing.T) {
	var tcpMappings TCPMappings
	if err := tcpMappings.Set("1234"); err != nil {
		t.Fatal(err)
	}
	if err := tcpMappings.Set("1234:192.168.1.1"); err != nil {
		t.Fatal(err)
	}
	if err := tcpMappings.Set("1234:192.168.1.1:4321"); err != nil {
		t.Fatal(err)
	}
	if err := tcpMappings.Set("192.168.1.2:1234:192.168.1.1:4321"); err != nil {
		t.Fatal(err)
	}
	if err := tcpMappings.Set("1234:[2000::1]:4321"); err != nil {
		t.Fatal(err)
	}
	if err := tcpMappings.Set("[2001:1]:1234:[2000::1]:4321"); err != nil {
		t.Fatal(err)
	}
	if err := tcpMappings.Set("a"); err == nil {
		t.Fatal("'a' should be an invalid exposed port")
	}
	if err := tcpMappings.Set("1234:localhost"); err == nil {
		t.Fatal("mapped address must be an IP literal")
	}
	if err := tcpMappings.Set("127.0.0.1:1234:localhost"); err == nil {
		t.Fatal("mapped address must be an IP literal")
	}
	if err := tcpMappings.Set("[2000:1]:1234:localhost"); err == nil {
		t.Fatal("mapped address must be an IP literal")
	}
	if err := tcpMappings.Set("localhost:1234:127.0.0.1"); err == nil {
		t.Fatal("listen address must be an IP literal")
	}
	if err := tcpMappings.Set("localhost:1234:127.0.0.1"); err == nil {
		t.Fatal("listen address must be an IP literal")
	}
	if err := tcpMappings.Set("localhost:1234:[2000:1]"); err == nil {
		t.Fatal("listen address must be an IP literal")
	}
	if err := tcpMappings.Set("localhost:1234:[2000:1]"); err == nil {
		t.Fatal("listen address must be an IP literal")
	}
	if err := tcpMappings.Set("1234:localhost:a"); err == nil {
		t.Fatal("'a' should be an invalid mapped port")
	}
	if err := tcpMappings.Set("127.0.0.1:1234:127.0.0.1:a"); err == nil {
		t.Fatal("'a' should be an invalid mapped port")
	}
	if err := tcpMappings.Set("[2000::1]:1234:[2000::1]:a"); err == nil {
		t.Fatal("'a' should be an invalid mapped port")
	}
	var udpMappings UDPMappings
	if err := udpMappings.Set("1234"); err != nil {
		t.Fatal(err)
	}
	if err := udpMappings.Set("1234:192.168.1.1"); err != nil {
		t.Fatal(err)
	}
	if err := udpMappings.Set("1234:192.168.1.1:4321"); err != nil {
		t.Fatal(err)
	}
	if err := udpMappings.Set("192.168.1.2:1234:192.168.1.1:4321"); err != nil {
		t.Fatal(err)
	}
	if err := udpMappings.Set("1234:[2000::1]:4321"); err != nil {
		t.Fatal(err)
	}
	if err := udpMappings.Set("[2001:1]:1234:[2000::1]:4321"); err != nil {
		t.Fatal(err)
	}
	if err := udpMappings.Set("a"); err == nil {
		t.Fatal("'a' should be an invalid exposed port")
	}
	if err := udpMappings.Set("1234:localhost"); err == nil {
		t.Fatal("mapped address must be an IP literal")
	}
	if err := udpMappings.Set("127.0.0.1:1234:localhost"); err == nil {
		t.Fatal("mapped address must be an IP literal")
	}
	if err := udpMappings.Set("[2000:1]:1234:localhost"); err == nil {
		t.Fatal("mapped address must be an IP literal")
	}
	if err := udpMappings.Set("localhost:1234:127.0.0.1"); err == nil {
		t.Fatal("listen address must be an IP literal")
	}
	if err := udpMappings.Set("localhost:1234:127.0.0.1"); err == nil {
		t.Fatal("listen address must be an IP literal")
	}
	if err := udpMappings.Set("localhost:1234:[2000:1]"); err == nil {
		t.Fatal("listen address must be an IP literal")
	}
	if err := udpMappings.Set("localhost:1234:[2000:1]"); err == nil {
		t.Fatal("listen address must be an IP literal")
	}
	if err := udpMappings.Set("1234:localhost:a"); err == nil {
		t.Fatal("'a' should be an invalid mapped port")
	}
	if err := udpMappings.Set("127.0.0.1:1234:127.0.0.1:a"); err == nil {
		t.Fatal("'a' should be an invalid mapped port")
	}
	if err := udpMappings.Set("[2000::1]:1234:[2000::1]:a"); err == nil {
		t.Fatal("'a' should be an invalid mapped port")
	}
}
