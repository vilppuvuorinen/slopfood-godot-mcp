package godot

import "testing"

func TestUIDCapable(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{version: "4.4.stable.official", want: true},
		{version: "4.4", want: true},
		{version: "4.4.1.stable", want: true},
		{version: "4.5.dev6", want: true},
		{version: "4.7.2.stable.official.ed1daf0bf", want: true},
		{version: " 4.7.2.stable ", want: true},
		{version: "5.0.stable", want: true},
		{version: "4.3.9.stable", want: false},
		{version: "4.0.stable", want: false},
		{version: "3.9.stable", want: false},
		{version: "", want: false},
		{version: "garbage", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			if got := UIDCapable(tt.version); got != tt.want {
				t.Errorf("UIDCapable(%q) = %v, want %v", tt.version, got, tt.want)
			}
		})
	}
}
