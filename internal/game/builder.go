package game

import (
	"cluedo-toolbox/internal/ai"
	"cluedo-toolbox/internal/config"
	"cluedo-toolbox/internal/events"
	"cluedo-toolbox/internal/player"
	"errors"
	"math/rand"

	"github.com/sirupsen/logrus"
)

// GameBuilder provides a step-by-step API for constructing a Game object.
type GameBuilder struct {
	cfg          *config.GameConfig
	eventManager *events.Manager
	log          *logrus.Logger
	rand         *rand.Rand
	numHumans    int
	numAI        int
}

// NewBuilder creates a new GameBuilder with its required dependencies.
func NewBuilder(cfg *config.GameConfig, logger *logrus.Logger, rand *rand.Rand) *GameBuilder {
	return &GameBuilder{
		cfg:          cfg,
		log:          logger,
		rand:         rand,
		eventManager: events.NewManager(),
	}
}

// EventManager is a public getter for the unexported field.
func (b *GameBuilder) EventManager() *events.Manager {
	return b.eventManager
}

func (b *GameBuilder) WithHumanPlayers(n int) *GameBuilder {
	b.numHumans = n
	return b
}

func (b *GameBuilder) WithAIPlayers(n int) *GameBuilder {
	b.numAI = n
	return b
}

// Build constructs the Game object after all options have been configured.
func (b *GameBuilder) Build() (*Game, error) {
	totalPlayers := b.numHumans + b.numAI
	if totalPlayers < 2 || totalPlayers > len(b.cfg.Suspects) {
		return nil, errors.New("invalid number of players")
	}

	// 1. Create and shuffle player names
	playerNames := b.cfg.Suspects[:totalPlayers]
	b.rand.Shuffle(len(playerNames), func(i, j int) { playerNames[i], playerNames[j] = playerNames[j], playerNames[i] })

	// 2. Create the Game object
	game := &Game{
		Config:       b.cfg,
		EventManager: b.eventManager,
		log:          b.log,
		rand:         b.rand,
		Solution:     make(map[config.CardCategory]string),
	}

	// 3. Create players, inject dependencies, and subscribe them to events
	for i, name := range playerNames {
		var p player.Player
		if i < b.numHumans {
			// p = player.NewHumanPlayer()
			p = player.NewHumanPlayer(b.eventManager)

		} else {
			// Inject logger and a new random source for each AI
			aiRand := rand.New(rand.NewSource(b.rand.Int63()))
			chooser := ai.NewRandomChooser(aiRand)
			p = ai.NewAdvancedAIBrain(b.log, aiRand, chooser)
			// p = ai.NewAdvancedAIBrain(b.log, rand.New(rand.NewSource(b.rand.Int63())))
		}

		playerNamesCopy := make([]string, len(playerNames))
		copy(playerNamesCopy, playerNames)
		p.Setup(b.cfg.DeepCopy(), playerNamesCopy, name)

		game.Players = append(game.Players, p)
		b.eventManager.Subscribe(p)
	}

	// 4. Deal the cards
	game.deal()

	b.eventManager.Publish(events.GameReadyEvent{Players: game.Players})

	return game, nil
}
