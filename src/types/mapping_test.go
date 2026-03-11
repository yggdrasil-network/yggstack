package types

import "testing"

// // // // // // // // // //

func TestParseMappingString(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantFirstA  string
		wantFirstP  int
		wantSecondA string
		wantSecondP int
		wantErr     bool
	}{
		// 1 token
		{
			name:        "single port",
			input:       "1234",
			wantFirstP:  1234,
			wantSecondP: 1234,
		},
		{
			name:    "single non-numeric",
			input:   "abc",
			wantErr: true,
		},
		{
			name:    "single zero port",
			input:   "0",
			wantErr: true,
		},

		// 2 tokens
		{
			name:        "two ports",
			input:       "1234:5678",
			wantFirstP:  1234,
			wantSecondP: 5678,
		},
		{
			name:    "two tokens non-numeric second",
			input:   "1234:abc",
			wantErr: true,
		},

		// 3 tokens
		{
			name:        "port:address:port",
			input:       "1234:192.168.1.1:5678",
			wantFirstP:  1234,
			wantSecondA: "192.168.1.1",
			wantSecondP: 5678,
		},
		{
			name:        "port:hostname:port parses without error",
			input:       "1234:localhost:5678",
			wantFirstP:  1234,
			wantSecondA: "localhost",
			wantSecondP: 5678,
		},

		// 4 tokens
		{
			name:        "addr:port:addr:port",
			input:       "192.168.1.2:1234:192.168.1.1:4321",
			wantFirstA:  "192.168.1.2",
			wantFirstP:  1234,
			wantSecondA: "192.168.1.1",
			wantSecondP: 4321,
		},
		{
			name:    "addr:port:addr:non-numeric",
			input:   "127.0.0.1:1234:127.0.0.1:abc",
			wantErr: true,
		},

		// >4 tokens (IPv6)
		{
			name:        "port:ipv6:port",
			input:       "1234:[2000::1]:4321",
			wantFirstP:  1234,
			wantSecondA: "2000::1",
			wantSecondP: 4321,
		},
		{
			name:        "ipv6:port:ipv6:port",
			input:       "[2001::1]:1234:[2000::1]:4321",
			wantFirstA:  "2001::1",
			wantFirstP:  1234,
			wantSecondA: "2000::1",
			wantSecondP: 4321,
		},
		{
			name:    "ipv6:port:ipv6:non-numeric",
			input:   "[2000::1]:1234:[2000::1]:abc",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			firstA, firstP, secondA, secondP, err := parseMappingString(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for input %q, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for input %q: %v", tt.input, err)
			}
			if firstA != tt.wantFirstA {
				t.Errorf("first_address: got %q, want %q", firstA, tt.wantFirstA)
			}
			if firstP != tt.wantFirstP {
				t.Errorf("first_port: got %d, want %d", firstP, tt.wantFirstP)
			}
			if secondA != tt.wantSecondA {
				t.Errorf("second_address: got %q, want %q", secondA, tt.wantSecondA)
			}
			if secondP != tt.wantSecondP {
				t.Errorf("second_port: got %d, want %d", secondP, tt.wantSecondP)
			}
		})
	}
}

func TestTCPLocalMappingsSet(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		// second_address must be IPv6
		{
			name:  "valid ipv6 second address",
			input: "1234:[2000::1]:4321",
		},
		{
			name:    "ipv4 second address rejected",
			input:   "1234:192.168.1.1:4321",
			wantErr: true,
		},
		{
			name:    "single port no ipv6",
			input:   "1234",
			wantErr: true,
		},
		{
			name:  "full spec with ipv6",
			input: "127.0.0.1:1234:[2000::1]:4321",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m TCPLocalMappings
			err := m.Set(tt.input)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for input %q", tt.input)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error for input %q: %v", tt.input, err)
			}
		})
	}
}

func TestTCPRemoteMappingsSet(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		// first_address must be empty
		{
			name:  "single port",
			input: "1234",
		},
		{
			name:  "two ports",
			input: "2022:22",
		},
		{
			name:  "port:addr:port",
			input: "22:192.168.1.1:2022",
		},
		{
			name:    "first address not empty rejected",
			input:   "192.168.1.2:1234:192.168.1.1:4321",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m TCPRemoteMappings
			err := m.Set(tt.input)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for input %q", tt.input)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error for input %q: %v", tt.input, err)
			}
		})
	}
}

func TestUDPLocalMappingsSet(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:  "valid ipv6 second address",
			input: "1234:[2000::1]:4321",
		},
		{
			name:    "ipv4 second address rejected",
			input:   "1234:192.168.1.1:4321",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m UDPLocalMappings
			err := m.Set(tt.input)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for input %q", tt.input)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error for input %q: %v", tt.input, err)
			}
		})
	}
}

func TestUDPRemoteMappingsSet(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:  "single port",
			input: "1234",
		},
		{
			name:  "port:addr:port",
			input: "22:192.168.1.1:2022",
		},
		{
			name:    "first address not empty rejected",
			input:   "192.168.1.2:1234:192.168.1.1:4321",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m UDPRemoteMappings
			err := m.Set(tt.input)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error for input %q", tt.input)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error for input %q: %v", tt.input, err)
			}
		})
	}
}
