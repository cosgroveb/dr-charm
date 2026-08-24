package dragonrealms

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type creatureStatus struct {
	hostile    bool
	disengaged bool
	flying     bool
}

type injuryReading struct {
	kind     string
	severity int
}

type flagsPhase uint8

const (
	captureInactive flagsPhase = iota
	captureWaiting
	captureStarted
)

type flagsCapture struct {
	phase    flagsPhase
	deadline time.Time
	lines    int
}

type reducer struct {
	public      Snapshot
	pendingRoom Room

	components       map[string]string
	resources        map[string]int
	indicators       map[string]bool
	creatureStatuses map[string]creatureStatus
	injuries         map[string]injuryReading
	containers       map[string]string
	containerTargets map[string]string
	skills           map[string]int
	flags            map[string]bool
	guild            string
	handNouns        Hands
	handExists       Hands

	flagsCapture flagsCapture
	inExperience bool
	newsCategory string
}

var (
	guildPattern        = regexp.MustCompile(`^\s*Name:\s.+?\bGuild:\s+([A-Za-z][A-Za-z' ]*?)\s*$`)
	flagPattern         = regexp.MustCompile(`^\s*([A-Za-z]{3,20})\s+(ON|OFF)\b`)
	expPattern          = regexp.MustCompile(`([A-Z][A-Za-z]*(?: [A-Z][A-Za-z]*)*):\s+(\d+)\s+\d+%`)
	rankPercentPattern  = regexp.MustCompile(`\b(\d+)\s+\d+%`)
	deadCreaturePattern = regexp.MustCompile(`(?i:appears dead|(dead))`)
	newsCategoryPattern = regexp.MustCompile(`^\*\* Category (\d+) - .* \*\*$`)
	newsItemPattern     = regexp.MustCompile(`^\s*(\d+) - .+$`)
)

var knownFlags = func() map[string]string {
	result := make(map[string]string)
	for _, name := range strings.Split("LogOn,LogOff,Disconnect,ShowDeaths,RoomNames,Description,RoomBrief,BattleBrief,CombatBrief,MonsterBold,Inactivity,Portrait,StatusPrompt,AvoidJoiners,AvoidHolders,AvoidDancers,AvoidWhispers,AvoidDraggers,AvoidTeachers,AvoidSinging,NoHarnessShare,HarnessWarning,HarnessVerbose,AutoSneak,ConciseThoughts,HideLogin,DeathLocation,HidePreStrings,HidePostStrings,HideMyCusLogin,HideOtCusLogin,HideTrivia,SkinKills,LootKills,ShowRoomID", ",") {
		result[strings.ToLower(name)] = name
	}
	return result
}()

func newReducer(character string) *reducer {
	return &reducer{
		public:           Snapshot{Connection: ConnectionConnected, Character: character},
		components:       make(map[string]string),
		resources:        make(map[string]int),
		indicators:       make(map[string]bool),
		creatureStatuses: make(map[string]creatureStatus),
		injuries:         make(map[string]injuryReading),
		containers:       make(map[string]string),
		containerTargets: make(map[string]string),
		skills:           make(map[string]int),
		flags:            make(map[string]bool),
	}
}

func (r *reducer) apply(action protocolAction) (Update, bool) {
	return r.applyAt(action, time.Now())
}

func (r *reducer) applyAt(action protocolAction, now time.Time) (Update, bool) {
	var update Update
	publish := false
	for _, event := range action.events {
		switch event.kind {
		case eventDisplay:
			if event.display.Kind == DisplayText {
				display, suppress := r.applyText(event.display, now)
				if suppress {
					continue
				}
				event.display = display
			}
			update.Display = append(update.Display, event.display)
			publish = true
		case eventDiagnostic:
			update.Diagnostics = append(update.Diagnostics, Diagnostic{Text: sanitizeDiagnostic(event.value)})
			publish = true
		case eventComponent:
			r.components[event.name] = event.value
			r.applyComponent(event)
		case eventProgress:
			if r.applyProgress(event.name, event.number) {
				publish = true
			}
		case eventResource:
			r.resources[event.name] = event.number
		case eventRoomImage:
			if event.value == "0" {
				r.pendingRoom.Image = ""
			} else {
				r.pendingRoom.Image = event.value
			}
		case eventRoundTime:
			r.public.Timers.Round = event.timestamp
			publish = true
		case eventCastTime:
			r.public.Timers.Cast = event.timestamp
			publish = true
		case eventSpellTime:
			spell := strings.TrimSpace(r.public.PreparedSpell)
			if spell != "" && !strings.EqualFold(spell, "none") {
				r.public.Timers.Spell = event.timestamp
				publish = true
			}
		case eventIndicator:
			name := strings.ToUpper(event.name)
			r.indicators[name] = event.flag
			if r.applyPosture(name) {
				publish = true
			}
		case eventCreatureStatus:
			r.creatureStatuses[event.name] = creatureStatus{hostile: event.flag, disengaged: event.flag2, flying: event.flag3}
		case eventHand:
			parts := strings.SplitN(event.aux, "\x00", 2)
			noun, exist := "", ""
			if len(parts) > 0 {
				noun = parts[0]
			}
			if len(parts) > 1 {
				exist = parts[1]
			}
			if event.name == "left" {
				r.public.Hands.Left, r.handNouns.Left, r.handExists.Left = event.value, noun, exist
			} else {
				r.public.Hands.Right, r.handNouns.Right, r.handExists.Right = event.value, noun, exist
			}
			publish = true
		case eventSpell:
			changed := !strings.EqualFold(r.public.PreparedSpell, event.value)
			r.public.PreparedSpell = event.value
			trimmed := strings.TrimSpace(event.value)
			if trimmed == "" || strings.EqualFold(trimmed, "none") {
				r.public.Timers.Spell = time.Time{}
			} else if changed || r.public.Timers.Spell.IsZero() {
				r.public.Timers.Spell = now
			}
			publish = true
		case eventCompass:
			r.pendingRoom.Compass = append([]string(nil), event.items...)
		case eventPrompt:
			r.public.Room = cloneRoom(r.pendingRoom)
			r.public.Prompt = event.value
			publish = true
		case eventSettingsInfo:
			if r.public.Connection != ConnectionReady {
				r.public.Connection = ConnectionReady
				r.armFlags(now)
				publish = true
			}
		case eventNav:
			if r.pendingRoom.ID != event.value {
				r.creatureStatuses = make(map[string]creatureStatus)
			}
			r.pendingRoom.ID = event.value
		case eventApp:
			if event.name != "" {
				r.public.Character = event.name
				publish = true
			}
		case eventContainer:
			r.containers[event.name] = event.value
			r.containerTargets[event.name] = event.aux
		case eventInjury:
			r.injuries[event.name] = injuryReading{kind: event.value, severity: event.number}
		}
	}
	if !publish {
		return Update{}, false
	}
	update.Snapshot = r.snapshot()
	update.Display = cloneDisplay(update.Display)
	return update, true
}

func (r *reducer) applyComponent(event protocolEvent) {
	id := strings.ToLower(event.name)
	if strings.HasPrefix(id, "exp ") {
		matches := rankPercentPattern.FindStringSubmatch(event.value)
		if len(matches) == 2 {
			if rank, err := strconv.Atoi(matches[1]); err == nil {
				name := []rune(strings.TrimSpace(event.name[4:]))
				if len(name) > 0 {
					name[0] = unicode.ToUpper(name[0])
					r.skills[string(name)] = rank
				}
			}
		}
		return
	}
	switch id {
	case "room title":
		if event.value != r.pendingRoom.Title {
			r.creatureStatuses = make(map[string]creatureStatus)
		}
		r.pendingRoom.Title = event.value
	case "room desc":
		r.pendingRoom.Description = event.value
	case "room exits":
		r.pendingRoom.Exits = splitRoomValues(event.value)
	case "room objs":
		r.pendingRoom.Objects = splitRoomValues(event.value)
		r.pendingRoom.Creatures = filterLiveCreatures(event.items)
	case "room players":
		r.pendingRoom.Players = splitRoomValues(event.value)
	case "pc name":
		r.public.Character = strings.TrimSpace(event.value)
	}
}

func (r *reducer) applyProgress(name string, value int) bool {
	switch strings.ToLower(name) {
	case "health", "health2":
		r.public.Vitals.Health = value
	case "mana", "mana2":
		r.public.Vitals.Mana = value
	case "stamina", "fatigue":
		r.public.Vitals.Stamina = value
	case "spirit":
		r.public.Vitals.Spirit = value
	case "concentration":
		r.public.Vitals.Concentration = value
	case "encumbrance":
		r.public.Vitals.Encumbrance = value
	default:
		return false
	}
	return true
}

func (r *reducer) applyPosture(name string) bool {
	switch name {
	case "ICONSTANDING", "ICONKNEELING", "ICONSITTING", "ICONPRONE":
	default:
		return false
	}
	posture := PostureUnknown
	switch {
	case r.indicators["ICONPRONE"]:
		posture = PostureProne
	case r.indicators["ICONSITTING"]:
		posture = PostureSitting
	case r.indicators["ICONKNEELING"]:
		posture = PostureKneeling
	case r.indicators["ICONSTANDING"]:
		posture = PostureStanding
	}
	changed := r.public.Posture != posture
	r.public.Posture = posture
	return changed
}

func (r *reducer) applyText(display DisplayEvent, now time.Time) (DisplayEvent, bool) {
	text := display.Text
	if r.flagsCapture.phase != captureInactive && display.Stream == "main" {
		if !now.Before(r.flagsCapture.deadline) || r.flagsCapture.lines >= 80 {
			r.flagsCapture = flagsCapture{}
		} else {
			r.flagsCapture.lines++
			if match := flagPattern.FindStringSubmatch(text); len(match) == 3 {
				if canonical, ok := knownFlags[strings.ToLower(match[1])]; ok {
					r.flags[canonical] = strings.EqualFold(match[2], "ON")
					r.flagsCapture.phase = captureStarted
					return display, true
				}
			}
			lower := strings.ToLower(strings.TrimSpace(text))
			if lower == "usage" || lower == "example" || strings.HasPrefix(lower, "flag ") || strings.HasPrefix(lower, "flag names may be abbreviated") || strings.HasPrefix(lower, "flag") && strings.Contains(lower, "status") && strings.Contains(lower, "behavior") {
				r.flagsCapture.phase = captureStarted
				return display, true
			}
			if strings.HasPrefix(lower, "for other setting options") {
				r.flagsCapture = flagsCapture{}
				return display, true
			}
			if r.flagsCapture.phase == captureStarted {
				r.flagsCapture = flagsCapture{}
			}
		}
	}

	if match := guildPattern.FindStringSubmatch(text); len(match) == 2 {
		r.guild = match[1]
	}
	if display.Stream == "main" && strings.HasPrefix(text, "You") {
		for phrase, reading := range map[string]injuryReading{
			"a case of uncontrollable convulsions":      {kind: "wound", severity: 3},
			"a case of sporadic convulsions":            {kind: "wound", severity: 2},
			"a strange case of muscle twitching":        {kind: "wound", severity: 1},
			"a very difficult time with muscle control": {kind: "scar", severity: 3},
			"constant muscle spasms":                    {kind: "scar", severity: 2},
			"developed slurred speech":                  {kind: "scar", severity: 1},
		} {
			if strings.Contains(text, phrase) {
				r.injuries["nsys"] = reading
			}
		}
	}

	if strings.Contains(text, "Showing all skills") {
		r.inExperience = true
	} else if r.inExperience && strings.Contains(text, "Total Ranks Displayed") {
		r.inExperience = false
	} else if r.inExperience {
		for _, match := range expPattern.FindAllStringSubmatch(text, -1) {
			if rank, err := strconv.Atoi(match[2]); err == nil {
				r.skills[match[1]] = rank
			}
		}
	}

	trimmed := strings.TrimLeft(text, " \t")
	if strings.HasPrefix(trimmed, "Listing all news items.") || strings.HasPrefix(trimmed, "ITEM # - HEADLINE") {
		r.newsCategory = ""
		return display, false
	}
	if strings.HasPrefix(trimmed, "Type NEWS HELP") || strings.HasPrefix(trimmed, "END NEWS ITEM") {
		r.newsCategory = ""
		return display, false
	}
	if match := newsCategoryPattern.FindStringSubmatch(trimmed); len(match) == 2 {
		r.newsCategory = match[1]
		return display, false
	}
	if r.newsCategory != "" {
		if match := newsItemPattern.FindStringSubmatchIndex(text); len(match) >= 4 {
			startBytes := match[2]
			endBytes := match[3]
			startRunes := utf8.RuneCountInString(text[:startBytes])
			item := text[startBytes:endBytes]
			display.Links = append(display.Links, LinkSpan{Span: Span{Start: startRunes, Length: utf8.RuneCountInString(text) - startRunes}, Target: "news " + r.newsCategory + " " + item})
		}
	}
	return display, false
}

func (r *reducer) armFlags(now time.Time) {
	r.flagsCapture = flagsCapture{phase: captureWaiting, deadline: now.Add(8 * time.Second)}
}

func (r *reducer) snapshot() Snapshot {
	return cloneSnapshot(r.public)
}

func (r *reducer) resetTransient(character string) {
	guild := r.guild
	skills := r.skills
	*r = *newReducer(character)
	r.guild = guild
	r.skills = skills
}

func cloneRoom(room Room) Room {
	room.Exits = append([]string(nil), room.Exits...)
	room.Objects = append([]string(nil), room.Objects...)
	room.Players = append([]string(nil), room.Players...)
	room.Creatures = append([]string(nil), room.Creatures...)
	room.Compass = append([]string(nil), room.Compass...)
	return room
}

func splitRoomValues(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	value = strings.ReplaceAll(value, "\n", ",")
	value = strings.ReplaceAll(value, " and ", ",")
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		part = strings.TrimSuffix(part, ".")
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func filterLiveCreatures(creatures []string) []string {
	var result []string
	for _, creature := range creatures {
		if !deadCreaturePattern.MatchString(creature) {
			result = append(result, creature)
		}
	}
	return result
}
