package game

import (
	"cluedo-toolbox/internal/config"
	"cluedo-toolbox/internal/events"
	"cluedo-toolbox/internal/player"
	"math/rand"
	"sort"
	"time"

	"github.com/sirupsen/logrus"
)

// Game represents the state and logic of a single Cluedo game.
type Game struct {
	Config       *config.GameConfig
	Players      []player.Player
	Solution     map[config.CardCategory]string
	EventManager *events.Manager
	turn         int
	log          *logrus.Logger
	rand         *rand.Rand
}

// deal initializes the solution and deals the remaining cards to players.
func (g *Game) deal() {
	deck := make([]string, len(g.Config.AllCards))
	copy(deck, g.Config.AllCards)
	g.rand.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })

	dealtCategories := make(map[config.CardCategory]bool)
	var cardsToDeal []string
	for i := len(deck) - 1; i >= 0; i-- {
		card := deck[i]
		category := g.Config.CardToType[card]
		if _, exists := dealtCategories[category]; !exists {
			g.Solution[category] = card
			dealtCategories[category] = true
		} else {
			cardsToDeal = append(cardsToDeal, card)
		}
	}
	sort.Strings(cardsToDeal)

	hands := make([][]string, len(g.Players))
	for i, card := range cardsToDeal {
		playerIndex := i % len(g.Players)
		hands[playerIndex] = append(hands[playerIndex], card)
	}

	for i, p := range g.Players {
		p.ReceiveHand(hands[i])
		g.log.Debugf("%s Hand: %v", p.Name(), hands[i])
	}
	g.log.Debugf("Ground Truth Initialized. Solution: %+v", g.Solution)
}

// handleSuggestion processes a suggestion, finding a disprover.
func (g *Game) handleSuggestion(suggester player.Player, suggestion map[config.CardCategory]string) (string, string) {
	suggesterIdx := -1
	for i, p := range g.Players {
		if p.Name() == suggester.Name() {
			suggesterIdx = i
			break
		}
	}

	for i := 1; i < len(g.Players); i++ {
		playerIdx := (suggesterIdx + i) % len(g.Players)
		cardShown := g.Players[playerIdx].ChooseCardToShow(suggestion)
		if cardShown != "" {
			return g.Players[playerIdx].Name(), cardShown
		}
	}
	return "", ""
}

// RunSimulation executes the main game loop until a winner is found or the turn limit is reached.
// RunSimulation is now a pure, "headless" game loop.
func (g *Game) RunSimulation() (string, bool) {
	for g.turn < 50 {
		currentPlayer := g.Players[g.turn%len(g.Players)]
		g.EventManager.Publish(events.TurnStartEvent{TurnNumber: g.turn + 1, PlayerName: currentPlayer.Name()})

		if accusation := currentPlayer.ShouldAccuse(); accusation != nil {
			isCorrect := g.checkAccusation(accusation)
			g.EventManager.Publish(events.GameOverEvent{
				Winner:     currentPlayer.Name(),
				Solution:   g.Solution,
				Accusation: accusation,
				IsCorrect:  isCorrect,
			})
			return currentPlayer.Name(), isCorrect
		}

		suggestion := currentPlayer.MakeSuggestion()
		g.EventManager.Publish(events.SuggestionMadeEvent{PlayerName: currentPlayer.Name(), Suggestion: suggestion})
		disproverName, revealedCard := g.handleSuggestion(currentPlayer, suggestion)

		if disproverName != "" {
			g.EventManager.Publish(events.DisprovalEvent{SuggesterName: currentPlayer.Name(), DisproverName: disproverName, RevealedCard: revealedCard})
		} else {
			g.EventManager.Publish(events.NoDisprovalEvent{})
		}

		// Notify all players for their internal logic.
		// Each player only gets to see the revealed card if they are the suggester.
		for _, p := range g.Players {
			logicEvent := events.TurnResolvedEvent{
				SuggesterName: currentPlayer.Name(),
				Suggestion:    suggestion,
				DisproverName: disproverName,
			}
			if p.Name() == currentPlayer.Name() {
				logicEvent.RevealedCard = revealedCard
			}
			p.HandleEvent(logicEvent)
		}

		g.turn++
		if !currentPlayer.IsHuman() {
			time.Sleep(50 * time.Millisecond)
		}
	}

	// Game ended on turn limit
	g.EventManager.Publish(events.GameOverEvent{Solution: g.Solution})
	return "", false
}

func (g *Game) checkAccusation(accusation map[config.CardCategory]string) bool {
	for cat, card := range accusation {
		if g.Solution[cat] != card {
			return false
		}
	}
	return true
}
