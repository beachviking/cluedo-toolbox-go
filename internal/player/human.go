package player

import (
	"cluedo-toolbox/internal/config"
	"cluedo-toolbox/internal/events"
	"sort"
)

// HumanPlayer represents a player controlled by a person.
type HumanPlayer struct {
	name         string
	cfg          *config.GameConfig
	hand         map[string]struct{}
	eventManager *events.Manager // Add this field
}

// NewHumanPlayer now accepts the event manager it will publish to.
func NewHumanPlayer(eventManager *events.Manager) *HumanPlayer {
	return &HumanPlayer{
		hand:         make(map[string]struct{}),
		eventManager: eventManager,
	}
}

func (h *HumanPlayer) Name() string  { return h.name }
func (h *HumanPlayer) IsHuman() bool { return true }
func (h *HumanPlayer) Hand() []string {
	var cards []string
	for card := range h.hand {
		cards = append(cards, card)
	}
	sort.Strings(cards)
	return cards
}

func (h *HumanPlayer) Setup(cfg *config.GameConfig, playerNames []string, myName string) {
	h.name = myName
	h.cfg = cfg
}

func (h *HumanPlayer) ReceiveHand(cards []string) {
	for _, card := range cards {
		h.hand[card] = struct{}{}
	}
	// *** THE FIX: Publish an event instead of printing directly. ***
	h.eventManager.Publish(events.HumanHandRevealedEvent{
		PlayerName: h.name,
		Hand:       h.Hand(),
	})
}

func (h *HumanPlayer) HandleEvent(e events.Event) {
	// switch event := e.(type) {
	// // Listen for the logic event, not the rendering event
	// case events.TurnResolvedEvent:
	// 	// When the human is the suggester and saw a card, we need to announce it.
	// 	// For a simulation, this is enough. For an interactive human game,
	// 	// the CLI would handle this announcement.
	// 	if h.name == event.SuggesterName && event.RevealedCard != "" {
	// 		// This could publish another event like "CardRevealedToHuman" if needed.
	// 		// For now, we keep it simple.
	// 	}
	// }
}

func (h *HumanPlayer) ChooseCardToShow(suggestion map[config.CardCategory]string) string {
	var canShow []string
	for _, card := range suggestion {
		if _, ok := h.hand[card]; ok {
			canShow = append(canShow, card)
		}
	}
	if len(canShow) == 0 {
		return ""
	}
	// In simulation, auto-pick. In a real game, this would prompt the user.
	return canShow[0]
}

func (h *HumanPlayer) DisplayNotes() {}

// MakeSuggestion and ShouldAccuse are handled by the interactive CLI loop for humans.
func (h *HumanPlayer) MakeSuggestion() map[config.CardCategory]string { return nil }
func (h *HumanPlayer) ShouldAccuse() map[config.CardCategory]string   { return nil }
