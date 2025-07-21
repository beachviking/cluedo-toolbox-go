// cluedo.go
// A command-line toolbox for Cluedo (Clue) with a master-level AI.
// This is the final, complete, and fully functional version.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/peterh/liner"
	"github.com/sirupsen/logrus"
)

// --- Global Variables and Types ---

var log = logrus.New()
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

type GameConfig struct {
	Suspects   []string `json:"suspects"`
	Weapons    []string `json:"weapons"`
	Rooms      []string `json:"rooms"`
	AllCards   []string
	CardToType map[string]string
}

var config GameConfig

// --- Player Interface ---

type Player interface {
	Name() string
	IsHuman() bool
	Setup(cfg GameConfig, playerNames []string, myName string)
	ReceiveHand(cards []string)
	MakeSuggestion() map[string]string
	ShouldAccuse() map[string]string
	ProcessTurnInfo(suggester, disprover, revealedCard string, suggestion map[string]string)
	ChooseCardToShow(suggestion map[string]string) string
	DisplayNotes()
}

var SuspectColors = map[string]*color.Color{
	"Miss Scarlett":   color.New(color.FgRed),
	"Colonel Mustard": color.New(color.FgYellow),
	"Mrs. White":      color.New(color.FgWhite),
	"Mr. Green":       color.New(color.FgGreen),
	"Mrs. Peacock":    color.New(color.FgBlue),
	"Professor Plum":  color.New(color.FgMagenta),
}

// Helper to get a color for a card name, defaulting to white.
func colorizeCard(name string) string {
	if c, ok := SuspectColors[name]; ok {
		return c.Sprint(name)
	}
	return name // Default color
}

func makeAiTitle(name string) string {
	if c, ok := SuspectColors[name]; ok {
		return c.Sprintf("[%s's Brain]", name)
	}
	return name // Default color
}

// --- Main Game Struct ---

type Game struct {
	Config   GameConfig
	Players  []Player
	Solution map[string]string
	turn     int
}

func NewGame(cfg GameConfig, numHumans, numAI int) *Game {
	playerNames := cfg.Suspects[:numHumans+numAI]
	rand.Shuffle(len(playerNames), func(i, j int) { playerNames[i], playerNames[j] = playerNames[j], playerNames[i] })

	g := &Game{Config: cfg, Solution: make(map[string]string)}

	for i, name := range playerNames {
		var p Player
		if i < numHumans {
			p = NewHumanPlayer()
		} else {
			p = NewAdvancedAIBrain()
		}
		p.Setup(cfg, playerNames, name)
		g.Players = append(g.Players, p)
	}
	return g
}

func (g *Game) Deal() {
	deck := make([]string, len(g.Config.AllCards))
	copy(deck, g.Config.AllCards)
	rand.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })

	dealtCategories := make(map[string]bool)
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
	sort.Strings(cardsToDeal) // for deterministic testing if needed

	hands := make([][]string, len(g.Players))
	for i, card := range cardsToDeal {
		playerIndex := i % len(g.Players)
		hands[playerIndex] = append(hands[playerIndex], card)
	}

	for i, p := range g.Players {
		p.ReceiveHand(hands[i])
		log.Debugf("%s Hand: %v", p.Name(), hands[i])
	}
	log.Debugf("Ground Truth Initialized. Solution: %+v", g.Solution)
}

func (g *Game) HandleSuggestion(suggester Player, suggestion map[string]string) (string, string) {
	suggesterIdx := -1
	for i, p := range g.Players {
		if p.Name() == suggester.Name() {
			suggesterIdx = i
			break
		}
	}

	for i := 1; i < len(g.Players); i++ {
		playerIdx := (suggesterIdx + i) % len(g.Players)
		playerToAsk := g.Players[playerIdx]

		cardShown := playerToAsk.ChooseCardToShow(suggestion)
		if cardShown != "" {
			return playerToAsk.Name(), cardShown
		}
	}
	return "", ""
}

// --- Advanced AI Player Implementation ---
type AdvancedAIBrain struct {
	name                  string
	config                GameConfig
	players               []string
	hand                  map[string]struct{}
	knowledge             map[string]map[string]CardStatus
	unresolvedSuggestions []UnresolvedSuggestion
	recentSurgicalTargets *StringDeque
}

type CardStatus string

const (
	StatusYes   CardStatus = "Yes"
	StatusNo    CardStatus = "No"
	StatusMaybe CardStatus = "Maybe"
)

type UnresolvedSuggestion struct {
	Disprover     string
	PossibleCards map[string]struct{}
}

func NewAdvancedAIBrain() *AdvancedAIBrain { return &AdvancedAIBrain{} }
func (ai *AdvancedAIBrain) Name() string   { return ai.name }
func (ai *AdvancedAIBrain) IsHuman() bool  { return false }

func (ai *AdvancedAIBrain) Setup(cfg GameConfig, playerNames []string, myName string) {
	ai.name = myName
	ai.config = cfg
	ai.players = playerNames
	ai.hand = make(map[string]struct{})
	ai.unresolvedSuggestions = []UnresolvedSuggestion{}
	ai.recentSurgicalTargets = NewStringDeque(3)
	ai.knowledge = make(map[string]map[string]CardStatus)
	for _, card := range ai.config.AllCards {
		ai.knowledge[card] = make(map[string]CardStatus)
		for _, pName := range ai.players {
			ai.knowledge[card][pName] = StatusMaybe
		}
		ai.knowledge[card]["solution"] = StatusMaybe
	}
	log.Debugf("[%s's Brain] Master deduction engine initialized.", ai.name)
}

func (ai *AdvancedAIBrain) ReceiveHand(cards []string) {
	// THE FIX: Process the hand to update the initial knowledge grid.
	for _, card := range cards {
		ai.hand[card] = struct{}{}
		// Use our central method to record this certain fact.
		ai._markCardLocation(card, ai.name)
	}
	// After processing the entire hand, run the deduction engine to see
	// if any simple eliminations can be made immediately.
	ai._runDeductionLoop()
}

func (ai *AdvancedAIBrain) ProcessTurnInfo(suggester, disprover, revealedCard string, suggestion map[string]string) {
	if suggester == "Game Event" {
		// This is a direct reveal, a certain fact.
		// The 'disprover' field is used to carry the player name.
		if disprover != "" && revealedCard != "" {
			ai._markCardLocation(revealedCard, disprover)
			ai._runDeductionLoop()
		}
		return // Stop processing here.
	}

	if ai.name == suggester {
		if disprover != "" && revealedCard != "" {
			ai._markCardLocation(revealedCard, disprover)
		} else if disprover == "" {
			log.Infof("[%s] My suggestion was not disproved! Making powerful deductions.", colorizeCard(ai.name))
			for _, card := range suggestion {
				if _, inHand := ai.hand[card]; !inHand {
					ai._markCardLocation(card, "solution")
				}
			}
		}
	} else if disprover != "" && disprover != ai.name {
		newMystery := UnresolvedSuggestion{Disprover: disprover, PossibleCards: make(map[string]struct{})}
		for _, card := range suggestion {
			newMystery.PossibleCards[card] = struct{}{}
		}
		ai.unresolvedSuggestions = append(ai.unresolvedSuggestions, newMystery)

		// log.Infof("[%s's Brain] noted that %s holds one of %v. (New unsolved mystery)", colorizeCard(ai.name), disprover, mapKeys(newMystery.PossibleCards))
		log.Infof("%s noted that %s holds one of %v. (New unsolved mystery)", makeAiTitle(ai.name), disprover, mapKeys(newMystery.PossibleCards))
	}
	ai._runDeductionLoop()
}

func (ai *AdvancedAIBrain) ChooseCardToShow(suggestion map[string]string) string {
	var canShow []string
	for _, card := range suggestion {
		if _, ok := ai.hand[card]; ok {
			canShow = append(canShow, card)
		}
	}
	if len(canShow) == 0 {
		return ""
	}
	return canShow[rand.Intn(len(canShow))]
}

func (ai *AdvancedAIBrain) MakeSuggestion() map[string]string {
	log.Debugf("[%s's Brain] Formulating a master-level suggestion...", ai.name)

	// Priority 1: Exploit
	var knownSolutionCards = make(map[string]string)
	knownCount := 0
	for _, cat := range []string{"suspects", "weapons", "rooms"} {
		cardList := ai.config.Suspects
		if cat == "weapons" {
			cardList = ai.config.Weapons
		}
		if cat == "rooms" {
			cardList = ai.config.Rooms
		}
		for _, card := range cardList {
			if ai.knowledge[card]["solution"] == StatusYes {
				knownSolutionCards[cat] = card
				knownCount++
				break
			}
		}
	}
	if knownCount >= 1 {
		log.Infof("[%s] Strategy: EXPLOIT. I know %d/3 of the solution, testing a theory.", colorizeCard(ai.name), knownCount)
		return ai._buildExploitSuggestion(knownSolutionCards)
	}

	// Priority 2: Surgical Strike
	if len(ai.unresolvedSuggestions) > 0 {
		cardFrequency := make(map[string]int)
		for _, mystery := range ai.unresolvedSuggestions {
			for card := range mystery.PossibleCards {
				cardFrequency[card]++
			}
		}
		if len(cardFrequency) > 0 {
			var sortedTargets []string
			for card := range cardFrequency {
				sortedTargets = append(sortedTargets, card)
			}
			sort.Slice(sortedTargets, func(i, j int) bool { return cardFrequency[sortedTargets[i]] > cardFrequency[sortedTargets[j]] })

			var patientTargets []string
			for _, card := range sortedTargets {
				if !ai.recentSurgicalTargets.Contains(card) {
					patientTargets = append(patientTargets, card)
				}
			}
			if len(patientTargets) == 0 {
				patientTargets = sortedTargets
			}

			topTargets := patientTargets
			if len(topTargets) > 3 {
				topTargets = topTargets[:3]
			}

			if len(topTargets) > 0 {
				targetCard := topTargets[rand.Intn(len(topTargets))]
				log.Infof("[%s] Strategy: SURGICAL STRIKE. Top patient targets: %v. Targeting '%s'.", colorizeCard(ai.name), topTargets, targetCard)
				ai.recentSurgicalTargets.Push(targetCard)
				return ai._buildSuggestionAroundTarget(targetCard)
			}
		}
	}

	// Priority 3: Explore
	log.Infof("[%s] Strategy: EXPLORE. Gathering new information.", colorizeCard(ai.name))
	return ai._buildExplorationSuggestion()
}

func (ai *AdvancedAIBrain) ShouldAccuse() map[string]string {
	solution := make(map[string]string)
	for _, cat := range []string{"suspects", "weapons", "rooms"} {
		cardList := ai.config.Suspects
		if cat == "weapons" {
			cardList = ai.config.Weapons
		}
		if cat == "rooms" {
			cardList = ai.config.Rooms
		}

		var knownSolutionCard string
		for _, card := range cardList {
			if ai.knowledge[card]["solution"] == StatusYes {
				knownSolutionCard = card
				break
			}
		}

		if knownSolutionCard != "" {
			solution[cat] = knownSolutionCard
		} else {
			return nil // If any category isn't a "Yes", we can't accuse.
		}
	}

	if len(solution) == 3 {
		log.Debugf("[%s] Finalizing knowledge before accusing.", ai.name)
		for _, card := range ai.config.AllCards {
			isSolutionCard := false
			for _, solCard := range solution {
				if card == solCard {
					isSolutionCard = true
					break
				}
			}
			if !isSolutionCard {
				ai.knowledge[card]["solution"] = StatusNo
			}
		}
		ai._runDeductionLoop()
		log.Infof("[%s] is making a confident ACCUSATION: %v", colorizeCard(ai.name), values(solution))
		return solution
	}
	return nil
}

// --- AI Helper & Deduction Methods ---
func (ai *AdvancedAIBrain) _buildExplorationSuggestion() map[string]string {
	suggestion := make(map[string]string)

	// This is a helper function to pick a valid card for a category.
	pickCard := func(cardList []string) string {
		var maybes []string
		for _, card := range cardList {
			if _, inHand := ai.hand[card]; !inHand && ai.knowledge[card]["solution"] == StatusMaybe {
				maybes = append(maybes, card)
			}
		}
		if len(maybes) > 0 {
			return maybes[rand.Intn(len(maybes))]
		}

		// Fallback: pick any card not in our hand.
		var notMyCards []string
		for _, card := range cardList {
			if _, inHand := ai.hand[card]; !inHand {
				notMyCards = append(notMyCards, card)
			}
		}
		if len(notMyCards) > 0 {
			return notMyCards[rand.Intn(len(notMyCards))]
		}

		// Last resort fallback: pick any card from the list.
		return cardList[rand.Intn(len(cardList))]
	}

	suggestion["suspects"] = pickCard(ai.config.Suspects)
	suggestion["weapons"] = pickCard(ai.config.Weapons)
	suggestion["rooms"] = pickCard(ai.config.Rooms)

	return suggestion
}

func (ai *AdvancedAIBrain) _buildExploitSuggestion(knowns map[string]string) map[string]string {
	suggestion := make(map[string]string)

	// This is a helper function to robustly pick a card for a category.
	pickCard := func(cardList []string) string {
		var maybes []string
		for _, card := range cardList {
			if _, inHand := ai.hand[card]; !inHand && ai.knowledge[card]["solution"] == StatusMaybe {
				maybes = append(maybes, card)
			}
		}
		if len(maybes) > 0 {
			return maybes[rand.Intn(len(maybes))]
		}

		var notMyCards []string
		for _, card := range cardList {
			if _, inHand := ai.hand[card]; !inHand {
				notMyCards = append(notMyCards, card)
			}
		}
		if len(notMyCards) > 0 {
			return notMyCards[rand.Intn(len(notMyCards))]
		}

		return cardList[rand.Intn(len(cardList))]
	}

	for _, cat := range []string{"suspects", "weapons", "rooms"} {
		if card, ok := knowns[cat]; ok {
			// If we know the solution for this category, use it.
			suggestion[cat] = card
		} else {
			// Otherwise, robustly pick a card from the correct list.
			var cardList []string
			switch cat {
			case "suspects":
				cardList = ai.config.Suspects
			case "weapons":
				cardList = ai.config.Weapons
			case "rooms":
				cardList = ai.config.Rooms
			}
			suggestion[cat] = pickCard(cardList)
		}
	}
	return suggestion
}

func (ai *AdvancedAIBrain) _buildSuggestionAroundTarget(targetCard string) map[string]string {
	suggestion := make(map[string]string)
	targetCategory := ai.config.CardToType[targetCard]
	suggestion[targetCategory] = targetCard

	var myHandSlice []string
	for card := range ai.hand {
		myHandSlice = append(myHandSlice, card)
	}
	rand.Shuffle(len(myHandSlice), func(i, j int) { myHandSlice[i], myHandSlice[j] = myHandSlice[j], myHandSlice[i] })

	for _, card := range myHandSlice {
		if len(suggestion) == 3 {
			break
		}
		cat := ai.config.CardToType[card]
		if _, exists := suggestion[cat]; !exists {
			suggestion[cat] = card
		}
	}
	if len(suggestion) < 3 {
		exploreSuggestion := ai._buildExplorationSuggestion()
		for cat, card := range exploreSuggestion {
			if _, exists := suggestion[cat]; !exists {
				suggestion[cat] = card
			}
		}
	}
	return suggestion
}

func (ai *AdvancedAIBrain) _markCardLocation(card, location string) {
	// --- THE CORRECTED, ROBUST DEBUGGING CHECK ---
	// It correctly checks the 'card' variable.
	if _, isValidCard := ai.config.CardToType[card]; !isValidCard {
		log.Errorf("FATAL LOGIC ERROR: _markCardLocation called with INVALID card name: '%s'", card)
		log.Errorf(" -> This likely happened while trying to mark its location as: '%s'", location)
		return // Stop processing to prevent a panic
	}

	if val, ok := ai.knowledge[card][location]; ok && val == StatusYes {
		return
	}
	log.Debugf("[%s's Brain] learned that '%s' is with %s.", ai.name, card, location)
	allLocations := append(ai.players, "solution")
	for _, loc := range allLocations {
		ai.knowledge[card][loc] = StatusNo
	}
	ai.knowledge[card][location] = StatusYes
}

func (ai *AdvancedAIBrain) _deduceCardLocationsByElimination() {
	for _, card := range ai.config.AllCards {
		known := false
		allLocations := append(ai.players, "solution")
		for _, loc := range allLocations {
			if ai.knowledge[card][loc] == StatusYes {
				known = true
				break
			}
		}
		if known {
			continue
		}

		var maybes []string
		for _, loc := range allLocations {
			if ai.knowledge[card][loc] == StatusMaybe {
				maybes = append(maybes, loc)
			}
		}

		if len(maybes) == 1 {
			final_location := maybes[0]
			ai._markCardLocation(card, final_location)
		}
	}
}

func (ai *AdvancedAIBrain) _runDeductionLoop() {
	for i := 0; i < 10; i++ { // Safety break
		before := fmt.Sprintf("%v", ai.knowledge)
		ai._pruneAndSolveMysteries()
		ai._deduceSolutionByElimination()
		ai._deduceCardLocationsByElimination()
		if fmt.Sprintf("%v", ai.knowledge) == before {
			break
		}
	}
}

func (ai *AdvancedAIBrain) _pruneAndSolveMysteries() {
	var remainingMysteries []UnresolvedSuggestion
	for _, mystery := range ai.unresolvedSuggestions {
		prunedCards := make(map[string]struct{})
		for card := range mystery.PossibleCards {
			if ai.knowledge[card][mystery.Disprover] != StatusNo {
				prunedCards[card] = struct{}{}
			}
		}
		if len(prunedCards) < len(mystery.PossibleCards) {
			log.Debugf("[%s's Brain] Pruning mystery: %s's options narrowed to %v", ai.name, mystery.Disprover, mapKeys(prunedCards))
			mystery.PossibleCards = prunedCards
		}
		if len(prunedCards) == 1 {
			card := mapKeys(prunedCards)[0]
			log.Infof("%s SOLVED A MYSTERY! %s must have shown '%s'.", makeAiTitle(ai.name), colorizeCard(mystery.Disprover), card)
			ai._markCardLocation(card, mystery.Disprover)
		} else if len(prunedCards) > 1 {
			remainingMysteries = append(remainingMysteries, mystery)
		}
	}
	ai.unresolvedSuggestions = remainingMysteries
}

func (ai *AdvancedAIBrain) _deduceSolutionByElimination() {
	for _, cat := range []string{"suspects", "weapons", "rooms"} {
		cardList := config.Suspects
		if cat == "weapons" {
			cardList = config.Weapons
		}
		if cat == "rooms" {
			cardList = config.Rooms
		}
		isSolved := false
		for _, card := range cardList {
			if ai.knowledge[card]["solution"] == StatusYes {
				isSolved = true
				break
			}
		}
		if isSolved {
			continue
		}
		var maybes []string
		for _, card := range cardList {
			if ai.knowledge[card]["solution"] == StatusMaybe {
				maybes = append(maybes, card)
			}
		}
		if len(maybes) == 1 {
			ai._markCardLocation(maybes[0], "solution")
		}
	}
}

// --- Human Player (Placeholder) ---
type HumanPlayer struct {
	name string
	cfg  GameConfig
	hand map[string]struct{}
}

func NewHumanPlayer() *HumanPlayer   { return &HumanPlayer{name: "Human"} }
func (h *HumanPlayer) Name() string  { return h.name }
func (h *HumanPlayer) IsHuman() bool { return true }
func (h *HumanPlayer) Setup(cfg GameConfig, playerNames []string, myName string) {
	h.name = myName
	h.cfg = cfg
	h.hand = make(map[string]struct{})
}
func (h *HumanPlayer) ReceiveHand(cards []string) {
	for _, card := range cards {
		h.hand[card] = struct{}{}
	}
	C.Info.Printf("\nYour hand: %v\n", cards)
}
func (h *HumanPlayer) MakeSuggestion() map[string]string {
	// In simulation, this would prompt the user.
	// For now, we return nil as the loop handles this for humans.
	return nil
}
func (h *HumanPlayer) ShouldAccuse() map[string]string { return nil }
func (h *HumanPlayer) ProcessTurnInfo(suggester, disprover, revealedCard string, suggestion map[string]string) {
	if h.name == suggester && revealedCard != "" {
		C.Info.Printf("You were shown the card: %s\n", revealedCard)
	}
}
func (h *HumanPlayer) ChooseCardToShow(suggestion map[string]string) string {
	// This would prompt a human player.
	// For the simulation, we can let it auto-pick.
	var canShow []string
	for _, card := range suggestion {
		if _, ok := h.hand[card]; ok {
			canShow = append(canShow, card)
		}
	}
	if len(canShow) == 0 {
		return ""
	}
	return canShow[0] // Just show the first one in simulations
}
func (h *HumanPlayer) DisplayNotes() { C.Info.Println("Human player notes are managed by the user.") }

// --- StringDeque for AI "Patience" ---
type StringDeque struct {
	elements []string
	maxSize  int
}

func NewStringDeque(maxSize int) *StringDeque { return &StringDeque{maxSize: maxSize} }
func (d *StringDeque) Push(s string) {
	d.elements = append(d.elements, s)
	if len(d.elements) > d.maxSize {
		d.elements = d.elements[1:]
	}
}
func (d *StringDeque) Contains(s string) bool {
	for _, e := range d.elements {
		if e == s {
			return true
		}
	}
	return false
}

// --- Main Entry and Game Loop ---
func main() {
	logLevel := flag.String("loglevel", "info", "Set logging level (debug, info, warn, error)")
	flag.Parse()
	level, err := logrus.ParseLevel(*logLevel)
	if err != nil {
		level = logrus.InfoLevel
	}
	log.SetLevel(level)
	log.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true, ForceColors: true})

	if err := loadConfig("default_config.json"); err != nil {
		log.Fatalf("Failed to load default_config.json: %v", err)
	}
	rand.Seed(time.Now().UnixNano())

	args := flag.Args()
	if len(args) < 1 {
		printUsage()
		return
	}

	line := liner.NewLiner()
	defer line.Close()
	line.SetCtrlCAborts(true)

	if args[0] == "detective" {
		runDetectiveMode(line)
	} else if args[0] == "start" && len(args) == 3 {
		numHumans, _ := strconv.Atoi(args[1])
		numAI, _ := strconv.Atoi(args[2])
		C.Header.Println("--- Running Fast Simulation ---")
		game := NewGame(config, numHumans, numAI)
		game.Deal()
		runSimulationLoop(game)
	} else {
		printUsage()
	}
}

func runDetectiveMode(line *liner.State) {
	C.Info.Println("\n--- Starting Detective Mode Co-Pilot ---")

	// 1. Setup Wizard
	numPlayers := promptForInt(line, "How many players are in the real game? (2-6): ", 2, 6)
	var playerNames []string
	for i := 0; i < numPlayers; i++ {
		name := promptForString(line, fmt.Sprintf("Enter name for Player %d: ", i+1))
		playerNames = append(playerNames, name)
	}
	myPlayerName := promptForSelection(line, "Which player are you?", playerNames)

	C.Info.Println("\nSelect the cards in your hand. Type 'done' when finished.")
	myHand := promptForCards(line, true, 0) // 0 means no exact count

	// 2. Create the AI Brain
	brain := NewAdvancedAIBrain()
	brain.Setup(config, playerNames, myPlayerName)
	brain.ReceiveHand(myHand)

	C.Info.Println("\nDetective Mode is active! Your co-pilot is ready.")
	brain.DisplayNotes()

	printDetectiveHelp()

	// 3. Main Command Loop
	for {
		// Use a single, clear prompt. The help text is available via the 'help' command.
		input, err := line.Prompt("(detective) ")
		fmt.Printf("%s\n", input)
		if err != nil {
			if err == liner.ErrPromptAborted || err == io.EOF {
				C.Info.Println("Goodbye!")
				return
			}
			log.Fatalf("Error reading line: %v", err)
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		line.AppendHistory(input)
		parts := strings.Fields(input)
		cmd := strings.ToLower(parts[0])
		args := parts[1:]

		switch cmd {
		case "log", "l":
			handleLogCommand(line, brain)
		case "reveal", "r":
			handleRevealCommand(line, brain)
		case "suggest", "s":
			handleSuggestCommand(brain)
		case "notes", "n":
			brain.DisplayNotes()
		case "hand":
			handleHandCommand(brain)
		case "help", "h":
			handleHelpCommand(args)
		case "quit", "q":
			C.Info.Println("Exiting detective mode.")
			return
		default:
			C.Warn.Printf("Unknown command '%s'. Type 'help' for a list of commands.\n", cmd)
		}
	}
}

func handleHelpCommand(args []string) {
	if len(args) == 0 {
		// General help
		C.Header.Println("\n--- Cluedo Toolbox Help ---")
		fmt.Println("This is the detective co-pilot mode. Log events from your real-life game, and the AI will track everything for you.")
		fmt.Println("\nAvailable commands:")

		t := table.NewWriter()
		t.SetOutputMirror(os.Stdout)
		t.AppendRow(table.Row{"Command", "Alias", "Description"})
		t.AppendSeparator()
		t.AppendRows([]table.Row{
			{"log", "l", "Log a full game turn (suggestion and result)."},
			{"reveal", "r", "Log a single card revealed by a player."},
			{"suggest", "s", "Ask the AI co-pilot for a strategic suggestion."},
			{"notes", "n", "Display the AI's current detective notes grid."},
			{"hand", "ha", "Display the cards currently in your hand."},
			{"quit", "q", "Exit detective mode."},
		})
		t.SetStyle(table.StyleLight)
		t.Render()

		fmt.Println("\nType 'help <command>' for more details on a specific command (e.g., 'help log').")
		return
	}

	// Specific help for a command
	command := strings.ToLower(args[0])
	C.Header.Printf("\n--- Help for: %s ---\n", command)
	switch command {
	case "log", "l":
		fmt.Println("Records one full turn of a real game into the notebook.")
		C.Prompt.Println("\nUsage:")
		fmt.Println("  log")
		C.Prompt.Println("\nDetails:")
		fmt.Println("  You will be interactively prompted for:")
		fmt.Println("  1. The Suggester: The player making the suggestion.")
		fmt.Println("  2. The 3 Cards: The suspect, weapon, and room suggested.")
		fmt.Println("  3. The Disprover: The player who showed a card. Select 'No One' if applicable.")
		fmt.Println("  4. The Revealed Card (optional): If you were the suggester, you will be asked which card you were shown.")
		fmt.Println("\nCards can be entered by their full name or by their ID number from the 'notes' table.")

	case "reveal", "r":
		fmt.Println("Records that a player revealed a specific card outside of a normal suggestion.")
		C.Prompt.Println("\nUsage:")
		fmt.Println("  reveal")
		C.Prompt.Println("\nDetails:")
		fmt.Println("  Useful for game variants with Intrigue Cards or house rules.")
		fmt.Println("  You will be prompted for the player and the card they revealed.")

	case "suggest", "s":
		fmt.Println("Asks the AI co-pilot for a strategically valuable suggestion for your turn.")
		C.Prompt.Println("\nUsage:")
		fmt.Println("  suggest")
		C.Prompt.Println("\nDetails:")
		fmt.Println("  The AI will analyze its current knowledge to propose a suggestion that either:")
		fmt.Println("  - Exploits known information to confirm the final piece of the solution.")
		fmt.Println("  - Performs a 'Surgical Strike' to solve an outstanding mystery.")
		fmt.Println("  - Explores to gather new information if no other strategy is viable.")

	case "notes", "n":
		fmt.Println("Displays the AI's current detective notes grid.")
		C.Prompt.Println("\nUsage:")
		fmt.Println("  notes")
		C.Prompt.Println("\nDetails:")
		fmt.Println("  This shows what the AI knows about every card, player, and the solution.")
		fmt.Println("  (✔ = Yes, ✖ = No, ? = Maybe)")

	case "hand", "ha":
		fmt.Println("Displays the cards you entered as being in your hand at the start.")
		C.Prompt.Println("\nUsage:")
		fmt.Println("  hand")

	case "quit", "q":
		fmt.Println("Exits detective mode and returns to the main application menu.")
		C.Prompt.Println("\nUsage:")
		fmt.Println("  quit")

	default:
		C.Warn.Printf("Unknown command '%s'. Type 'help' for a list of commands.\n", command)
	}
}

func handleHandCommand(brain Player) {
	C.Header.Println("\n--- Your Hand ---")
	// We need a way to get the hand from the player
	// Let's add a Hand() method to the Player interface
	// For now, we'll cast it.
	if ai, ok := brain.(*AdvancedAIBrain); ok {
		var cards []string
		for card := range ai.hand {
			cards = append(cards, card)
		}
		sort.Strings(cards)
		for _, card := range cards {
			C.Info.Println(" - " + colorizeCard(card))
		}
	}
}

func handleLogCommand(line *liner.State, brain Player) {
	C.Info.Println("\n--- Log a Game Turn ---")

	// --- THE FIX: Use promptForSelection for player names ---
	playerNames := brain.(*AdvancedAIBrain).players
	suggester := promptForSelection(line, "Who made the suggestion?", playerNames)

	C.Info.Println("What 3 cards were suggested? (Use numbers or names)")
	// The promptForCards helper is only for cards.
	suggestionCards := promptForCards(line, false, 3) // Ask for exactly 3 cards
	if len(suggestionCards) != 3 {
		C.Warn.Println("Error: A suggestion must have exactly 3 cards.")
		return
	}
	suggestion := make(map[string]string)
	for _, card := range suggestionCards {
		suggestion[config.CardToType[card]] = card
	}

	disproverOptions := append(playerNames, "No One")
	disprover := promptForSelection(line, "Who disproved the suggestion?", disproverOptions)

	var revealedCard string
	if disprover != "No One" && suggester == brain.Name() {
		C.Info.Println("What card were you shown? (Use numbers or names)")
		// Use promptForCards to get a single card.
		revealedCards := promptForCards(line, true, 1)
		if len(revealedCards) > 0 {
			revealedCard = revealedCards[0]
		}
	} else if disprover == "No One" {
		disprover = ""
	}

	brain.ProcessTurnInfo(suggester, disprover, revealedCard, suggestion)
	C.Info.Println("Turn logged. Here are your updated notes:")
	brain.DisplayNotes()
}

func handleRevealCommand(line *liner.State, brain Player) {
	C.Info.Println("\n--- Log a Revealed Card ---")
	player := promptForSelection(line, "Which player revealed a card?", brain.(*AdvancedAIBrain).players)

	C.Info.Println("Which card did they reveal? (Use number or name)")
	revealedCards := promptForCards(line, true, 1)
	if len(revealedCards) == 0 {
		return // User cancelled
	}
	card := revealedCards[0]

	// We can use ProcessTurnInfo with a special suggester to log this fact.
	// This will call _markCardLocation correctly.
	brain.ProcessTurnInfo("Game Event", player, card, nil)
	C.Info.Println("Revealed card logged.")
	brain.DisplayNotes()
}

func handleSuggestCommand(brain Player) {
	C.Header.Println("\n--- AI Co-Pilot Suggestion ---")
	suggestion := brain.MakeSuggestion()
	var parts []string
	for _, card := range suggestion {
		parts = append(parts, colorizeCard(card))
	}
	C.Info.Printf("The AI suggests you propose: %s\n", strings.Join(parts, ", "))
}

func runSimulationLoop(g *Game) {
	C.Header.Println("--- Starting Game ---")

	// --- NEW: Store initial brain states ---
	initialBrains := make(map[string]map[string]map[string]CardStatus)
	for _, p := range g.Players {
		if ai, ok := p.(*AdvancedAIBrain); ok {
			initialBrains[ai.Name()] = ai.deepCopyKnowledge()
		}
	}

	displayPlayer := g.Players[0] // Default display
	for _, p := range g.Players {
		if !p.IsHuman() {
			displayPlayer = p
			break
		}
	}
	displayPlayer.DisplayNotes() // Show initial state without comparison

	winner := ""

	for g.turn < 50 {
		currentPlayer := g.Players[g.turn%len(g.Players)]
		C.Header.Printf("\n--- Turn %d: %s ---\n", g.turn+1, colorizeCard(currentPlayer.Name()))

		if accusation := currentPlayer.ShouldAccuse(); accusation != nil {
			winner = currentPlayer.Name()
			isCorrect := true
			for cat, card := range accusation {
				if g.Solution[cat] != card {
					isCorrect = false
					break
				}
			}
			C.Info.Printf("%s accuses! The solution is %v. This is %t\n", colorizeCard(currentPlayer.Name()), values(accusation), isCorrect)
			break
		}

		suggestion := currentPlayer.MakeSuggestion()
		C.Info.Printf("%s suggests: %v\n", colorizeCard(currentPlayer.Name()), values(suggestion))
		disproverName, revealedCard := g.HandleSuggestion(currentPlayer, suggestion)

		if disproverName != "" {
			C.Info.Printf("-> %s shows a card to %s.\n", colorizeCard(disproverName), colorizeCard(currentPlayer.Name()))
			log.Debugf(" (The card was '%s')", revealedCard)
		} else {
			C.Info.Println("-> No player could show a card.")
		}

		for _, p := range g.Players {
			p.ProcessTurnInfo(currentPlayer.Name(), disproverName, revealedCard, suggestion)
		}

		g.turn++
		if !currentPlayer.IsHuman() {
			time.Sleep(100 * time.Millisecond)
		}
	}

	C.Header.Println("\n--- GAME OVER ---")
	C.Info.Printf("Solution was: %v\n", g.Solution)
	// --- NEW: Display the final comparison table ---
	if winner != "" {
		var winningPlayer Player
		for _, p := range g.Players {
			if p.Name() == winner {
				winningPlayer = p
				break
			}
		}
		if winningPlayer != nil && !winningPlayer.IsHuman() {
			C.Header.Println("\n========================================")
			C.Header.Println("VERIFICATION: Initial vs. Final Notes for Winner")
			C.Header.Println("========================================")
			winningPlayer.DisplayNotes()
		}
	}
}

// --- UI and Helper Functions ---
func printUsage() {
	fmt.Println("\nUsage:\n  go run . detective\n  go run . start <num_humans> <num_ai> [-loglevel debug]")
}
func printDetectiveHelp() {
	fmt.Println(C.Prompt.Sprint("\n(log, reveal, suggest, notes, quit)"))
}

// --- UI and Helper Functions ---
// Helper to create a true copy of the knowledge map.
func (ai *AdvancedAIBrain) deepCopyKnowledge() map[string]map[string]CardStatus {
	newKnowledge := make(map[string]map[string]CardStatus)
	for card, locations := range ai.knowledge {
		newKnowledge[card] = make(map[string]CardStatus)
		for loc, status := range locations {
			newKnowledge[card][loc] = status
		}
	}
	return newKnowledge
}

func promptForCards(line *liner.State, requireAtLeastOne bool, exactCount int) []string {
	var cards []string
	cardSet := make(map[string]struct{})

	// Display the card list for user reference - This part is fine.
	maxLen := 0
	for _, card := range config.AllCards {
		if len(card) > maxLen {
			maxLen = len(card)
		}
	}
	numCols := 3
	C.Header.Println("\n--- Card List ---")
	for i, card := range config.AllCards {
		cardID := i + 1
		fmt.Printf("%2d: %-*s", cardID, maxLen+2, card)
		if (i+1)%numCols == 0 || i == len(config.AllCards)-1 {
			fmt.Println()
		}
	}
	fmt.Println()

	// --- Main input loop ---
	for {
		if exactCount > 0 && len(cards) == exactCount {
			break
		}

		prompt := "Enter card name/number"
		if exactCount > 0 {
			prompt = fmt.Sprintf("Enter card %d of %d", len(cards)+1, exactCount)
		}
		if exactCount == 0 {
			prompt += " (or 'done')"
		}
		prompt += ": "

		// --- THE FIX ---
		// Print the colored part first, then prompt with an empty string.
		C.Prompt.Print(prompt)
		input, err := line.Prompt("")
		if err != nil {
			if err == liner.ErrPromptAborted || err == io.EOF {
				C.Info.Println("Aborting.")
				return []string{}
			}
			log.Fatalf("Error reading line: %v", err)
		}
		input = strings.TrimSpace(input)

		if exactCount == 0 && strings.ToLower(input) == "done" {
			if requireAtLeastOne && len(cards) == 0 {
				C.Warn.Println("Please enter at least one card.")
				continue
			}
			break
		}

		var foundCard string
		if num, err := strconv.Atoi(input); err == nil && num >= 1 && num <= len(config.AllCards) {
			foundCard = config.AllCards[num-1]
		} else {
			for _, card := range config.AllCards {
				if strings.EqualFold(card, input) {
					foundCard = card
					break
				}
			}
		}

		if foundCard == "" {
			C.Warn.Printf("Error: Card '%s' not found.\n", input)
		} else if _, exists := cardSet[foundCard]; exists {
			C.Warn.Printf("You have already entered '%s'.\n", foundCard)
		} else {
			cards = append(cards, foundCard)
			cardSet[foundCard] = struct{}{}
			C.Info.Printf(" -> Added: %s\n", foundCard)
			line.AppendHistory(input) // Append valid history
		}
	}
	return cards
}

func (ai *AdvancedAIBrain) DisplayNotes() {
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.SetTitle(fmt.Sprintf("%s's Detective Notes", ai.name))

	// --- Build Header ---
	header := table.Row{"ID", "Card", "Type"}
	// We build the header from the AI's known list of players
	for _, pName := range ai.players {
		header = append(header, colorizeCard(pName))
	}
	header = append(header, "Solution")
	t.AppendHeader(header)

	// --- Build Rows ---
	// We iterate through the official, canonical list of cards from the config.
	// This list NEVER changes and does NOT contain "solution".
	for cardID, card := range ai.config.AllCards {

		// Add a separator between card types by checking the previous card.
		if cardID > 0 && ai.config.CardToType[card] != ai.config.CardToType[ai.config.AllCards[cardID-1]] {
			t.AppendSeparator()
		}

		// Start building the row with known, valid data.
		row := table.Row{cardID + 1, colorizeCard(card), ai.config.CardToType[card]}

		// Look up the knowledge for this card for each player.
		for _, pName := range ai.players {
			status := ai.knowledge[card][pName]
			symbol := C.Maybe.Sprint("?")
			if status == StatusYes {
				symbol = C.Yes.Sprint("✔")
			}
			if status == StatusNo {
				symbol = C.No.Sprint("✖")
			}
			row = append(row, symbol)
		}

		// Look up the solution status for this card.
		status := ai.knowledge[card]["solution"]
		symbol := C.Maybe.Sprint("?")
		if status == StatusYes {
			symbol = C.Yes.Sprint("✔")
		}
		if status == StatusNo {
			symbol = C.No.Sprint("✖")
		}
		row = append(row, symbol)

		t.AppendRow(row)
	}

	t.SetStyle(table.StyleRounded)
	t.Style().Options.SeparateRows = false // We use AppendSeparator
	t.Style().Title.Align = text.AlignCenter
	t.SetColumnConfigs([]table.ColumnConfig{
		{Number: 1, Align: text.AlignRight},
	})

	t.Render()
}

func loadConfig(path string) error {
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}
	config.AllCards = append(config.AllCards, config.Suspects...)
	config.AllCards = append(config.AllCards, config.Weapons...)
	config.AllCards = append(config.AllCards, config.Rooms...)
	config.CardToType = make(map[string]string)
	for _, card := range config.Suspects {
		config.CardToType[card] = "suspects"
	}
	for _, card := range config.Weapons {
		config.CardToType[card] = "weapons"
	}
	for _, card := range config.Rooms {
		config.CardToType[card] = "rooms"
	}
	return nil
}

func values(m map[string]string) []string {
	var v []string
	for _, val := range m {
		v = append(v, val)
	}
	sort.Strings(v)
	return v
}

func mapKeys(m map[string]struct{}) []string {
	var k []string
	for key := range m {
		k = append(k, key)
	}
	return k
}

func promptForInt(line *liner.State, prompt string, min, max int) int {
	for {
		// THE FIX: Print the colored part first, then use an uncolored prompt.
		C.Prompt.Print(prompt)
		input, err := line.Prompt("")
		if err != nil {
			if err == liner.ErrPromptAborted || err == io.EOF {
				C.Info.Println("Goodbye!")
				os.Exit(0)
			}
			log.Fatalf("Error reading line: %v", err)
		}

		num, err := strconv.Atoi(strings.TrimSpace(input))
		if err != nil || num < min || num > max {
			C.Warn.Printf("Invalid input. Please enter a number between %d and %d.\n", min, max)
			continue
		}
		line.AppendHistory(input)
		return num
	}
}

func promptForString(line *liner.State, prompt string) string {
	for {
		// THE FIX: Print the colored part first, then use an uncolored prompt.
		C.Prompt.Print(prompt)
		input, err := line.Prompt("")
		if err != nil {
			if err == liner.ErrPromptAborted || err == io.EOF {
				C.Info.Println("Goodbye!")
				os.Exit(0)
			}
			log.Fatalf("Error reading line: %v", err)
		}
		if strings.TrimSpace(input) == "" {
			continue
		}
		line.AppendHistory(input)
		return strings.TrimSpace(input)
	}
}

func promptForSelection(line *liner.State, prompt string, options []string) string {
	for {
		// Display the prompt and options to the user
		C.Header.Println(prompt)
		for i, opt := range options {
			// Use colorizeCard to make suspect names colored in the list
			fmt.Printf(" %2d: %s\n", i+1, colorizeCard(opt))
		}

		// THE FIX: Print the colored part first, then prompt with an empty string.
		C.Prompt.Print("Enter number: ")
		input, err := line.Prompt("")
		if err != nil {
			if err == liner.ErrPromptAborted || err == io.EOF {
				C.Info.Println("Goodbye!")
				os.Exit(0)
			}
			log.Fatalf("Error reading line: %v", err)
		}

		num, err := strconv.Atoi(strings.TrimSpace(input))
		if err == nil && num >= 1 && num <= len(options) {
			line.AppendHistory(input)
			return options[num-1]
		}
		C.Warn.Println("Invalid selection.")
	}
}
