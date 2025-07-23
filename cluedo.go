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

// CardCategory defines the type of a card using a typed enum.
type CardCategory int

const (
	CategorySuspect CardCategory = iota
	CategoryWeapon
	CategoryRoom
)

// String returns the string representation of a CardCategory.
func (cc CardCategory) String() string {
	return []string{"suspects", "weapons", "rooms"}[cc]
}

// CardStatus defines the knowledge state of a card using a typed enum.
type CardStatus int

const (
	StatusMaybe CardStatus = iota
	StatusYes
	StatusNo
)

type GameConfig struct {
	Suspects   []string                `json:"suspects"`
	Weapons    []string                `json:"weapons"`
	Rooms      []string                `json:"rooms"`
	AllCards   []string                `json:"-"` // Populated at runtime
	CardToType map[string]CardCategory `json:"-"` // Populated at runtime
}

// deepCopy creates a new GameConfig with all slices copied.
func (c GameConfig) deepCopy() GameConfig {
	newCfg := GameConfig{
		CardToType: make(map[string]CardCategory),
	}
	newCfg.Suspects = make([]string, len(c.Suspects))
	copy(newCfg.Suspects, c.Suspects)
	newCfg.Weapons = make([]string, len(c.Weapons))
	copy(newCfg.Weapons, c.Weapons)
	newCfg.Rooms = make([]string, len(c.Rooms))
	copy(newCfg.Rooms, c.Rooms)
	newCfg.AllCards = make([]string, len(c.AllCards))
	copy(newCfg.AllCards, c.AllCards)
	for k, v := range c.CardToType {
		newCfg.CardToType[k] = v
	}
	return newCfg
}

// TurnEvent encapsulates all information about a single turn's suggestion.
type TurnEvent struct {
	SuggesterName string
	Suggestion    map[CardCategory]string
	DisproverName string // Empty if no one disproved
	RevealedCard  string // Card shown to the suggester; empty if not seen by the current player
}

// --- Player Interface ---

type Player interface {
	Name() string
	IsHuman() bool
	Hand() []string // Returns the player's cards.
	Setup(cfg GameConfig, playerNames []string, myName string)
	ReceiveHand(cards []string)
	MakeSuggestion() map[CardCategory]string
	ShouldAccuse() map[CardCategory]string
	ProcessTurnEvent(event TurnEvent)
	ChooseCardToShow(suggestion map[CardCategory]string) string
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
	Solution map[CardCategory]string
	turn     int
}

func NewGame(cfg GameConfig, numHumans, numAI int) *Game {
	if numHumans+numAI > len(cfg.Suspects) {
		log.Fatalf("Cannot create a game with %d players; only %d suspects are available.", numHumans+numAI, len(cfg.Suspects))
	}
	playerNames := cfg.Suspects[:numHumans+numAI]
	rand.Shuffle(len(playerNames), func(i, j int) { playerNames[i], playerNames[j] = playerNames[j], playerNames[i] })

	g := &Game{
		Config:   cfg,
		Solution: make(map[CardCategory]string),
	}

	for i, name := range playerNames {
		var p Player
		if i < numHumans {
			p = NewHumanPlayer()
		} else {
			p = NewAdvancedAIBrain()
		}
		// *** BUG FIX ***: Give each player a DEEP COPY of the config and player list
		// to prevent shared state corruption.
		playerNamesCopy := make([]string, len(playerNames))
		copy(playerNamesCopy, playerNames)
		p.Setup(cfg.deepCopy(), playerNamesCopy, name)
		g.Players = append(g.Players, p)
	}
	return g
}

func (g *Game) Deal() {
	deck := make([]string, len(g.Config.AllCards))
	copy(deck, g.Config.AllCards)
	rand.Shuffle(len(deck), func(i, j int) { deck[i], deck[j] = deck[j], deck[i] })

	dealtCategories := make(map[CardCategory]bool)
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

	solutionForPrinting := make(map[string]string)
	for cat, card := range g.Solution {
		solutionForPrinting[cat.String()] = card
	}
	log.Debugf("Ground Truth Initialized. Solution: %+v", solutionForPrinting)
}

func (g *Game) HandleSuggestion(suggester Player, suggestion map[CardCategory]string) (string, string) {
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
	knowledge             map[string]map[string]CardStatus // card -> location -> status
	unresolvedSuggestions []UnresolvedSuggestion
	recentSurgicalTargets *StringDeque
	strategies            []SuggestionStrategy
}

type UnresolvedSuggestion struct {
	Disprover     string
	PossibleCards map[string]struct{}
}

func NewAdvancedAIBrain() *AdvancedAIBrain {
	ai := &AdvancedAIBrain{}
	// Strategies are prioritized. The first one that can apply will be used.
	ai.strategies = []SuggestionStrategy{
		&ExploitStrategy{},
		&SurgicalStrikeStrategy{},
		&ExploreStrategy{},
	}
	return ai
}
func (ai *AdvancedAIBrain) Name() string  { return ai.name }
func (ai *AdvancedAIBrain) IsHuman() bool { return false }
func (ai *AdvancedAIBrain) Hand() []string {
	var cards []string
	for card := range ai.hand {
		cards = append(cards, card)
	}
	sort.Strings(cards)
	return cards
}

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
	for _, card := range cards {
		ai.hand[card] = struct{}{}
		ai._markCardLocation(card, ai.name)
	}
	ai._runDeductionLoop()
}

func (ai *AdvancedAIBrain) ProcessTurnEvent(event TurnEvent) {
	// A special event type for direct reveals (e.g., from intrigue cards)
	if event.SuggesterName == "Game Event" {
		if event.DisproverName != "" && event.RevealedCard != "" {
			ai._markCardLocation(event.RevealedCard, event.DisproverName)
			ai._runDeductionLoop()
		}
		return
	}

	// I made the suggestion
	if ai.name == event.SuggesterName {
		if event.DisproverName != "" && event.RevealedCard != "" {
			// My suggestion was disproved, and I was shown the card.
			ai._markCardLocation(event.RevealedCard, event.DisproverName)
		} else if event.DisproverName == "" {
			// My suggestion was not disproved by anyone. This is powerful.
			log.Infof("[%s] My suggestion was not disproved! Making powerful deductions.", colorizeCard(ai.name))
			for _, card := range event.Suggestion {
				if _, inHand := ai.hand[card]; !inHand {
					ai._markCardLocation(card, "solution")
				}
			}
		}
	} else if event.DisproverName != "" && event.DisproverName != ai.name {
		// Someone else's suggestion was disproved by a third party.
		newMystery := UnresolvedSuggestion{Disprover: event.DisproverName, PossibleCards: make(map[string]struct{})}
		for _, card := range event.Suggestion {
			if _, isValid := ai.config.CardToType[card]; isValid {
				newMystery.PossibleCards[card] = struct{}{}
			}
		}
		if len(newMystery.PossibleCards) > 0 {
			ai.unresolvedSuggestions = append(ai.unresolvedSuggestions, newMystery)
			log.Infof("%s noted that %s holds one of %v. (New unsolved mystery)", makeAiTitle(ai.name), colorizeCard(event.DisproverName), mapKeys(newMystery.PossibleCards))
		}
	}
	ai._runDeductionLoop()
}

func (ai *AdvancedAIBrain) ChooseCardToShow(suggestion map[CardCategory]string) string {
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

func (ai *AdvancedAIBrain) MakeSuggestion() map[CardCategory]string {
	log.Debugf("[%s's Brain] Formulating a master-level suggestion...", ai.name)
	for _, s := range ai.strategies {
		if suggestion, ok := s.BuildSuggestion(ai); ok {
			return suggestion
		}
	}
	return (&ExploreStrategy{}).mustBuild(ai)
}

func (ai *AdvancedAIBrain) ShouldAccuse() map[CardCategory]string {
	solution := make(map[CardCategory]string)
	categories := []CardCategory{CategorySuspect, CategoryWeapon, CategoryRoom}

	for _, cat := range categories {
		cardList := ai.config.cardListForCategory(cat)
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
			return nil
		}
	}

	if len(solution) == 3 {
		log.Debugf("[%s] Finalizing knowledge before accusing.", ai.name)
		ai._runDeductionLoop()
		log.Infof("%s is making a confident ACCUSATION: %v", colorizeCard(ai.name), values(solution))
		return solution
	}
	return nil
}

// --- AI Strategy Pattern ---

type SuggestionStrategy interface {
	BuildSuggestion(ai *AdvancedAIBrain) (map[CardCategory]string, bool)
}

type ExploitStrategy struct{}

func (s *ExploitStrategy) BuildSuggestion(ai *AdvancedAIBrain) (map[CardCategory]string, bool) {
	knownSolutionCards := make(map[CardCategory]string)
	knownCount := 0
	categories := []CardCategory{CategorySuspect, CategoryWeapon, CategoryRoom}

	for _, cat := range categories {
		for _, card := range ai.config.cardListForCategory(cat) {
			if ai.knowledge[card]["solution"] == StatusYes {
				knownSolutionCards[cat] = card
				knownCount++
				break
			}
		}
	}

	if knownCount > 0 && knownCount < 3 {
		log.Infof("[%s] Strategy: EXPLOIT. I know %d/3 of the solution, testing a theory.", colorizeCard(ai.name), knownCount)
		suggestion := make(map[CardCategory]string)
		for _, cat := range categories {
			if card, ok := knownSolutionCards[cat]; ok {
				suggestion[cat] = card
			} else {
				suggestion[cat] = ai._pickUnknownCard(cat)
			}
		}
		return suggestion, true
	}
	return nil, false
}

type SurgicalStrikeStrategy struct{}

func (s *SurgicalStrikeStrategy) BuildSuggestion(ai *AdvancedAIBrain) (map[CardCategory]string, bool) {
	if len(ai.unresolvedSuggestions) == 0 {
		return nil, false
	}

	cardFrequency := make(map[string]int)
	for _, mystery := range ai.unresolvedSuggestions {
		for card := range mystery.PossibleCards {
			cardFrequency[card]++
		}
	}

	if len(cardFrequency) > 0 {
		sortedTargets := sortByValue(cardFrequency)
		var patientTargets []string
		for _, card := range sortedTargets {
			if !ai.recentSurgicalTargets.Contains(card) {
				patientTargets = append(patientTargets, card)
			}
		}
		if len(patientTargets) == 0 {
			patientTargets = sortedTargets
		}

		targetCard := patientTargets[0]
		log.Infof("[%s] Strategy: SURGICAL STRIKE. Targeting '%s' to solve a mystery.", colorizeCard(ai.name), targetCard)
		ai.recentSurgicalTargets.Push(targetCard)
		return ai._buildSuggestionAroundTarget(targetCard), true
	}
	return nil, false
}

type ExploreStrategy struct{}

func (s *ExploreStrategy) BuildSuggestion(ai *AdvancedAIBrain) (map[CardCategory]string, bool) {
	log.Infof("[%s] Strategy: EXPLORE. Gathering new information.", colorizeCard(ai.name))
	return s.mustBuild(ai), true
}

func (s *ExploreStrategy) mustBuild(ai *AdvancedAIBrain) map[CardCategory]string {
	return map[CardCategory]string{
		CategorySuspect: ai._pickUnknownCard(CategorySuspect),
		CategoryWeapon:  ai._pickUnknownCard(CategoryWeapon),
		CategoryRoom:    ai._pickUnknownCard(CategoryRoom),
	}
}

// --- AI Helper & Deduction Methods ---

func (ai *AdvancedAIBrain) _pickUnknownCard(cat CardCategory) string {
	cardList := ai.config.cardListForCategory(cat)
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

func (ai *AdvancedAIBrain) _buildSuggestionAroundTarget(targetCard string) map[CardCategory]string {
	suggestion := make(map[CardCategory]string)
	targetCategory := ai.config.CardToType[targetCard]
	suggestion[targetCategory] = targetCard

	myHandSlice := ai.Hand()
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
		if _, ok := suggestion[CategorySuspect]; !ok {
			suggestion[CategorySuspect] = ai._pickUnknownCard(CategorySuspect)
		}
		if _, ok := suggestion[CategoryWeapon]; !ok {
			suggestion[CategoryWeapon] = ai._pickUnknownCard(CategoryWeapon)
		}
		if _, ok := suggestion[CategoryRoom]; !ok {
			suggestion[CategoryRoom] = ai._pickUnknownCard(CategoryRoom)
		}
	}
	return suggestion
}

func (ai *AdvancedAIBrain) _markCardLocation(card, location string) bool {
	if _, isValidCard := ai.config.CardToType[card]; !isValidCard {
		log.Errorf("FATAL LOGIC ERROR: _markCardLocation called with INVALID card name: '%s'", card)
		return false
	}
	if val, ok := ai.knowledge[card][location]; ok && val == StatusYes {
		return false // No change
	}

	log.Debugf("[%s's Brain] learned that '%s' is with %s.", ai.name, card, location)

	allLocations := make([]string, len(ai.players)+1)
	copy(allLocations, ai.players)
	allLocations[len(ai.players)] = "solution"

	for _, loc := range allLocations {
		ai.knowledge[card][loc] = StatusNo
	}
	ai.knowledge[card][location] = StatusYes
	return true
}

func (ai *AdvancedAIBrain) _runDeductionLoop() {
	for i := 0; i < 10; i++ { // Safety break
		var changed bool
		changed = ai._pruneAndSolveMysteries() || changed
		changed = ai._deduceSolutionByElimination() || changed
		changed = ai._deduceCardLocationsByElimination() || changed

		if !changed {
			break
		}
	}
}

func (ai *AdvancedAIBrain) _pruneAndSolveMysteries() bool {
	var changed bool
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
			changed = true
		}

		if len(prunedCards) == 1 {
			card := mapKeys(prunedCards)[0]
			log.Infof("%s SOLVED A MYSTERY! %s must have shown '%s'.", makeAiTitle(ai.name), colorizeCard(mystery.Disprover), card)
			if ai._markCardLocation(card, mystery.Disprover) {
				changed = true
			}
		} else if len(prunedCards) > 1 {
			remainingMysteries = append(remainingMysteries, mystery)
		}
	}
	if len(remainingMysteries) < len(ai.unresolvedSuggestions) {
		changed = true
	}
	ai.unresolvedSuggestions = remainingMysteries
	return changed
}

func (ai *AdvancedAIBrain) _deduceCardLocationsByElimination() bool {
	var changed bool
	for _, card := range ai.config.AllCards {
		if ai.isCardKnown(card) {
			continue
		}

		allLocations := make([]string, len(ai.players)+1)
		copy(allLocations, ai.players)
		allLocations[len(ai.players)] = "solution"

		var maybes []string
		for _, loc := range allLocations {
			if ai.knowledge[card][loc] == StatusMaybe {
				maybes = append(maybes, loc)
			}
		}

		if len(maybes) == 1 {
			finalLocation := maybes[0]
			if ai._markCardLocation(card, finalLocation) {
				changed = true
			}
		}
	}
	return changed
}

func (ai *AdvancedAIBrain) _deduceSolutionByElimination() bool {
	var changed bool
	categories := []CardCategory{CategorySuspect, CategoryWeapon, CategoryRoom}

	for _, cat := range categories {
		if ai.isSolutionKnownFor(cat) {
			continue
		}
		var maybes []string
		for _, card := range ai.config.cardListForCategory(cat) {
			if ai.knowledge[card]["solution"] == StatusMaybe {
				maybes = append(maybes, card)
			}
		}
		if len(maybes) == 1 {
			if ai._markCardLocation(maybes[0], "solution") {
				changed = true
			}
		}
	}
	return changed
}

func (ai *AdvancedAIBrain) isCardKnown(card string) bool {
	allLocations := make([]string, len(ai.players)+1)
	copy(allLocations, ai.players)
	allLocations[len(ai.players)] = "solution"

	for _, loc := range allLocations {
		if ai.knowledge[card][loc] == StatusYes {
			return true
		}
	}
	return false
}

func (ai *AdvancedAIBrain) isSolutionKnownFor(cat CardCategory) bool {
	for _, card := range ai.config.cardListForCategory(cat) {
		if ai.knowledge[card]["solution"] == StatusYes {
			return true
		}
	}
	return false
}

// --- Human Player Implementation ---
type HumanPlayer struct {
	name string
	cfg  GameConfig
	hand map[string]struct{}
}

func NewHumanPlayer() *HumanPlayer   { return &HumanPlayer{hand: make(map[string]struct{})} }
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
func (h *HumanPlayer) Setup(cfg GameConfig, playerNames []string, myName string) {
	h.name = myName
	h.cfg = cfg
}
func (h *HumanPlayer) ReceiveHand(cards []string) {
	for _, card := range cards {
		h.hand[card] = struct{}{}
	}
	C.Info.Printf("\nYour hand: %v\n", cards)
}
func (h *HumanPlayer) MakeSuggestion() map[CardCategory]string { return nil }
func (h *HumanPlayer) ShouldAccuse() map[CardCategory]string   { return nil }
func (h *HumanPlayer) ProcessTurnEvent(event TurnEvent) {
	if h.name == event.SuggesterName && event.RevealedCard != "" {
		C.Info.Printf("You were shown the card: %s\n", colorizeCard(event.RevealedCard))
	}
}
func (h *HumanPlayer) ChooseCardToShow(suggestion map[CardCategory]string) string {
	var canShow []string
	for _, card := range suggestion {
		if _, ok := h.hand[card]; ok {
			canShow = append(canShow, card)
		}
	}
	if len(canShow) == 0 {
		return ""
	}
	return canShow[0]
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

	gameConfig, err := loadConfig("default_config.json")
	if err != nil {
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

	switch args[0] {
	case "detective":
		runDetectiveMode(line, gameConfig)
	case "start":
		if len(args) != 3 {
			printUsage()
			return
		}
		numHumans, _ := strconv.Atoi(args[1])
		numAI, _ := strconv.Atoi(args[2])
		C.Header.Println("--- Running Fast Simulation ---")
		game := NewGame(gameConfig, numHumans, numAI)
		game.Deal()
		runSimulationLoop(game)
	default:
		printUsage()
	}
}

func runDetectiveMode(line *liner.State, cfg GameConfig) {
	C.Info.Println("\n--- Starting Detective Mode Co-Pilot ---")

	numPlayers := promptForInt(line, "How many players are in the real game? (2-6): ", 2, 6)
	var playerNames []string
	for i := 0; i < numPlayers; i++ {
		name := promptForString(line, fmt.Sprintf("Enter name for Player %d: ", i+1))
		playerNames = append(playerNames, name)
	}
	myPlayerName := promptForSelection(line, "Which player are you?", playerNames)

	C.Info.Println("\nSelect the cards in your hand. Type 'done' when finished.")
	myHand := promptForCards(line, cfg, true, 0)

	brain := NewAdvancedAIBrain()
	brain.Setup(cfg, playerNames, myPlayerName)
	brain.ReceiveHand(myHand)

	C.Info.Println("\nDetective Mode is active! Your co-pilot is ready.")
	brain.DisplayNotes()
	printDetectiveHelp()

	for {
		input, err := line.Prompt("(detective) ")
		if err != nil {
			if err == liner.ErrPromptAborted || err == io.EOF {
				C.Info.Println("\nGoodbye!")
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

		switch cmd {
		case "log", "l":
			handleLogCommand(line, brain)
		case "reveal", "r":
			handleRevealCommand(line, brain)
		case "suggest", "s":
			handleSuggestCommand(brain)
		case "notes", "n":
			brain.DisplayNotes()
		case "hand", "ha":
			handleHandCommand(brain)
		case "help", "h":
			printDetectiveHelp()
		case "quit", "q":
			C.Info.Println("Exiting detective mode.")
			return
		default:
			C.Warn.Printf("Unknown command '%s'. Type 'help' for a list of commands.\n", cmd)
		}
	}
}

func handleHandCommand(brain Player) {
	C.Header.Println("\n--- Your Hand ---")
	for _, card := range brain.Hand() {
		C.Info.Println(" - " + colorizeCard(card))
	}
}

func handleLogCommand(line *liner.State, brain Player) {
	C.Info.Println("\n--- Log a Game Turn ---")
	ai, ok := brain.(*AdvancedAIBrain)
	if !ok {
		C.Warn.Println("This command can only be used with an AI co-pilot.")
		return
	}

	suggester := promptForSelection(line, "Who made the suggestion?", ai.players)

	C.Info.Println("What 3 cards were suggested? (Use numbers or names)")
	suggestionCards := promptForCards(line, ai.config, false, 3)
	if len(suggestionCards) != 3 {
		C.Warn.Println("Error: A suggestion must have exactly 3 cards.")
		return
	}
	suggestion := make(map[CardCategory]string)
	for _, card := range suggestionCards {
		suggestion[ai.config.CardToType[card]] = card
	}

	disproverOptions := append(ai.players, "No One")
	disprover := promptForSelection(line, "Who disproved the suggestion?", disproverOptions)

	event := TurnEvent{
		SuggesterName: suggester,
		Suggestion:    suggestion,
	}

	if disprover != "No One" {
		event.DisproverName = disprover
		if suggester == brain.Name() {
			C.Info.Println("What card were you shown? (Use numbers or names)")
			revealedCards := promptForCards(line, ai.config, true, 1)
			if len(revealedCards) > 0 {
				event.RevealedCard = revealedCards[0]
			}
		}
	}

	brain.ProcessTurnEvent(event)
	C.Info.Println("Turn logged. Here are your updated notes:")
	brain.DisplayNotes()
}

func handleRevealCommand(line *liner.State, brain Player) {
	C.Info.Println("\n--- Log a Revealed Card ---")
	ai, ok := brain.(*AdvancedAIBrain)
	if !ok {
		C.Warn.Println("This command can only be used with an AI co-pilot.")
		return
	}

	player := promptForSelection(line, "Which player revealed a card?", ai.players)
	C.Info.Println("Which card did they reveal? (Use number or name)")
	revealedCards := promptForCards(line, ai.config, true, 1)
	if len(revealedCards) == 0 {
		return // User cancelled
	}

	event := TurnEvent{
		SuggesterName: "Game Event",
		DisproverName: player,
		RevealedCard:  revealedCards[0],
	}
	brain.ProcessTurnEvent(event)
	C.Info.Println("Revealed card logged.")
	brain.DisplayNotes()
}

func handleSuggestCommand(brain Player) {
	C.Header.Println("\n--- AI Co-Pilot Suggestion ---")
	suggestion := brain.MakeSuggestion()
	var parts []string
	for _, cat := range []CardCategory{CategorySuspect, CategoryWeapon, CategoryRoom} {
		parts = append(parts, colorizeCard(suggestion[cat]))
	}
	C.Info.Printf("The AI suggests you propose: %s\n", strings.Join(parts, ", "))
}

func runSimulationLoop(g *Game) {
	C.Header.Println("--- Starting Game ---")
	for _, p := range g.Players {
		if !p.IsHuman() {
			p.DisplayNotes()
			break
		}
	}

	winner := ""
	for g.turn < 50 {
		currentPlayer := g.Players[g.turn%len(g.Players)]
		C.Header.Printf("\n--- Turn %d: %s ---\n", g.turn+1, colorizeCard(currentPlayer.Name()))

		if accusation := currentPlayer.ShouldAccuse(); accusation != nil {
			isCorrect := true
			for cat, card := range accusation {
				if g.Solution[cat] != card {
					isCorrect = false
					break
				}
			}
			C.Info.Printf("%s ACCUSES with: %s, %s, %s\n", colorizeCard(currentPlayer.Name()), accusation[CategorySuspect], accusation[CategoryWeapon], accusation[CategoryRoom])
			if isCorrect {
				C.Yes.Println("The accusation is CORRECT!", colorizeCard(currentPlayer.Name()), "wins!")
				winner = currentPlayer.Name()
			} else {
				C.No.Println("The accusation is INCORRECT!", colorizeCard(currentPlayer.Name()), "is out of the game.")
			}
			break
		}

		suggestion := currentPlayer.MakeSuggestion()
		C.Info.Printf("%s suggests: %s, %s, %s\n", colorizeCard(currentPlayer.Name()), suggestion[CategorySuspect], suggestion[CategoryWeapon], suggestion[CategoryRoom])
		disproverName, revealedCard := g.HandleSuggestion(currentPlayer, suggestion)

		event := TurnEvent{
			SuggesterName: currentPlayer.Name(),
			Suggestion:    suggestion,
			DisproverName: disproverName,
		}

		if disproverName != "" {
			C.Info.Printf("-> %s shows a card to %s.\n", colorizeCard(disproverName), colorizeCard(currentPlayer.Name()))
			log.Debugf(" (The card was '%s')", revealedCard)
		} else {
			C.Info.Println("-> No player could show a card.")
		}

		for _, p := range g.Players {
			event.RevealedCard = ""
			if p.Name() == currentPlayer.Name() {
				event.RevealedCard = revealedCard
			}
			p.ProcessTurnEvent(event)
		}

		g.turn++
		if !currentPlayer.IsHuman() {
			time.Sleep(50 * time.Millisecond)
		}
	}

	C.Header.Println("\n--- GAME OVER ---")
	solutionForPrinting := make(map[string]string)
	for _, cat := range []CardCategory{CategorySuspect, CategoryWeapon, CategoryRoom} {
		solutionForPrinting[cat.String()] = g.Solution[cat]
	}
	C.Info.Printf("The correct solution was: %v\n", solutionForPrinting)

	if winner != "" {
		C.Yes.Printf("Winner: %s\n", colorizeCard(winner))
		var winningPlayer Player
		for _, p := range g.Players {
			if p.Name() == winner {
				winningPlayer = p
				break
			}
		}
		if winningPlayer != nil && !winningPlayer.IsHuman() {
			C.Header.Println("\n========================================")
			C.Header.Printf("Final Notes for Winner: %s\n", colorizeCard(winner))
			C.Header.Println("========================================")
			winningPlayer.DisplayNotes()
		}
	} else {
		C.Warn.Println("Game ended without a correct accusation.")
	}
}

// --- UI and Helper Functions ---
func printUsage() {
	fmt.Println(C.Header.Sprint("\n--- Cluedo Toolbox ---"))
	fmt.Println("Usage:")
	fmt.Println(C.Prompt.Sprint("  go run . detective"))
	fmt.Println("    To run the AI co-pilot for a real-life game.")
	fmt.Println(C.Prompt.Sprint("  go run . start <humans> <ai>"))
	fmt.Println("    To run a fast simulation with a mix of players.")
	fmt.Println("\nFlags:")
	fmt.Println("  -loglevel debug    Enable detailed AI logic tracing.")
}

func printDetectiveHelp() {
	C.Header.Println("\n--- Detective Mode Help ---")
	fmt.Println("Log events from your real-life game, and the AI will track everything for you.")
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
		{"help", "h", "Show this help message."},
		{"quit", "q", "Exit detective mode."},
	})
	t.SetStyle(table.StyleLight)
	t.Render()
	C.Prompt.Print("\nEnter a command: ")
}

func promptForCards(line *liner.State, cfg GameConfig, requireAtLeastOne bool, exactCount int) []string {
	var cards []string
	cardSet := make(map[string]struct{})

	maxLen := 0
	for _, card := range cfg.AllCards {
		if len(card) > maxLen {
			maxLen = len(card)
		}
	}
	numCols := 3
	C.Header.Println("\n--- Card List ---")
	for i, card := range cfg.AllCards {
		cardID := i + 1
		fmt.Printf("%2d: %-*s", cardID, maxLen+2, colorizeCard(card))
		if (i+1)%numCols == 0 || i == len(cfg.AllCards)-1 {
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

		input := promptForString(line, prompt+": ")
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
			C.Warn.Printf("Error: Card '%s' not found.\n", input)
		} else if _, exists := cardSet[foundCard]; exists {
			C.Warn.Printf("You have already entered '%s'.\n", foundCard)
		} else {
			cards = append(cards, foundCard)
			cardSet[foundCard] = struct{}{}
			C.Info.Printf(" -> Added: %s\n", colorizeCard(foundCard))
			line.AppendHistory(input)
		}
	}
	return cards
}

func (ai *AdvancedAIBrain) DisplayNotes() {
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.SetTitle(fmt.Sprintf("%s's Detective Notes", ai.name))

	header := table.Row{"ID", "Card", "Type"}
	for _, pName := range ai.players {
		header = append(header, colorizeCard(pName))
	}
	header = append(header, "Solution")
	t.AppendHeader(header)

	// *** BUG FIX ***: Iterate over the canonical AllCards list to prevent
	// showing corrupted data like 'solution' or wrong IDs.
	for cardID, card := range ai.config.AllCards {
		if cardID > 0 && ai.config.CardToType[card] != ai.config.CardToType[ai.config.AllCards[cardID-1]] {
			t.AppendSeparator()
		}

		cat := ai.config.CardToType[card]
		row := table.Row{cardID + 1, colorizeCard(card), cat.String()}
		for _, pName := range ai.players {
			row = append(row, statusToSymbol(ai.knowledge[card][pName]))
		}
		row = append(row, statusToSymbol(ai.knowledge[card]["solution"]))
		t.AppendRow(row)
	}

	t.SetStyle(table.StyleRounded)
	t.Style().Options.SeparateRows = false
	t.Style().Title.Align = text.AlignCenter
	t.SetColumnConfigs([]table.ColumnConfig{{Number: 1, Align: text.AlignRight}})
	t.Render()
}

func statusToSymbol(status CardStatus) string {
	switch status {
	case StatusYes:
		return C.Yes.Sprint("✔")
	case StatusNo:
		return C.No.Sprint("✖")
	default:
		return C.Maybe.Sprint("?")
	}
}

func loadConfig(path string) (GameConfig, error) {
	var cfg GameConfig
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}

	cfg.CardToType = make(map[string]CardCategory)
	sort.Strings(cfg.Suspects)
	sort.Strings(cfg.Weapons)
	sort.Strings(cfg.Rooms)

	for _, card := range cfg.Suspects {
		cfg.AllCards = append(cfg.AllCards, card)
		cfg.CardToType[card] = CategorySuspect
	}
	for _, card := range cfg.Weapons {
		cfg.AllCards = append(cfg.AllCards, card)
		cfg.CardToType[card] = CategoryWeapon
	}
	for _, card := range cfg.Rooms {
		cfg.AllCards = append(cfg.AllCards, card)
		cfg.CardToType[card] = CategoryRoom
	}
	return cfg, nil
}

func (c GameConfig) cardListForCategory(cat CardCategory) []string {
	switch cat {
	case CategorySuspect:
		return c.Suspects
	case CategoryWeapon:
		return c.Weapons
	case CategoryRoom:
		return c.Rooms
	default:
		return nil
	}
}

func values(m map[CardCategory]string) []string {
	var v []string
	for _, cat := range []CardCategory{CategorySuspect, CategoryWeapon, CategoryRoom} {
		v = append(v, m[cat])
	}
	return v
}

func mapKeys(m map[string]struct{}) []string {
	var k []string
	for key := range m {
		k = append(k, key)
	}
	sort.Strings(k)
	return k
}

func sortByValue(m map[string]int) []string {
	type kv struct {
		Key   string
		Value int
	}
	var ss []kv
	for k, v := range m {
		ss = append(ss, kv{k, v})
	}
	sort.Slice(ss, func(i, j int) bool {
		return ss[i].Value > ss[j].Value
	})
	var result []string
	for _, kv := range ss {
		result = append(result, kv.Key)
	}
	return result
}

func promptForInt(line *liner.State, prompt string, min, max int) int {
	for {
		input := promptForString(line, prompt)
		num, err := strconv.Atoi(strings.TrimSpace(input))
		if err != nil || num < min || num > max {
			C.Warn.Printf("Invalid input. Please enter a number between %d and %d.\n", min, max)
			continue
		}
		return num
	}
}

func promptForString(line *liner.State, prompt string) string {
	for {
		C.Prompt.Print(prompt)
		input, err := line.Prompt("")
		if err != nil {
			if err == liner.ErrPromptAborted || err == io.EOF {
				C.Info.Println("\nGoodbye!")
				os.Exit(0)
			}
			log.Fatalf("Error reading line: %v", err)
		}
		trimmed := strings.TrimSpace(input)
		if trimmed != "" {
			line.AppendHistory(trimmed)
			return trimmed
		}
	}
}

func promptForSelection(line *liner.State, prompt string, options []string) string {
	for {
		C.Header.Println("\n" + prompt)
		for i, opt := range options {
			fmt.Printf(" %2d: %s\n", i+1, colorizeCard(opt))
		}

		input := promptForString(line, "Enter number or name: ")
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
