package netstack

import (
	"net"
	"testing"

	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/network/ipv6"
)

// // // // // // // // // //

func TestConvertToFullAddr_IPv6(t *testing.T) {
	ip := net.ParseIP("2001:db8::1")
	fa, proto, err := convertToFullAddr(ip, 8080)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if proto != ipv6.ProtocolNumber {
		t.Errorf("proto = %d, want ipv6.ProtocolNumber", proto)
	}
	if fa.NIC != 1 {
		t.Errorf("NIC = %d, want 1", fa.NIC)
	}
	if fa.Port != 8080 {
		t.Errorf("Port = %d, want 8080", fa.Port)
	}
	want := tcpip.AddrFromSlice(ip.To16())
	if fa.Addr != want {
		t.Errorf("Addr = %v, want %v", fa.Addr, want)
	}
}

func TestConvertToFullAddr_IPv4(t *testing.T) {
	ip := net.ParseIP("192.168.1.1")
	fa, proto, err := convertToFullAddr(ip, 443)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if proto != ipv6.ProtocolNumber {
		t.Errorf("proto = %d, want ipv6.ProtocolNumber", proto)
	}
	if fa.Port != 443 {
		t.Errorf("Port = %d, want 443", fa.Port)
	}
	// IPv4 is converted to IPv4-mapped IPv6 via To16()
	want := tcpip.AddrFromSlice(ip.To16())
	if fa.Addr != want {
		t.Errorf("Addr = %v, want %v", fa.Addr, want)
	}
}

func TestConvertToFullAddr_NilIP(t *testing.T) {
	fa, _, err := convertToFullAddr(nil, 80)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// nil IP → empty address
	if fa.Addr != (tcpip.Address{}) {
		t.Errorf("expected empty address for nil IP, got %v", fa.Addr)
	}
}

// //

func TestConvertToFullAddrFromString_Valid(t *testing.T) {
	tests := []struct {
		endpoint  string
		wantPort  uint16
		wantEmpty bool
	}{
		{"[2001:db8::1]:9000", 9000, false},
		{"192.168.1.1:443", 443, false},
		{"[::1]:0", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.endpoint, func(t *testing.T) {
			fa, proto, err := convertToFullAddrFromString(tt.endpoint)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if proto != ipv6.ProtocolNumber {
				t.Errorf("proto = %d, want ipv6.ProtocolNumber", proto)
			}
			if fa.Port != tt.wantPort {
				t.Errorf("Port = %d, want %d", fa.Port, tt.wantPort)
			}
		})
	}
}

func TestConvertToFullAddrFromString_NoPort(t *testing.T) {
	// Port 80 should be used when no port is given.
	fa, _, err := convertToFullAddrFromString("[2001:db8::1]:")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fa.Port != 80 {
		t.Errorf("Port = %d, want 80", fa.Port)
	}
}

func TestConvertToFullAddrFromString_InvalidFormat(t *testing.T) {
	_, _, err := convertToFullAddrFromString("not-an-endpoint")
	if err == nil {
		t.Fatal("expected error for invalid endpoint")
	}
}

func TestConvertToFullAddrFromString_InvalidPort(t *testing.T) {
	_, _, err := convertToFullAddrFromString("[::1]:notaport")
	if err == nil {
		t.Fatal("expected error for non-numeric port")
	}
}

// //

func BenchmarkConvertToFullAddrFromString(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _, _ = convertToFullAddrFromString("[2001:db8::1]:8080")
	}
}
