package main

import "testing"

func TestValidateChainsFlags(t *testing.T) {
	cases := []struct {
		name     string
		chains   bool
		prefixes []string
		wantErr  bool
	}{
		{"chains_without_prefix_rejected", true, nil, true},
		{"chains_with_prefix_ok", true, []string{"example.com/gen"}, false},
		{"syntax_mode_without_prefix_ok", false, nil, false},
		{"syntax_mode_with_prefix_ok", false, []string{"example.com/gen"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateChainsFlags(tc.chains, tc.prefixes)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateChainsFlags(%v, %v) error = %v, wantErr %v", tc.chains, tc.prefixes, err, tc.wantErr)
			}
		})
	}
}
