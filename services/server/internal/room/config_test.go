package room

import (
	"testing"
)

func TestConfigValidation(t *testing.T) {
	valid := Config{
		Name: "周五夜场", MaxPlayers: 8, BlindPreset: "5/10", ActionSeconds: 30,
		VoiceEnabled: true, ChipDenominations: []int64{5, 10, 20, 50, 100, 200, 1000},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}

	tests := []struct {
		name  string
		chips []int64
	}{
		{"missing base", []int64{5, 10, 20, 50, 200}},
		{"custom chip", []int64{5, 10, 20, 50, 100, 250}},
		{"duplicate chip", []int64{5, 10, 20, 50, 100, 100}},
		{"negative chip", []int64{5, 10, 20, 50, 100, -200}},
		{"zero chip", []int64{5, 10, 20, 50, 100, 0}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := valid
			config.ChipDenominations = test.chips
			if err := config.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}
