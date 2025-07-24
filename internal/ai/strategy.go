package ai

import (
	"cluedo-toolbox/internal/config"
	"sort"
)

// SuggestionStrategy defines the interface for an AI's decision-making logic.
type SuggestionStrategy interface {
	BuildSuggestion(ai *AdvancedAIBrain) (map[config.CardCategory]string, bool)
}

// --- Strategy Implementations ---

// 1. ExploitStrategy
type ExploitStrategy struct{}

func (s *ExploitStrategy) BuildSuggestion(ai *AdvancedAIBrain) (map[config.CardCategory]string, bool) {
	knownSolutionCards := make(map[config.CardCategory]string)
	knownCount := 0
	categories := []config.CardCategory{config.CategorySuspect, config.CategoryWeapon, config.CategoryRoom}

	for _, cat := range categories {
		for _, card := range ai.config.CardListForCategory(cat) {
			if ai.knowledge[card]["solution"] == StatusYes {
				knownSolutionCards[cat] = card
				knownCount++
				break
			}
		}
	}

	if knownCount > 0 && knownCount < 3 {
		ai.log.Infof("Strategy: EXPLOIT. I know %d/3 of the solution.", knownCount)
		suggestion := make(map[config.CardCategory]string)
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

// 2. SurgicalStrikeStrategy
type SurgicalStrikeStrategy struct{}

func (s *SurgicalStrikeStrategy) BuildSuggestion(ai *AdvancedAIBrain) (map[config.CardCategory]string, bool) {
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
		targetCard := ai.chooser.Choose(patientTargets)
		ai.log.Infof("[%s] Strategy: SURGICAL STRIKE. Targeting '%s'.", ai.name, targetCard)
		ai.recentSurgicalTargets.Push(targetCard)
		return ai._buildSuggestionAroundTarget(targetCard), true
		// targetCard := patientTargets[0]
		// ai.log.Infof("Strategy: SURGICAL STRIKE. Targeting '%s'.", targetCard)
		// ai.recentSurgicalTargets.Push(targetCard)
		// return ai._buildSuggestionAroundTarget(targetCard), true
	}
	return nil, false
}

// 3. ExploreStrategy
type ExploreStrategy struct{}

func (s *ExploreStrategy) BuildSuggestion(ai *AdvancedAIBrain) (map[config.CardCategory]string, bool) {
	ai.log.Infof("Strategy: EXPLORE. Gathering new information.")
	return s.mustBuild(ai), true
}

func (s *ExploreStrategy) mustBuild(ai *AdvancedAIBrain) map[config.CardCategory]string {
	return map[config.CardCategory]string{
		config.CategorySuspect: ai._pickUnknownCard(config.CategorySuspect),
		config.CategoryWeapon:  ai._pickUnknownCard(config.CategoryWeapon),
		config.CategoryRoom:    ai._pickUnknownCard(config.CategoryRoom),
	}
}

// --- Strategy Helpers ---

func (ai *AdvancedAIBrain) _pickUnknownCard(cat config.CardCategory) string {
	cardList := ai.config.CardListForCategory(cat)
	var maybes []string
	for _, card := range cardList {
		if _, inHand := ai.hand[card]; !inHand && ai.knowledge[card]["solution"] == StatusMaybe {
			maybes = append(maybes, card)
		}
	}
	if len(maybes) > 0 {
		return maybes[ai.rand.Intn(len(maybes))]
	}

	var notMyCards []string
	for _, card := range cardList {
		if _, inHand := ai.hand[card]; !inHand {
			notMyCards = append(notMyCards, card)
		}
	}
	if len(notMyCards) > 0 {
		// return notMyCards[ai.rand.Intn(len(notMyCards))]
		return ai.chooser.Choose(notMyCards) // <-- Use chooser

	}
	// return cardList[ai.rand.Intn(len(cardList))]
	return ai.chooser.Choose(cardList) // <-- Use chooser
}

func (ai *AdvancedAIBrain) _buildSuggestionAroundTarget(targetCard string) map[config.CardCategory]string {
	suggestion := make(map[config.CardCategory]string)
	targetCategory := ai.config.CardToType[targetCard]
	suggestion[targetCategory] = targetCard

	myHandSlice := ai.Hand()
	ai.rand.Shuffle(len(myHandSlice), func(i, j int) { myHandSlice[i], myHandSlice[j] = myHandSlice[j], myHandSlice[i] })

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
		if _, ok := suggestion[config.CategorySuspect]; !ok {
			suggestion[config.CategorySuspect] = ai._pickUnknownCard(config.CategorySuspect)
		}
		if _, ok := suggestion[config.CategoryWeapon]; !ok {
			suggestion[config.CategoryWeapon] = ai._pickUnknownCard(config.CategoryWeapon)
		}
		if _, ok := suggestion[config.CategoryRoom]; !ok {
			suggestion[config.CategoryRoom] = ai._pickUnknownCard(config.CategoryRoom)
		}
	}
	return suggestion
}

// --- Utility Types and Functions ---

type StringDeque struct {
	elements []string
	maxSize  int
}

func NewStringDeque(maxSize int) *StringDeque {
	return &StringDeque{maxSize: maxSize}
}
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
