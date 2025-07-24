package ai

import (
	"math/rand"
	"sort"
)

// Chooser defines an interface for selecting a single card from a list of options.
// This allows us to swap out random and deterministic selection strategies.
type Chooser interface {
	Choose(cards []string) string
}

// --- Implementations ---

// RandomChooser implements the Chooser interface by picking an element randomly.
type RandomChooser struct {
	rand *rand.Rand
}

// NewRandomChooser creates a new random chooser.
func NewRandomChooser(rand *rand.Rand) *RandomChooser {
	return &RandomChooser{rand: rand}
}

func (r *RandomChooser) Choose(cards []string) string {
	if len(cards) == 0 {
		return ""
	}
	return cards[r.rand.Intn(len(cards))]
}

// DeterministicChooser implements the Chooser interface by always picking the first
// card alphabetically. This is used for predictable testing.
type DeterministicChooser struct{}

func (d *DeterministicChooser) Choose(cards []string) string {
	if len(cards) == 0 {
		return ""
	}
	// Sort the slice to guarantee the choice is always the same.
	sort.Strings(cards)
	return cards[0]
}
