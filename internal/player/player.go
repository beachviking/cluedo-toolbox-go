package player

import (
	"cluedo-toolbox/internal/config"
	"cluedo-toolbox/internal/events"
)

// Player is the interface that all player types (human or AI) must implement.
// It also implements events.Listener to react to game events.
type Player interface {
	events.Listener // Embed the Listener interface

	Name() string
	IsHuman() bool
	Hand() []string
	Setup(cfg *config.GameConfig, playerNames []string, myName string)
	ReceiveHand(cards []string)
	MakeSuggestion() map[config.CardCategory]string
	ShouldAccuse() map[config.CardCategory]string
	ChooseCardToShow(suggestion map[config.CardCategory]string) string
	DisplayNotes()
}
