package game

import (
	"cluedo-toolbox/internal/config"
	"io/ioutil"
	"math/rand"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestGameDeal(t *testing.T) {
	// GIVEN a standard game configuration
	cfg, _ := config.Load("../../default_config.json")
	log := logrus.New()
	log.SetOutput(ioutil.Discard)
	seededRand := rand.New(rand.NewSource(1))

	// WHEN we build a new game (which calls deal() automatically)
	game, err := NewBuilder(cfg, log, seededRand).WithAIPlayers(4).Build()
	if err != nil {
		t.Fatalf("Failed to build game: %v", err)
	}

	// THEN the resulting game state must be valid
	t.Run("solution has one of each card type", func(t *testing.T) {
		if _, ok := game.Solution[config.CategorySuspect]; !ok {
			t.Error("Solution is missing a suspect")
		}
		if _, ok := game.Solution[config.CategoryWeapon]; !ok {
			t.Error("Solution is missing a weapon")
		}
		if _, ok := game.Solution[config.CategoryRoom]; !ok {
			t.Error("Solution is missing a room")
		}
		if len(game.Solution) != 3 {
			t.Errorf("Expected solution to have 3 cards, but it has %d", len(game.Solution))
		}
	})

	t.Run("all cards are accounted for", func(t *testing.T) {
		totalCardsInHands := 0
		for _, p := range game.Players {
			totalCardsInHands += len(p.Hand())
		}
		totalCards := len(game.Solution) + totalCardsInHands

		if totalCards != len(cfg.AllCards) {
			t.Errorf("Card count mismatch. Expected %d total cards, but accounted for %d", len(cfg.AllCards), totalCards)
		}
	})

	t.Run("no player has a solution card", func(t *testing.T) {
		solutionCards := make(map[string]struct{})
		for _, card := range game.Solution {
			solutionCards[card] = struct{}{}
		}

		for _, p := range game.Players {
			for _, card := range p.Hand() {
				if _, isSolution := solutionCards[card]; isSolution {
					t.Errorf("Player %s was dealt a solution card: %s", p.Name(), card)
				}
			}
		}
	})
}
