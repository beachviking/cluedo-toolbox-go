package ai

import (
	"cluedo-toolbox/internal/config"
	"cluedo-toolbox/internal/events"
	"math/rand"
	"sort"

	"github.com/sirupsen/logrus"
)

// CardStatus defines the knowledge state of a card.
type CardStatus int

const (
	StatusMaybe CardStatus = iota
	StatusYes
	StatusNo
)

// AdvancedAIBrain implements the Player interface with a sophisticated deduction engine.
type AdvancedAIBrain struct {
	name                  string
	config                *config.GameConfig
	players               []string
	hand                  map[string]struct{}
	knowledge             map[string]map[string]CardStatus
	unresolvedSuggestions []UnresolvedSuggestion
	recentSurgicalTargets *StringDeque
	strategies            []SuggestionStrategy
	log                   logrus.FieldLogger
	chooser               Chooser
	rand                  *rand.Rand
}

// --- Public Getters for CLI ---
func (ai *AdvancedAIBrain) Config() *config.GameConfig                  { return ai.config }
func (ai *AdvancedAIBrain) Players() []string                           { return ai.players }
func (ai *AdvancedAIBrain) Knowledge() map[string]map[string]CardStatus { return ai.knowledge }

// UnresolvedSuggestion tracks a disproval where the specific card shown is unknown.
type UnresolvedSuggestion struct {
	Disprover     string
	PossibleCards map[string]struct{}
}

// NewAdvancedAIBrain is the constructor for the AI player. It injects dependencies.
func NewAdvancedAIBrain(logger *logrus.Logger, rand *rand.Rand, chooser Chooser) *AdvancedAIBrain {
	ai := &AdvancedAIBrain{
		log:     logger,
		rand:    rand,    // Still needed for shuffling
		chooser: chooser, // Store the chooser
	}

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

func (ai *AdvancedAIBrain) Setup(cfg *config.GameConfig, playerNames []string, myName string) {
	ai.name = myName
	ai.config = cfg
	ai.players = playerNames
	// ai.log = ai.log.WithField("player", ai.name) // Add context to the logger

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
	ai.log.Debugf("Master deduction engine initialized.")
}

func (ai *AdvancedAIBrain) ReceiveHand(cards []string) {
	for _, card := range cards {
		ai.hand[card] = struct{}{}
		ai._markCardLocation(card, ai.name)
	}
	ai._runDeductionLoop()
}

// In HandleEvent(e events.Event)
func (ai *AdvancedAIBrain) HandleEvent(e events.Event) {
	switch event := e.(type) {
	case events.TurnResolvedEvent: // <-- Listen for the new event type
		ai.processTurnEvent(event)
	}
}

func (ai *AdvancedAIBrain) processTurnEvent(event events.TurnResolvedEvent) {
	// A special event type for direct reveals (e.g., from intrigue cards)
	if event.SuggesterName == "Game Event" {
		if event.DisproverName != "" && event.RevealedCard != "" {
			ai._markCardLocation(event.RevealedCard, event.DisproverName)
		}
		return
	}

	// The event's RevealedCard is the ground truth. We only learn from it if we were the suggester.
	if ai.name == event.SuggesterName {
		if event.DisproverName != "" && event.RevealedCard != "" {
			ai._markCardLocation(event.RevealedCard, event.DisproverName)
		} else if event.DisproverName == "" {
			ai.log.Infof("My suggestion was not disproved! Making powerful deductions.")
			for _, card := range event.Suggestion {
				if _, inHand := ai.hand[card]; !inHand {
					ai._markCardLocation(card, "solution")
				}
			}
		}
	} else if event.DisproverName != "" && event.DisproverName != ai.name {
		mystery := UnresolvedSuggestion{Disprover: event.DisproverName, PossibleCards: make(map[string]struct{})}
		for _, card := range event.Suggestion {
			mystery.PossibleCards[card] = struct{}{}
		}
		ai.unresolvedSuggestions = append(ai.unresolvedSuggestions, mystery)
		ai.log.Infof("Noted that %s holds one of %v.", event.DisproverName, mapKeys(mystery.PossibleCards))
	}

	ai._runDeductionLoop()
}

func (ai *AdvancedAIBrain) ChooseCardToShow(suggestion map[config.CardCategory]string) string {
	var canShow []string
	for _, card := range suggestion {
		if _, ok := ai.hand[card]; ok {
			canShow = append(canShow, card)
		}
	}
	return ai.chooser.Choose(canShow)
}

func (ai *AdvancedAIBrain) MakeSuggestion() map[config.CardCategory]string {
	ai.log.Debugf("Formulating a master-level suggestion...")
	for _, s := range ai.strategies {
		if suggestion, ok := s.BuildSuggestion(ai); ok {
			return suggestion
		}
	}
	return (&ExploreStrategy{}).mustBuild(ai)
}

func (ai *AdvancedAIBrain) ShouldAccuse() map[config.CardCategory]string {
	solution := make(map[config.CardCategory]string)
	categories := []config.CardCategory{config.CategorySuspect, config.CategoryWeapon, config.CategoryRoom}

	for _, cat := range categories {
		cardList := ai.config.CardListForCategory(cat)
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
		ai.log.Debugf("Finalizing knowledge before accusing.")
		return solution
	}
	return nil
}

func (ai *AdvancedAIBrain) DisplayNotes() {
	// The AI provides its knowledge to the CLI for rendering.
	// cli.RenderNotes(ai.name, ai.config, ai.players, ai.knowledge)
}

// --- Internal Deduction Logic ---

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

func (ai *AdvancedAIBrain) _markCardLocation(card, location string) bool {
	if _, isValid := ai.config.CardToType[card]; !isValid {
		ai.log.Errorf("FATAL LOGIC ERROR: _markCardLocation called with INVALID card name: '%s'", card)
		return false
	}
	if val, ok := ai.knowledge[card][location]; ok && val == StatusYes {
		return false // No change
	}
	ai.log.Debugf("Learned that '%s' is with %s.", card, location)
	allLocations := make([]string, len(ai.players)+1)
	copy(allLocations, ai.players)
	allLocations[len(ai.players)] = "solution"
	for _, loc := range allLocations {
		ai.knowledge[card][loc] = StatusNo
	}
	ai.knowledge[card][location] = StatusYes
	return true
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
			ai.log.Debugf("Pruning mystery: %s's options narrowed to %v", mystery.Disprover, mapKeys(prunedCards))
			mystery.PossibleCards = prunedCards
			changed = true
		}
		if len(prunedCards) == 1 {
			card := mapKeys(prunedCards)[0]
			ai.log.Infof("SOLVED A MYSTERY! %s must have shown '%s'.", mystery.Disprover, card)
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
		var maybes []string
		allLocations := make([]string, len(ai.players)+1)
		copy(allLocations, ai.players)
		allLocations[len(ai.players)] = "solution"
		isKnown := false
		for _, loc := range allLocations {
			if ai.knowledge[card][loc] == StatusYes {
				isKnown = true
				break
			}
			if ai.knowledge[card][loc] == StatusMaybe {
				maybes = append(maybes, loc)
			}
		}
		if !isKnown && len(maybes) == 1 {
			if ai._markCardLocation(card, maybes[0]) {
				changed = true
			}
		}
	}
	return changed
}

func (ai *AdvancedAIBrain) _deduceSolutionByElimination() bool {
	var changed bool
	categories := []config.CardCategory{config.CategorySuspect, config.CategoryWeapon, config.CategoryRoom}
	for _, cat := range categories {
		isSolved := false
		for _, card := range ai.config.CardListForCategory(cat) {
			if ai.knowledge[card]["solution"] == StatusYes {
				isSolved = true
				break
			}
		}
		if isSolved {
			continue
		}
		var maybes []string
		for _, card := range ai.config.CardListForCategory(cat) {
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

func mapKeys(m map[string]struct{}) []string {
	k := make([]string, 0, len(m))
	for key := range m {
		k = append(k, key)
	}
	sort.Strings(k)
	return k
}
