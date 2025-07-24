package events

import (
	"cluedo-toolbox/internal/config"
)

// Event is a marker interface for all event types.
type Event interface{}

// Listener defines an interface for any component that wants to react to events.
type Listener interface {
	HandleEvent(e Event)
}

// Manager (or Event Bus) manages listeners and dispatches events.
type Manager struct {
	listeners []Listener
}

func NewManager() *Manager {
	return &Manager{}
}
func (em *Manager) Subscribe(l Listener) {
	em.listeners = append(em.listeners, l)
}
func (em *Manager) Publish(e Event) {
	for _, l := range em.listeners {
		l.HandleEvent(e)
	}
}

// --- Event Types for Rendering ---

type TurnStartEvent struct {
	TurnNumber int
	PlayerName string
}

type SuggestionMadeEvent struct {
	PlayerName string
	Suggestion map[config.CardCategory]string
}

type DisprovalEvent struct {
	SuggesterName string
	DisproverName string
	RevealedCard  string // Ground truth, for logging
}

type NoDisprovalEvent struct{}

// GameReadyEvent is published once the game is built and cards are dealt.
// It signals the start of the simulation and provides the initial player state.
type GameReadyEvent struct {
	Players interface{}
}

type GameOverEvent struct {
	Winner     string
	Solution   map[config.CardCategory]string
	Accusation map[config.CardCategory]string // The final accusation made
	IsCorrect  bool
}

// --- Event Type for AI Logic ---

// TurnResolvedEvent is for the AI's internal logic. It contains the complete turn result.
type TurnResolvedEvent struct {
	SuggesterName string
	Suggestion    map[config.CardCategory]string
	DisproverName string // Empty if no one disproved
	RevealedCard  string // Card shown TO A SPECIFIC PLAYER; empty if not seen
}

type HumanHandRevealedEvent struct {
	PlayerName string
	Hand       []string
}
