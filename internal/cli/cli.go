package cli

import (
	"cluedo-toolbox/internal/ai"
	"cluedo-toolbox/internal/config"
	"cluedo-toolbox/internal/events"
	"cluedo-toolbox/internal/game"
	"cluedo-toolbox/internal/player"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"strconv"
	"strings"

	"github.com/peterh/liner"
	"github.com/sirupsen/logrus"
)

// CLI manages all command-line interactions.
type CLI struct {
	log  *logrus.Logger
	line *liner.State
}

// NewCLI creates a new command-line interface manager.
func NewCLI(log *logrus.Logger) *CLI {
	line := liner.NewLiner()
	line.SetCtrlCAborts(true)
	return &CLI{
		log:  log,
		line: line,
	}
}

// Run is the main entry point for the CLI application.
func (c *CLI) Run(args []string, cfg *config.GameConfig, rand *rand.Rand) error {
	defer c.line.Close()
	if len(args) < 1 {
		c.printUsage()
		return errors.New("no command provided")
	}

	switch args[0] {
	case "detective":
		return c.runDetectiveMode(cfg)
	case "start":
		if len(args) != 3 {
			c.printUsage()
			return errors.New("invalid arguments for 'start' command")
		}
		numHumans, _ := strconv.Atoi(args[1])
		numAI, _ := strconv.Atoi(args[2])
		return c.runSimulationMode(cfg, numHumans, numAI, rand)
	default:
		c.printUsage()
		return fmt.Errorf("unknown command '%s'", args[0])
	}
}

func (c *CLI) runSimulationMode(cfg *config.GameConfig, numHumans, numAI int, rand *rand.Rand) error {
	C.Header.Println("--- Running Fast Simulation ---")

	// Create a builder and subscribe our new renderer to it.
	builder := game.NewBuilder(cfg, c.log, rand)
	renderer := &SimulationRenderer{}
	builder.EventManager().Subscribe(renderer)

	game, err := builder.WithHumanPlayers(numHumans).WithAIPlayers(numAI).Build()
	if err != nil {
		return fmt.Errorf("failed to build game: %w", err)
	}

	// Run the simulation and get the result
	winnerName, _ := game.RunSimulation()

	// If there was a winner, find the player object and display their notes
	if winnerName != "" {
		for _, p := range game.Players {
			if p.Name() == winnerName {
				DisplayAINotes(p)
				// DisplayWinnerNotes(p)
				break
			}
		}
	}
	return nil
}

func (c *CLI) runDetectiveMode(cfg *config.GameConfig) error {
	C.Info.Println("\n--- Starting Detective Mode Co-Pilot ---")
	numPlayers := c.promptForInt("How many players are in the real game? (2-6): ", 2, 6)
	var playerNames []string
	for i := 0; i < numPlayers; i++ {
		name := c.promptForString(fmt.Sprintf("Enter name for Player %d: ", i+1))
		playerNames = append(playerNames, name)
	}
	myPlayerName := c.promptForSelection("Which player are you?", playerNames)
	C.Info.Println("\nSelect the cards in your hand. Type 'done' when finished.")
	myHand := c.promptForCards(cfg, true, 0)

	// Create and set up the AI brain
	rand := rand.New(rand.NewSource(1))
	chooser := ai.NewRandomChooser(rand)
	brain := ai.NewAdvancedAIBrain(c.log, rand, chooser)

	// brain := ai.NewAdvancedAIBrain(c.log, rand.New(rand.NewSource(1)))
	pNamesCopy := make([]string, len(playerNames))
	copy(pNamesCopy, playerNames)
	brain.Setup(cfg.DeepCopy(), pNamesCopy, myPlayerName)
	brain.ReceiveHand(myHand)

	C.Info.Println("\nDetective Mode is active! Your co-pilot is ready.")
	c.handleNotesCommand(brain) // Initial display
	c.printDetectiveHelp()

	// Main command loop for detective mode
	for {
		input, err := c.line.Prompt("(detective) ")
		if err != nil {
			if err == liner.ErrPromptAborted || err == io.EOF {
				C.Info.Println("\nGoodbye!")
				return nil
			}
			return fmt.Errorf("error reading line: %w", err)
		}
		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}
		c.line.AppendHistory(input)
		parts := strings.Fields(input)
		cmd := strings.ToLower(parts[0])

		switch cmd {
		case "log", "l":
			c.handleLogCommand(brain)
		case "reveal", "r":
			c.handleRevealCommand(brain)
		case "suggest", "s":
			c.handleSuggestCommand(brain)
		case "notes", "n":
			brain.DisplayNotes()
		case "hand", "ha":
			c.handleHandCommand(brain)
		case "help", "h":
			c.printDetectiveHelp()
		case "quit", "q":
			C.Info.Println("Exiting detective mode.")
			return nil
		default:
			C.Warn.Printf("Unknown command '%s'. Type 'help' for a list of commands.\n", cmd)
		}
	}
}

// handleNotesCommand now fetches data from the AI and calls the renderer.
func (c *CLI) handleNotesCommand(brain *ai.AdvancedAIBrain) {
	RenderNotes(
		brain.Name(),
		brain.Config(),
		brain.Players(),
		brain.Knowledge(),
	)
}

func (c *CLI) handleLogCommand(brain *ai.AdvancedAIBrain) {
	C.Info.Println("\n--- Log a Game Turn ---")
	suggester := c.promptForSelection("Who made the suggestion?", brain.Hand())
	C.Info.Println("What 3 cards were suggested?")
	suggestionCards := c.promptForCards(brain.Config(), false, 3)
	if len(suggestionCards) != 3 {
		C.Warn.Println("Error: A suggestion must have exactly 3 cards.")
		return
	}
	suggestion := make(map[config.CardCategory]string)
	for _, card := range suggestionCards {
		suggestion[brain.Config().CardToType[card]] = card
	}

	disproverOptions := append(brain.Hand(), "No One")
	disprover := c.promptForSelection("Who disproved the suggestion?", disproverOptions)

	event := events.TurnResolvedEvent{SuggesterName: suggester, Suggestion: suggestion}
	if disprover != "No One" {
		event.DisproverName = disprover
		if suggester == brain.Name() {
			C.Info.Println("What card were you shown?")
			revealedCards := c.promptForCards(brain.Config(), true, 1)
			if len(revealedCards) > 0 {
				event.RevealedCard = revealedCards[0]
			}
		}
	}
	brain.HandleEvent(event)
	C.Info.Println("Turn logged. Here are your updated notes:")
	brain.DisplayNotes()
}

func (c *CLI) handleRevealCommand(brain *ai.AdvancedAIBrain) {
	C.Info.Println("\n--- Log a Revealed Card ---")
	pName := c.promptForSelection("Which player revealed a card?", brain.Hand())
	C.Info.Println("Which card did they reveal?")
	revealedCards := c.promptForCards(brain.Config(), true, 1)
	if len(revealedCards) == 0 {
		return
	}
	event := events.TurnResolvedEvent{
		SuggesterName: "Game Event",
		DisproverName: pName,
		RevealedCard:  revealedCards[0],
	}
	brain.HandleEvent(event)
	C.Info.Println("Revealed card logged.")
	brain.DisplayNotes()
}

func (c *CLI) handleSuggestCommand(brain *ai.AdvancedAIBrain) {
	C.Header.Println("\n--- AI Co-Pilot Suggestion ---")
	suggestion := brain.MakeSuggestion()
	var parts []string
	for _, cat := range []config.CardCategory{config.CategorySuspect, config.CategoryWeapon, config.CategoryRoom} {
		parts = append(parts, ColorizeCard(suggestion[cat]))
	}
	C.Info.Printf("The AI suggests you propose: %s\n", strings.Join(parts, ", "))
}

func (c *CLI) handleHandCommand(brain player.Player) {
	C.Header.Println("\n--- Your Hand ---")
	for _, card := range brain.Hand() {
		C.Info.Println(" - " + ColorizeCard(card))
	}
}
