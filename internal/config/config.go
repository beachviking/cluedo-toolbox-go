package config

import (
	"encoding/json"
	"io/ioutil"
	"sort"
)

// CardCategory defines the type of a card using a typed enum.
type CardCategory int

const (
	CategorySuspect CardCategory = iota
	CategoryWeapon
	CategoryRoom
)

func (cc CardCategory) String() string {
	return []string{"suspects", "weapons", "rooms"}[cc]
}

// GameConfig holds the static definitions for a game of Cluedo.
type GameConfig struct {
	Suspects   []string                `json:"suspects"`
	Weapons    []string                `json:"weapons"`
	Rooms      []string                `json:"rooms"`
	AllCards   []string                `json:"-"`
	CardToType map[string]CardCategory `json:"-"`
}

// Load reads, parses, and prepares the game configuration from a file.
func Load(path string) (*GameConfig, error) {
	var cfg GameConfig
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	cfg.CardToType = make(map[string]CardCategory)
	sort.Strings(cfg.Suspects)
	sort.Strings(cfg.Weapons)
	sort.Strings(cfg.Rooms)

	for _, card := range cfg.Suspects {
		cfg.AllCards = append(cfg.AllCards, card)
		cfg.CardToType[card] = CategorySuspect
	}
	for _, card := range cfg.Weapons {
		cfg.AllCards = append(cfg.AllCards, card)
		cfg.CardToType[card] = CategoryWeapon
	}
	for _, card := range cfg.Rooms {
		cfg.AllCards = append(cfg.AllCards, card)
		cfg.CardToType[card] = CategoryRoom
	}
	return &cfg, nil
}

// DeepCopy creates a new GameConfig with all slices copied to prevent shared state.
func (c *GameConfig) DeepCopy() *GameConfig {
	newCfg := &GameConfig{
		CardToType: make(map[string]CardCategory),
	}
	newCfg.Suspects = make([]string, len(c.Suspects))
	copy(newCfg.Suspects, c.Suspects)
	newCfg.Weapons = make([]string, len(c.Weapons))
	copy(newCfg.Weapons, c.Weapons)
	newCfg.Rooms = make([]string, len(c.Rooms))
	copy(newCfg.Rooms, c.Rooms)
	newCfg.AllCards = make([]string, len(c.AllCards))
	copy(newCfg.AllCards, c.AllCards)
	for k, v := range c.CardToType {
		newCfg.CardToType[k] = v
	}
	return newCfg
}

// CardListForCategory is a helper to get the correct card list from the config.
func (c *GameConfig) CardListForCategory(cat CardCategory) []string {
	switch cat {
	case CategorySuspect:
		return c.Suspects
	case CategoryWeapon:
		return c.Weapons
	case CategoryRoom:
		return c.Rooms
	default:
		return nil
	}
}
