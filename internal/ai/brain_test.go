package ai

import (
	"cluedo-toolbox/internal/config"
	"io/ioutil"
	"math/rand"
	"testing"

	"github.com/sirupsen/logrus"
)

// setupTestAI is a helper function to create a clean AI instance for each test.
// This ensures tests are isolated from each other.
func setupTestAI() (*AdvancedAIBrain, *config.GameConfig) {
	// GIVEN a standard game configuration and a set of players
	cfg, _ := config.Load("../../default_config.json") // Assumes test is run from root
	playerNames := []string{"Player 1", "Player 2", "Player 3"}

	// GIVEN a "null" logger that discards output and a predictable random source
	log := logrus.New()
	log.SetOutput(ioutil.Discard)
	seededRand := rand.New(rand.NewSource(1)) // Use a fixed seed for deterministic tests

	// GIVEN a new AI brain
	chooser := &DeterministicChooser{}
	brain := NewAdvancedAIBrain(log, seededRand, chooser)
	brain.Setup(cfg.DeepCopy(), playerNames, "Player 1")

	// brain := NewAdvancedAIBrain(log, seededRand)
	// brain.Setup(cfg.DeepCopy(), playerNames, "Player 1")

	return brain, cfg
}

func TestMarkCardLocation(t *testing.T) {
	// GIVEN a fresh AI brain
	brain, _ := setupTestAI()

	// WHEN we learn a definitive fact (e.g., Player 2 has the Wrench)
	brain._markCardLocation("Wrench", "Player 2")

	// THEN the knowledge grid for "Wrench" should be correct
	t.Run("it marks the owner as Yes", func(t *testing.T) {
		if brain.knowledge["Wrench"]["Player 2"] != StatusYes {
			t.Errorf("Expected Player 2 to have Wrench, but status was not Yes")
		}
	})

	t.Run("it marks all other players as No", func(t *testing.T) {
		if brain.knowledge["Wrench"]["Player 1"] != StatusNo {
			t.Errorf("Expected Player 1 to NOT have Wrench, but status was not No")
		}
		if brain.knowledge["Wrench"]["Player 3"] != StatusNo {
			t.Errorf("Expected Player 3 to NOT have Wrench, but status was not No")
		}
	})

	t.Run("it marks the solution as No", func(t *testing.T) {
		if brain.knowledge["Wrench"]["solution"] != StatusNo {
			t.Errorf("Expected solution to NOT have Wrench, but status was not No")
		}
	})
}

func TestDeduceCardByElimination(t *testing.T) {
	// GIVEN an AI that knows a card is NOT held by anyone except Player 3
	brain, _ := setupTestAI()
	brain.knowledge["Rope"]["Player 1"] = StatusNo
	brain.knowledge["Rope"]["Player 2"] = StatusNo
	brain.knowledge["Rope"]["solution"] = StatusNo

	// WHEN the deduction loop runs
	changed := brain._deduceCardLocationsByElimination()

	// THEN it should deduce Player 3 must have the Rope
	if !changed {
		t.Errorf("Expected the deduction to make a change, but it did not")
	}
	if brain.knowledge["Rope"]["Player 3"] != StatusYes {
		t.Errorf("Expected Player 3 to be marked as having the Rope, but status was %v", brain.knowledge["Rope"]["Player 3"])
	}
}

func TestDeduceSolutionByElimination(t *testing.T) {
	// GIVEN an AI that knows all suspects except one are NOT in the solution
	brain, cfg := setupTestAI()
	for _, suspect := range cfg.Suspects {
		if suspect != "Mrs. Peacock" {
			brain.knowledge[suspect]["solution"] = StatusNo
		}
	}

	// WHEN the deduction loop runs
	changed := brain._deduceSolutionByElimination()

	// THEN it should deduce Mrs. Peacock must be in the solution
	if !changed {
		t.Errorf("Expected the deduction to make a change, but it did not")
	}
	if brain.knowledge["Mrs. Peacock"]["solution"] != StatusYes {
		t.Errorf("Expected Mrs. Peacock to be marked as the solution suspect, but status was not Yes")
	}
}

func TestPruneAndSolveMystery(t *testing.T) {
	t.Run("it prunes a mystery when a card is eliminated", func(t *testing.T) {
		// GIVEN an AI that thinks Player 2 has one of (Rope, Dagger, Pipe)
		brain, _ := setupTestAI()
		brain.unresolvedSuggestions = []UnresolvedSuggestion{
			{Disprover: "Player 2", PossibleCards: map[string]struct{}{"Rope": {}, "Dagger": {}, "Lead Pipe": {}}},
		}
		// AND we later learn Player 2 does NOT have the Dagger
		brain.knowledge["Dagger"]["Player 2"] = StatusNo

		// WHEN we prune mysteries
		brain._pruneAndSolveMysteries()

		// THEN the mystery should be narrowed down
		if len(brain.unresolvedSuggestions) != 1 {
			t.Fatalf("Expected 1 unresolved suggestion, got %d", len(brain.unresolvedSuggestions))
		}
		remainingCards := brain.unresolvedSuggestions[0].PossibleCards
		if len(remainingCards) != 2 {
			t.Errorf("Expected mystery to be pruned to 2 cards, but had %d", len(remainingCards))
		}
		if _, ok := remainingCards["Dagger"]; ok {
			t.Errorf("Expected Dagger to be pruned from mystery, but it was still present")
		}
	})

	t.Run("it solves a mystery when only one card remains", func(t *testing.T) {
		// GIVEN an AI with a mystery that has been pruned to one card
		brain, _ := setupTestAI()
		brain.unresolvedSuggestions = []UnresolvedSuggestion{
			{Disprover: "Player 3", PossibleCards: map[string]struct{}{"Conservatory": {}}},
		}

		// WHEN we prune mysteries
		brain._pruneAndSolveMysteries()

		// THEN the mystery should be resolved and the knowledge updated
		if len(brain.unresolvedSuggestions) != 0 {
			t.Errorf("Expected mystery to be resolved and removed, but %d remain", len(brain.unresolvedSuggestions))
		}
		if brain.knowledge["Conservatory"]["Player 3"] != StatusYes {
			t.Errorf("Expected to learn Player 3 has the Conservatory, but knowledge was not updated")
		}
	})
}
