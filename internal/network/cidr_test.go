package network

import (
	"testing"
)

func TestFirstUsableIP(t *testing.T) {
	tests := []struct {
		name    string
		cidr    string
		want    string
		wantErr bool
	}{
		{
			name: "standard kubernetes service CIDR /12",
			cidr: "10.96.0.0/12",
			want: "10.96.0.1",
		},
		{
			name: "small /24 network",
			cidr: "10.0.0.0/24",
			want: "10.0.0.1",
		},
		{
			name: "class B private",
			cidr: "172.16.0.0/16",
			want: "172.16.0.1",
		},
		{
			name: "class C private",
			cidr: "192.168.1.0/24",
			want: "192.168.1.1",
		},
		{
			name: "tiny /30 network",
			cidr: "10.0.0.0/30",
			want: "10.0.0.1",
		},
		{
			name:    "invalid CIDR",
			cidr:    "not-a-cidr",
			wantErr: true,
		},
		{
			name:    "empty string",
			cidr:    "",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FirstUsableIP(tt.cidr)
			if (err != nil) != tt.wantErr {
				t.Errorf("FirstUsableIP(%q) error = %v, wantErr %v", tt.cidr, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("FirstUsableIP(%q) = %q, want %q", tt.cidr, got, tt.want)
			}
		})
	}
}

func TestIncrementIP(t *testing.T) {
	tests := []struct {
		name string
		ip   []byte
		want []byte
	}{
		{
			name: "simple increment",
			ip:   []byte{10, 0, 0, 0},
			want: []byte{10, 0, 0, 1},
		},
		{
			name: "carry over last byte",
			ip:   []byte{10, 0, 0, 255},
			want: []byte{10, 0, 1, 0},
		},
		{
			name: "carry over two bytes",
			ip:   []byte{10, 0, 255, 255},
			want: []byte{10, 1, 0, 0},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := incrementIP(tt.ip)
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("incrementIP(%v) = %v, want %v", tt.ip, got, tt.want)
					break
				}
			}
		})
	}
}
