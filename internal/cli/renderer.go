package cli

import (
	"cluedo-toolbox/internal/ai"
	"cluedo-toolbox/internal/events"
	"cluedo-toolbox/internal/player"
	"fmt"
	"strings"
)

// SimulationRenderer implements the events.Listener interface to print game state to the console.
type SimulationRenderer struct{}

// HandleEvent is the central dispatcher for rendering events.
func (r *SimulationRenderer) HandleEvent(e events.Event) {
	switch event := e.(type) {
	case events.GameReadyEvent:
		C.Header.Println("--- Starting Game: Initial State ---")
		// Find the first AI player to display their initial notes.
		players, ok := event.Players.([]player.Player)
		if !ok {
			// This should never happen in our application, but it's safe to check.
			return
		}

		for _, p := range players {
			if !p.IsHuman() {
				DisplayAINotes(p) // Use the renamed helper function
				break
			}
		}
	case events.HumanHandRevealedEvent: // <-- Add this new case
		var cardParts []string
		for _, card := range event.Hand {
			cardParts = append(cardParts, ColorizeCard(card))
		}
		C.Info.Printf("\n%s's hand: %s\n", ColorizeCard(event.PlayerName), strings.Join(cardParts, ", "))
	case events.TurnStartEvent:
		C.Header.Printf("\n--- Turn %d: %s ---\n", event.TurnNumber, ColorizeCard(event.PlayerName))
	case events.SuggestionMadeEvent:
		var parts []string
		for _, card := range event.Suggestion {
			parts = append(parts, ColorizeCard(card))
		}
		C.Info.Printf("%s suggests: %s\n", ColorizeCard(event.PlayerName), strings.Join(parts, ", "))
	case events.DisprovalEvent:
		C.Info.Printf("-> %s shows a card to %s.\n", ColorizeCard(event.DisproverName), ColorizeCard(event.SuggesterName))
	case events.NoDisprovalEvent:
		C.Info.Println("-> No player could show a card.")
	case events.GameOverEvent:
		r.renderGameResult(event)
	}
}

func (r *SimulationRenderer) renderGameResult(event events.GameOverEvent) {
	C.Header.Println("\n--- GAME OVER ---")
	if event.Accusation != nil {
		var parts []string
		for _, card := range event.Accusation {
			parts = append(parts, ColorizeCard(card))
		}
		C.Info.Printf("%s ACCUSED with: %s\n", ColorizeCard(event.Winner), strings.Join(parts, ", "))
		if event.IsCorrect {
			C.Yes.Printf("The accusation is CORRECT! %s wins!\n", ColorizeCard(event.Winner))
		} else {
			C.No.Printf("The accusation is INCORRECT! %s is out of the game.\n", event.Winner)
		}
	}

	solutionForPrinting := make(map[string]string)
	for cat, card := range event.Solution {
		solutionForPrinting[cat.String()] = card
	}
	C.Info.Printf("The correct solution was: %v\n", solutionForPrinting)

	if event.Winner == "" && event.Accusation == nil {
		C.Warn.Println("Game ended without a correct accusation.")
	}
}

// DisplayWinnerNotes is a helper function to render the final board state of the winning AI.
// func DisplayWinnerNotes(p player.Player) {
// 	if brain, ok := p.(*ai.AdvancedAIBrain); ok {
// 		fmt.Println()
// 		RenderNotes(
// 			brain.Name(),
// 			brain.Config(),
// 			brain.Players(),
// 			brain.Knowledge(),
// 		)
// 	}
// }

// DisplayAINotes is a helper function to render the final board state of an AI player.
func DisplayAINotes(p player.Player) {
	if brain, ok := p.(*ai.AdvancedAIBrain); ok {
		fmt.Println()
		if brain.Name() != "" {
			C.Header.Printf("--- Notes for %s ---\n", ColorizeCard(brain.Name()))
		}
		RenderNotes(
			brain.Name(),
			brain.Config(),
			brain.Players(),
			brain.Knowledge(),
		)
	}
}
