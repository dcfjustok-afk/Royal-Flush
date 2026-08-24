package room

import (
	"errors"
	"fmt"
	"sort"
)

const (
	InitialTablePoints int64 = 1000
	MinPlayers               = 2
	MaxPlayers               = 8
)

var (
	baseDenominations  = []int64{5, 10, 20, 50, 100}
	largeDenominations = map[int64]bool{200: true, 500: true, 1000: true}
)

type Config struct {
	Name              string  `json:"name"`
	MaxPlayers        int     `json:"maxPlayers"`
	BlindPreset       string  `json:"blindPreset"`
	ActionSeconds     int     `json:"actionSeconds"`
	VoiceEnabled      bool    `json:"voiceEnabled"`
	ChipDenominations []int64 `json:"chipDenominations"`
}

func (c Config) Validate() error {
	if c.Name == "" || len([]rune(c.Name)) > 32 {
		return errors.New("room name must contain 1 to 32 characters")
	}
	if c.MaxPlayers < MinPlayers || c.MaxPlayers > MaxPlayers {
		return fmt.Errorf("maxPlayers must be between %d and %d", MinPlayers, MaxPlayers)
	}
	if _, _, err := c.Blinds(); err != nil {
		return err
	}
	if c.ActionSeconds != 20 && c.ActionSeconds != 30 && c.ActionSeconds != 45 {
		return errors.New("actionSeconds must be 20, 30, or 45")
	}
	if err := ValidateDenominations(c.ChipDenominations); err != nil {
		return err
	}
	return nil
}

func (c Config) Blinds() (int64, int64, error) {
	switch c.BlindPreset {
	case "2/5":
		return 2, 5, nil
	case "5/10":
		return 5, 10, nil
	case "10/20":
		return 10, 20, nil
	default:
		return 0, 0, errors.New("blindPreset must be 2/5, 5/10, or 10/20")
	}
}

func ValidateDenominations(values []int64) error {
	if len(values) < len(baseDenominations) || len(values) > len(baseDenominations)+len(largeDenominations) {
		return errors.New("chip denominations must contain all five base chips and only supported large chips")
	}
	seen := make(map[int64]bool, len(values))
	for _, value := range values {
		if value <= 0 {
			return errors.New("chip denominations must be positive")
		}
		if seen[value] {
			return fmt.Errorf("duplicate chip denomination %d", value)
		}
		seen[value] = true
	}
	for _, required := range baseDenominations {
		if !seen[required] {
			return fmt.Errorf("missing required chip denomination %d", required)
		}
	}
	for value := range seen {
		base := false
		for _, required := range baseDenominations {
			if value == required {
				base = true
				break
			}
		}
		if !base && !largeDenominations[value] {
			return fmt.Errorf("unsupported large chip denomination %d", value)
		}
	}
	return nil
}

func NormalizeDenominations(values []int64) []int64 {
	result := append([]int64(nil), values...)
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}
