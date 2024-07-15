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
	if err := tcpMappings.Set("1234:[2000::1]:4321"); err != nil {
		t.Fatal(err)
	}
	if err := tcpMappings.Set("a"); err == nil {
		t.Fatal("'a' should be an invalid exposed port")
	}
	if err := tcpMappings.Set("1234:localhost"); err == nil {
		t.Fatal("mapped address must be an IP literal")
	}
	if err := tcpMappings.Set("1234:localhost:a"); err == nil {
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
	if err := udpMappings.Set("1234:[2000::1]:4321"); err != nil {
		t.Fatal(err)
	}
	if err := udpMappings.Set("a"); err == nil {
		t.Fatal("'a' should be an invalid exposed port")
	}
	if err := udpMappings.Set("1234:localhost"); err == nil {
		t.Fatal("mapped address must be an IP literal")
	}
	if err := udpMappings.Set("1234:localhost:a"); err == nil {
		t.Fatal("'a' should be an invalid mapped port")
	}
}
