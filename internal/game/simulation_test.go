package game

import (
	"cluedo-toolbox/internal/ai"
	"cluedo-toolbox/internal/config"
	"cluedo-toolbox/internal/events"
	"cluedo-toolbox/internal/player"
	"io/ioutil"
	"math/rand"
	"testing"

	"github.com/sirupsen/logrus"
)

// setupDeterministicGoldenGame manually constructs the exact game state from the known correct log.
// This removes all randomness from the test setup, ensuring a predictable run.
func setupDeterministicGoldenGame(t *testing.T) *Game {
	// t.Helper() marks this as a test helper function.
	t.Helper()

	cfg, err := config.Load("../../default_config.json")
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	log := logrus.New()
	log.SetOutput(ioutil.Discard)
	// For debugging this test, uncomment the line below:
	// log.SetLevel(logrus.DebugLevel)

	// We still need a rand source for the AI's internal choices (like which card to show),
	// but it no longer affects the game setup.
	seededRand := rand.New(rand.NewSource(1))

	// 1. Manually set the known solution from the log.
	solution := map[config.CardCategory]string{
		config.CategorySuspect: "Mrs. White",
		config.CategoryWeapon:  "Lead Pipe",
		config.CategoryRoom:    "Kitchen",
	}

	// 2. Manually set the player order from the log.
	playerOrder := []string{"Miss Scarlett", "Mr. Green", "Colonel Mustard"}

	// 3. Manually set the hands for each player, deduced from the log and final notebook.
	hands := map[string][]string{
		"Miss Scarlett":   {"Colonel Mustard", "Mrs. Peacock", "Rope", "Ballroom", "Dining Room", "Lounge"},
		"Mr. Green":       {"Professor Plum", "Hall", "Conservatory", "Mr. Green", "Study", "Billiard Room"}, // 'Mr. Green' card deduced by elimination
		"Colonel Mustard": {"Miss Scarlett", "Dagger", "Candlestick", "Wrench", "Revolver", "Library"},       // 'Miss Scarlett' card deduced by elimination
	}

	// 4. Manually construct the game.
	eventManager := events.NewManager()
	game := &Game{
		Config:       cfg,
		Players:      []player.Player{},
		Solution:     solution,
		EventManager: eventManager,
		log:          log,
		rand:         seededRand,
	}

	// 5. Manually create, set up, and deal cards to each player.
	for _, name := range playerOrder {
		aiRand := rand.New(rand.NewSource(seededRand.Int63()))
		chooser := &ai.DeterministicChooser{} // Use the predictable chooser
		brain := ai.NewAdvancedAIBrain(log, aiRand, chooser)

		brain.Setup(cfg.DeepCopy(), playerOrder, name)
		brain.ReceiveHand(hands[name])
		game.Players = append(game.Players, brain)
		eventManager.Subscribe(brain)

		// brain := ai.NewAdvancedAIBrain(log, rand.New(rand.NewSource(seededRand.Int63())))
		// brain.Setup(cfg.DeepCopy(), playerOrder, name)
		// brain.ReceiveHand(hands[name]) // Deal the predetermined hand
		// game.Players = append(game.Players, brain)
		// eventManager.Subscribe(brain)
	}

	return game
}

func TestFullSimulation_GoldenRun(t *testing.T) {
	// GIVEN a game constructed with the exact state from the known correct log
	game := setupDeterministicGoldenGame(t)

	// WHEN we run the entire simulation to its conclusion
	winnerName, isCorrect := game.RunSimulation()

	// THEN the final outcome must match our "golden master" run exactly.

	t.Run("it produces the correct winner", func(t *testing.T) {
		expectedWinner := "Colonel Mustard"
		if winnerName != expectedWinner {
			t.Errorf("expected winner to be %s, but got %s", expectedWinner, winnerName)
		}
	})

	t.Run("the accusation was correct", func(t *testing.T) {
		if !isCorrect {
			t.Error("expected the final accusation to be correct, but it was not")
		}
	})

	t.Run("the game ended at the correct turn", func(t *testing.T) {
		// The log shows the accusation happens on Turn 15.
		// The game.turn counter will be 14 (since it's 0-indexed).
		expectedTurnCount := 11
		if game.turn != expectedTurnCount {
			t.Errorf("expected game to end on turn %d, but it ended on turn %d", expectedTurnCount, game.turn)
		}
	})
}
