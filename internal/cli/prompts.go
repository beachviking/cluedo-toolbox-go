package cli

import (
	"cluedo-toolbox/internal/ai"
	"cluedo-toolbox/internal/config"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
)

// C holds pre-configured color objects for printing to the console.
var C = struct {
	Yes, No, Maybe, Info, Warn, Header, Prompt, Debug *color.Color
}{
	Yes:    color.New(color.FgGreen),
	No:     color.New(color.FgRed),
	Maybe:  color.New(color.FgYellow),
	Info:   color.New(color.FgCyan),
	Warn:   color.New(color.FgHiYellow),
	Header: color.New(color.FgWhite, color.Bold),
	Prompt: color.New(color.FgHiWhite),
	Debug:  color.New(color.FgMagenta),
}

// SuspectColors maps suspect names to specific colors for display.
var SuspectColors = map[string]*color.Color{
	"Miss Scarlett":   color.New(color.FgRed),
	"Colonel Mustard": color.New(color.FgYellow),
	"Mrs. White":      color.New(color.FgWhite),
	"Mr. Green":       color.New(color.FgGreen),
	"Mrs. Peacock":    color.New(color.FgBlue),
	"Professor Plum":  color.New(color.FgMagenta),
}

// ColorizeCard returns a card name as a colored string if it's a suspect.
func ColorizeCard(name string) string {
	if c, ok := SuspectColors[name]; ok {
		return c.Sprint(name)
	}
	return name
}

// RenderNotes displays the AI's knowledge grid in a formatted table.
func RenderNotes(playerName string, cfg *config.GameConfig, players []string, knowledge map[string]map[string]ai.CardStatus) {
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.SetTitle(fmt.Sprintf("%s's Detective Notes", playerName))
	header := table.Row{"ID", "Card", "Type"}
	for _, pName := range players {
		header = append(header, ColorizeCard(pName))
	}
	header = append(header, "Solution")
	t.AppendHeader(header)

	for cardID, card := range cfg.AllCards {
		if cardID > 0 && cfg.CardToType[card] != cfg.CardToType[cfg.AllCards[cardID-1]] {
			t.AppendSeparator()
		}
		cat := cfg.CardToType[card]
		row := table.Row{cardID + 1, ColorizeCard(card), cat.String()}
		for _, pName := range players {
			row = append(row, statusToSymbol(knowledge[card][pName]))
		}
		row = append(row, statusToSymbol(knowledge[card]["solution"]))
		t.AppendRow(row)
	}
	t.SetStyle(table.StyleRounded)
	t.Style().Options.SeparateRows = false
	t.Style().Title.Align = text.AlignCenter
	t.SetColumnConfigs([]table.ColumnConfig{{Number: 1, Align: text.AlignRight}})
	t.Render()
}

func statusToSymbol(status ai.CardStatus) string {
	switch status {
	case ai.StatusYes:
		return C.Yes.Sprint("✔")
	case ai.StatusNo:
		return C.No.Sprint("✖")
	default:
		return C.Maybe.Sprint("?")
	}
}

// --- Prompting and Usage ---

func (c *CLI) printUsage() {
	C.Header.Println("\n--- Cluedo Toolbox ---")
	fmt.Println("Usage:")
	fmt.Println("  go run ./cmd/cluedo detective")
	fmt.Println("    To run the AI co-pilot for a real-life game.")
	fmt.Println("  go run ./cmd/cluedo start <humans> <ai>")
	fmt.Println("    To run a fast simulation with a mix of players.")
	fmt.Println("\nFlags:")
	fmt.Println("  -loglevel debug    Enable detailed AI logic tracing.")
}

func (c *CLI) printDetectiveHelp() {
	C.Header.Println("\n--- Detective Mode Help ---")
	fmt.Println("Log events from your real-life game, and the AI will track everything for you.")

	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.AppendHeader(table.Row{"Command", "Alias", "Description"})
	t.AppendSeparator()
	t.AppendRows([]table.Row{
		{"log", "l", "Log a full game turn (suggestion and result)."},
		{"reveal", "r", "Log a single card revealed by a player."},
		{"suggest", "s", "Ask the AI co-pilot for a strategic suggestion."},
		{"notes", "n", "Display the AI's current detective notes grid."},
		{"hand", "ha", "Display the cards currently in your hand."},
		{"help", "h", "Show this help message."},
		{"quit", "q", "Exit detective mode."},
	})
	t.SetStyle(table.StyleLight)
	t.Render()

	C.Prompt.Print("\nEnter a command: ")
}

func (c *CLI) promptForString(prompt string) string {
	for {
		C.Prompt.Print(prompt)
		input, err := c.line.Prompt("")
		if err != nil {
			C.Info.Println("\nGoodbye!")
			os.Exit(0)
		}
		trimmed := strings.TrimSpace(input)
		if trimmed != "" {
			c.line.AppendHistory(trimmed)
			return trimmed
		}
	}
}

func (c *CLI) promptForInt(prompt string, min, max int) int {
	for {
		input := c.promptForString(prompt)
		num, err := strconv.Atoi(input)
		if err != nil || num < min || num > max {
			C.Warn.Printf("Invalid input. Please enter a number between %d and %d.\n", min, max)
			continue
		}
		return num
	}
}

func (c *CLI) promptForSelection(prompt string, options []string) string {
	for {
		C.Header.Println("\n" + prompt)
		for i, opt := range options {
			fmt.Printf(" %2d: %s\n", i+1, ColorizeCard(opt))
		}
		input := c.promptForString("Enter number or name: ")
		if num, err := strconv.Atoi(input); err == nil && num >= 1 && num <= len(options) {
			return options[num-1]
		}
		for _, opt := range options {
			if strings.EqualFold(opt, input) {
				return opt
			}
		}
		C.Warn.Println("Invalid selection.")
	}
}

func (c *CLI) promptForCards(cfg *config.GameConfig, requireAtLeastOne bool, exactCount int) []string {
	var cards []string
	cardSet := make(map[string]struct{})
	C.Header.Println("\n--- Card List ---")
	for i, card := range cfg.AllCards {
		// fmt.Printf("%2d: %-18s", i+1, ColorizeCard(card))
		fmt.Printf("%2d: %-18s", i+1, card)
		if (i+1)%3 == 0 {
			fmt.Println()
		}
	}
	fmt.Println()

	for {
		if exactCount > 0 && len(cards) == exactCount {
			break
		}
		prompt := "Enter card name/number"
		if exactCount > 0 {
			prompt = fmt.Sprintf("Enter card %d of %d", len(cards)+1, exactCount)
		} else {
			prompt += " (or 'done')"
		}
		input := c.promptForString(prompt + ": ")
		if exactCount == 0 && strings.ToLower(input) == "done" {
			if requireAtLeastOne && len(cards) == 0 {
				C.Warn.Println("Please enter at least one card.")
				continue
			}
			break
		}
		var foundCard string
		if num, err := strconv.Atoi(input); err == nil && num >= 1 && num <= len(cfg.AllCards) {
			foundCard = cfg.AllCards[num-1]
		} else {
			for _, card := range cfg.AllCards {
				if strings.EqualFold(card, input) {
					foundCard = card
					break
				}
			}
		}
		if foundCard == "" {
			C.Warn.Printf("Error: Card '%s' not found.\n", foundCard)
		} else if _, exists := cardSet[foundCard]; exists {
			C.Warn.Printf("You have already entered '%s'.\n", foundCard)
		} else {
			cards = append(cards, foundCard)
			cardSet[foundCard] = struct{}{}
			C.Info.Printf(" -> Added: %s\n", ColorizeCard(foundCard))
		}
	}
	return cards
}
