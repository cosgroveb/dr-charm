package dragonrealms

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestReducerPublishesSemanticSnapshot(t *testing.T) {
	t.Parallel()

	input := strings.Join([]string{
		"<streamWindow id='room' title='Room' subtitle=' - [Town Square] (12345)'/>",
		"<component id='room desc'>A broad square.</component>",
		"<component id='room exits'><d>north</d>, <d>east</d></component>",
		"<component id='room objs'><pushBold/>a goblin<popBold/>, a bench</component>",
		"<component id='room players'>Heroine</component>",
		"<compass><dir value='north'/><dir value='east'/></compass>",
		"<resource picture='42'/>",
		"<progressBar id='health' value='87'/><progressBar id='mana' value='66'/>",
		"<progressBar id='stamina' value='55'/><progressBar id='spirit' value='44'/>",
		"<progressBar id='concentration' value='33'/><progressBar id='encumbrance' value='22'/>",
		"<roundTime value='1710000000'/><castTime value='1710000001'/>",
		"<left noun='blade' exist='1'>a blade</left><right>Empty</right>",
		"<spell>Fire Ball</spell><spellTime value='1710000002'/>",
		"<indicator id='IconSTANDING' visible='y'/>",
		"<crtrStatus exist='9' hostile='1' disengaged='0' flying='1'/>",
		"<prompt time='1710000003'>&gt;</prompt>",
	}, "")

	reducer := newReducer("Hero")
	var last Update
	for _, action := range newStreamDecoder().feed([]byte(input)) {
		if update, publish := reducer.apply(action); publish {
			last = update
		}
	}
	wantTime := time.Unix(1710000000, 0)
	if last.Snapshot.Character != "Hero" || last.Snapshot.Room.ID != "12345" || last.Snapshot.Room.Title != "[Town Square]" || last.Snapshot.Room.Description != "A broad square." {
		t.Fatalf("snapshot room = %#v", last.Snapshot)
	}
	if len(last.Snapshot.Room.Exits) != 2 || len(last.Snapshot.Room.Compass) != 2 || last.Snapshot.Room.Image != "42" || len(last.Snapshot.Room.Creatures) != 1 {
		t.Fatalf("room details = %#v", last.Snapshot.Room)
	}
	if last.Snapshot.Vitals != (Vitals{Health: 87, Mana: 66, Stamina: 55, Spirit: 44, Concentration: 33, Encumbrance: 22}) {
		t.Fatalf("vitals = %#v", last.Snapshot.Vitals)
	}
	if !last.Snapshot.Timers.Round.Equal(wantTime) || last.Snapshot.Hands.Left != "a blade" || last.Snapshot.PreparedSpell != "Fire Ball" || last.Snapshot.Posture != PostureStanding || last.Snapshot.Prompt != ">" {
		t.Fatalf("semantic state = %#v", last.Snapshot)
	}
	status := reducer.creatureStatuses["9"]
	if !status.hostile || status.disengaged || !status.flying {
		t.Fatalf("creature status = %#v", status)
	}
}

func TestReducerPublishesPromptEventOnce(t *testing.T) {
	t.Parallel()

	reducer := newReducer("Hero")
	prompt, published := reducer.apply(protocolAction{events: []protocolEvent{{kind: eventPrompt, value: ">"}}})
	if !published || !prompt.Prompted || prompt.Snapshot.Prompt != ">" {
		t.Fatalf("prompt update=%+v published=%v", prompt, published)
	}
	later, published := reducer.apply(protocolAction{events: []protocolEvent{{kind: eventRoundTime, timestamp: time.Unix(1, 0)}}})
	if !published || later.Prompted || later.Snapshot.Prompt != ">" {
		t.Fatalf("later update=%+v published=%v", later, published)
	}
}

func TestReducerKeepsPrivateProtocolState(t *testing.T) {
	t.Parallel()

	reducer := newReducer("Hero")
	input := "<component id='exp Arcana'>Arcana: 123 45%</component><resource id='mana' value='400'/><container id='stow' title='Backpack' target='#1'/><dialogData id='injuries'><image id='head' name='Injury2'/></dialogData>"
	for _, action := range newStreamDecoder().feed([]byte(input)) {
		reducer.apply(action)
	}
	if reducer.components["exp Arcana"] == "" || reducer.resources["mana"] != 400 || reducer.containers["stow"] != "Backpack" || reducer.skills["Arcana"] != 123 || reducer.injuries["head"].severity != 2 {
		t.Fatalf("private state not reduced: %#v", reducer)
	}
}

func TestReducerTextStateMachines(t *testing.T) {
	t.Parallel()

	reducer := newReducer("Hero")
	reducer.armFlags(time.Unix(100, 0))
	input := strings.Join([]string{
		"LogOn ON\n",
		"RoomNames OFF\n",
		"For other setting options use HELP\n",
		"Name: Hero Race: Human Guild: Moon Mage\n",
		"You have a strange case of muscle twitching.\n",
		"Showing all skills\n",
		"Arcana: 123 45%  Athletics: 55 10%\n",
		"Total Ranks Displayed: 178\n",
	}, "")
	for _, action := range newStreamDecoder().feed([]byte(input)) {
		reducer.applyAt(action, time.Unix(101, 0))
	}
	if !reducer.flags["LogOn"] || reducer.flags["RoomNames"] || reducer.guild != "Moon Mage" || reducer.injuries["nsys"].severity != 1 || reducer.skills["Arcana"] != 123 || reducer.skills["Athletics"] != 55 {
		t.Fatalf("text-derived state = flags %#v guild %q injuries %#v skills %#v", reducer.flags, reducer.guild, reducer.injuries, reducer.skills)
	}
}

func TestReducerInvalidTimestampAndDetachedSnapshot(t *testing.T) {
	t.Parallel()

	reducer := newReducer("Hero")
	actions := newStreamDecoder().feed([]byte("<roundTime value='999999999999999999999'/><component id='room exits'><d>north</d></component><prompt time='1'>&gt;</prompt>"))
	var update Update
	sawDiagnostic := false
	for _, action := range actions {
		if got, publish := reducer.apply(action); publish {
			update = got
			if len(got.Diagnostics) > 0 {
				sawDiagnostic = true
			}
		}
	}
	if !update.Snapshot.Timers.Round.IsZero() || !sawDiagnostic {
		t.Fatalf("invalid timestamp result = %#v", update)
	}
	update.Snapshot.Room.Exits[0] = "mutated"
	if reducer.snapshot().Room.Exits[0] != "north" {
		t.Fatal("published snapshot aliases reducer storage")
	}
}

func TestRoomChangeClearsCreatureStatus(t *testing.T) {
	t.Parallel()

	reducer := newReducer("Hero")
	for _, action := range newStreamDecoder().feed([]byte("<crtrStatus exist='9' hostile='1'/><nav rm='1'/>")) {
		reducer.apply(action)
	}
	if len(reducer.creatureStatuses) != 0 {
		t.Fatal("room change retained creature status")
	}
}

func TestReducerResetTransientReconstructsFreshState(t *testing.T) {
	t.Parallel()

	stamp := time.Unix(100, 0)
	reducer := newReducer("Old Hero")
	reducer.public = Snapshot{
		Connection: ConnectionReady,
		Character:  "Old Hero",
		Room: Room{
			ID: "1", Title: "Old Room", Description: "Old description",
			Exits: []string{"north"}, Objects: []string{"bench"}, Players: []string{"Friend"},
			Creatures: []string{"goblin"}, Compass: []string{"north"}, Image: "42",
		},
		Vitals:        Vitals{Health: 1, Mana: 2, Stamina: 3, Spirit: 4, Concentration: 5, Encumbrance: 6},
		Timers:        Timers{Round: stamp, Cast: stamp.Add(time.Second), Spell: stamp.Add(2 * time.Second)},
		Hands:         Hands{Left: "blade", Right: "shield"},
		PreparedSpell: "Fire Ball",
		Posture:       PostureProne,
		Prompt:        ">",
	}
	reducer.pendingRoom = Room{
		ID: "2", Title: "Pending", Description: "Pending description",
		Exits: []string{"south"}, Objects: []string{"rock"}, Players: []string{"Other"},
		Creatures: []string{"rat"}, Compass: []string{"south"}, Image: "7",
	}
	reducer.components["room title"] = "Old Room"
	reducer.resources["mana"] = 100
	reducer.indicators["ICONSTANDING"] = true
	reducer.creatureStatuses["1"] = creatureStatus{hostile: true}
	reducer.injuries["head"] = injuryReading{kind: "wound", severity: 2}
	reducer.containers["stow"] = "Backpack"
	reducer.containerTargets["stow"] = "#1"
	reducer.skills["Arcana"] = 123
	reducer.flags["LogOn"] = true
	reducer.guild = "Moon Mage"
	reducer.handNouns = Hands{Left: "blade", Right: "shield"}
	reducer.handExists = Hands{Left: "1", Right: "2"}
	reducer.flagsCapture = flagsCapture{phase: captureStarted, deadline: stamp, lines: 12}
	reducer.inExperience = true
	reducer.newsCategory = "9"
	skills := reducer.skills
	skillsPointer := reflect.ValueOf(skills).Pointer()

	reducer.resetTransient("New Hero")

	wantSnapshot := Snapshot{Connection: ConnectionConnected, Character: "New Hero"}
	if got := reducer.snapshot(); !reflect.DeepEqual(got, wantSnapshot) {
		t.Fatalf("reset snapshot = %#v, want %#v", got, wantSnapshot)
	}
	if !reflect.DeepEqual(reducer.pendingRoom, Room{}) {
		t.Fatalf("pending room survived reset: %#v", reducer.pendingRoom)
	}
	if reducer.guild != "Moon Mage" || reducer.skills["Arcana"] != 123 || reflect.ValueOf(reducer.skills).Pointer() != skillsPointer {
		t.Fatalf("durable state = guild %q skills %#v", reducer.guild, reducer.skills)
	}
	if reducer.components == nil || len(reducer.components) != 0 ||
		reducer.resources == nil || len(reducer.resources) != 0 ||
		reducer.indicators == nil || len(reducer.indicators) != 0 ||
		reducer.creatureStatuses == nil || len(reducer.creatureStatuses) != 0 ||
		reducer.injuries == nil || len(reducer.injuries) != 0 ||
		reducer.containers == nil || len(reducer.containers) != 0 ||
		reducer.containerTargets == nil || len(reducer.containerTargets) != 0 ||
		reducer.flags == nil || len(reducer.flags) != 0 {
		t.Fatalf("transient maps were not replaced with empty maps: %#v", reducer)
	}
	if reducer.handNouns != (Hands{}) || reducer.handExists != (Hands{}) || reducer.flagsCapture != (flagsCapture{}) || reducer.inExperience || reducer.newsCategory != "" {
		t.Fatalf("private transient state survived reset: %#v", reducer)
	}
}

func TestReducerPublishesReadinessOnce(t *testing.T) {
	t.Parallel()

	reducer := newReducer("Hero")
	action := protocolAction{events: []protocolEvent{{kind: eventSettingsInfo}}}
	update, publish := reducer.apply(action)
	if !publish || update.Snapshot.Connection != ConnectionReady {
		t.Fatalf("first settings info = update %#v, publish %v", update, publish)
	}
	if update, publish := reducer.apply(action); publish {
		t.Fatalf("duplicate settings info published %#v", update)
	}
}

func TestRoomTitlePreservesFieldsAlreadyPendingInBurst(t *testing.T) {
	t.Parallel()

	reducer := newReducer("Hero")
	input := "<component id='room desc'>Description first.</component><component id='room exits'>north</component><component id='room title'>[Title Last]</component><prompt time='1'>&gt;</prompt>"
	var update Update
	for _, action := range newStreamDecoder().feed([]byte(input)) {
		if got, publish := reducer.apply(action); publish {
			update = got
		}
	}
	if update.Snapshot.Room.Title != "[Title Last]" || update.Snapshot.Room.Description != "Description first." || !strings.EqualFold(strings.Join(update.Snapshot.Room.Exits, ","), "north") {
		t.Fatalf("committed room = %#v", update.Snapshot.Room)
	}
}

func TestRoomCreaturesExcludeDefaultDeadPatterns(t *testing.T) {
	t.Parallel()

	reducer := newReducer("Hero")
	input := "<component id='room objs'><pushBold/>a kobold<popBold/> that appears dead, <pushBold/>a rat (dead)<popBold/>, <pushBold/>a LIVE viper<popBold/>.</component><prompt time='1'>&gt;</prompt>"
	var update Update
	for _, action := range newStreamDecoder().feed([]byte(input)) {
		if got, publish := reducer.apply(action); publish {
			update = got
		}
	}
	if !reflect.DeepEqual(update.Snapshot.Room.Creatures, []string{"a LIVE viper"}) {
		t.Fatalf("public creatures = %#v", update.Snapshot.Room.Creatures)
	}
}

func TestPostureRecomputesFromRecognizedIndicatorFlags(t *testing.T) {
	t.Parallel()

	reducer := newReducer("Hero")
	apply := func(name string, visible bool) Posture {
		t.Helper()
		update, publish := reducer.apply(protocolAction{events: []protocolEvent{{kind: eventIndicator, name: name, flag: visible}}})
		if !publish {
			t.Fatalf("indicator %s=%v did not publish", name, visible)
		}
		return update.Snapshot.Posture
	}
	if got := apply("IconSTANDING", true); got != PostureStanding {
		t.Fatalf("standing posture = %v", got)
	}
	if got := apply("IconKNEELING", true); got != PostureKneeling {
		t.Fatalf("kneeling posture = %v", got)
	}
	if got := apply("IconKNEELING", false); got != PostureStanding {
		t.Fatalf("posture after kneeling cleared = %v", got)
	}
	if got := apply("IconSTANDING", false); got != PostureUnknown {
		t.Fatalf("posture after all recognized flags cleared = %v", got)
	}
	if _, publish := reducer.apply(protocolAction{events: []protocolEvent{{kind: eventIndicator, name: "IconHIDDEN", flag: true}}}); publish {
		t.Fatal("unrecognized posture indicator published a posture change")
	}
}

func TestExperienceComponentUsesIDAndFirstRankPercentPair(t *testing.T) {
	t.Parallel()

	reducer := newReducer("Hero")
	reducer.apply(protocolAction{events: []protocolEvent{{kind: eventComponent, name: "exp arcana", value: "token9 99%, learning 321 45%, historical 999 99%"}}})
	if reducer.skills["Arcana"] != 321 {
		t.Fatalf("Arcana rank = %d, want 321", reducer.skills["Arcana"])
	}
	if _, exists := reducer.skills["arcana"]; exists {
		t.Fatal("experience component retained lower-case protocol skill name")
	}
}

func TestPreparedSpellTimerLifecycle(t *testing.T) {
	t.Parallel()

	reducer := newReducer("Hero")
	applySpell := func(name string, now time.Time) Snapshot {
		t.Helper()
		update, publish := reducer.applyAt(protocolAction{events: []protocolEvent{{kind: eventSpell, value: name}}}, now)
		if !publish {
			t.Fatalf("spell %q did not publish", name)
		}
		return update.Snapshot
	}

	first := time.Unix(100, 0)
	if snapshot := applySpell("Fire Ball", first); snapshot.PreparedSpell != "Fire Ball" || !snapshot.Timers.Spell.Equal(first) {
		t.Fatalf("first spell snapshot = %#v", snapshot)
	}
	duplicate := time.Unix(200, 0)
	if snapshot := applySpell("fire ball", duplicate); snapshot.PreparedSpell != "fire ball" || !snapshot.Timers.Spell.Equal(first) {
		t.Fatalf("case-only duplicate snapshot = %#v", snapshot)
	}
	changed := time.Unix(300, 0)
	if snapshot := applySpell("Ice Patch", changed); !snapshot.Timers.Spell.Equal(changed) {
		t.Fatalf("changed spell snapshot = %#v", snapshot)
	}

	serverTime := time.Unix(250, 0)
	update, publish := reducer.applyAt(protocolAction{events: []protocolEvent{{kind: eventSpellTime, timestamp: serverTime}}}, time.Unix(400, 0))
	if !publish || !update.Snapshot.Timers.Spell.Equal(serverTime) {
		t.Fatalf("server spell time snapshot = %#v, publish %v", update.Snapshot, publish)
	}
	if snapshot := applySpell("None", time.Unix(500, 0)); !snapshot.Timers.Spell.IsZero() {
		t.Fatalf("cleared spell snapshot = %#v", snapshot)
	}
	if update, publish := reducer.applyAt(protocolAction{events: []protocolEvent{{kind: eventSpellTime, timestamp: time.Unix(600, 0)}}}, time.Unix(700, 0)); publish || !reducer.snapshot().Timers.Spell.IsZero() {
		t.Fatalf("unheld spell-time update = %#v, publish %v", update, publish)
	}
	if snapshot := applySpell("", time.Unix(800, 0)); !snapshot.Timers.Spell.IsZero() {
		t.Fatalf("empty spell snapshot = %#v", snapshot)
	}
}

func TestFlagsCaptureWaitsForMainStreamReportAndStopsAfterItStarts(t *testing.T) {
	t.Parallel()

	now := time.Unix(100, 0)
	reducer := newReducer("Hero")
	update, publish := reduceTextAt(reducer, "main", "LogOn ON", now)
	if !publish || len(update.Display) != 1 || reducer.flags["LogOn"] {
		t.Fatalf("inactive capture changed input: update %#v, flags %#v", update, reducer.flags)
	}

	reducer.armFlags(now)
	update, publish = reduceTextAt(reducer, "main", "A bell rings.", now.Add(time.Second))
	if !publish || len(update.Display) != 1 {
		t.Fatalf("waiting capture hid ordinary main line: %#v", update)
	}

	update, publish = reduceTextAt(reducer, "familiar", "LogOn ON", now.Add(2*time.Second))
	if !publish || len(update.Display) != 1 || len(reducer.flags) != 0 {
		t.Fatalf("non-main stream entered flags report: update %#v, flags %#v", update, reducer.flags)
	}

	for _, line := range []string{"Usage", "LogOn ON", "RoomNames OFF"} {
		update, publish = reduceTextAt(reducer, "main", line, now.Add(3*time.Second))
		if publish || len(update.Display) != 0 {
			t.Fatalf("flags report line %q was displayed: %#v", line, update)
		}
	}
	if !reducer.flags["LogOn"] || reducer.flags["RoomNames"] {
		t.Fatalf("captured flags = %#v", reducer.flags)
	}

	update, publish = reduceTextAt(reducer, "main", "A bell rings.", now.Add(4*time.Second))
	if !publish || len(update.Display) != 1 {
		t.Fatalf("first post-report line was not visible: %#v", update)
	}
	update, publish = reduceTextAt(reducer, "main", "RoomNames ON", now.Add(5*time.Second))
	if !publish || len(update.Display) != 1 || reducer.flags["RoomNames"] {
		t.Fatalf("post-report input remained captured: update %#v, flags %#v", update, reducer.flags)
	}
}

func TestFlagsCaptureRecognizesEveryFlagAndHonorsDeadline(t *testing.T) {
	t.Parallel()

	now := time.Unix(100, 0)
	reducer := newReducer("Hero")
	reducer.armFlags(now)
	for lower, canonical := range knownFlags {
		line := canonical + " ON"
		if _, publish := reduceTextAt(reducer, "main", line, now.Add(time.Second)); publish {
			t.Fatalf("known flag %q was displayed", canonical)
		}
		if !reducer.flags[canonical] || strings.ToLower(canonical) != lower {
			t.Fatalf("known flag %q was not captured", canonical)
		}
	}

	reducer.flags["RoomNames"] = false
	reducer.armFlags(now)
	update, publish := reduceTextAt(reducer, "main", "LogOn OFF", now.Add(8*time.Second))
	if !publish || len(update.Display) != 1 || !reducer.flags["LogOn"] {
		t.Fatalf("deadline did not release line unchanged: update %#v, flags %#v", update, reducer.flags)
	}
	update, publish = reduceTextAt(reducer, "main", "RoomNames ON", now.Add(time.Second))
	if !publish || len(update.Display) != 1 || reducer.flags["RoomNames"] {
		t.Fatalf("deadline stop did not remain inactive: update %#v, flags %#v", update, reducer.flags)
	}
}

func TestFlagsCaptureStopsAtLineLimit(t *testing.T) {
	t.Parallel()

	now := time.Unix(100, 0)
	reducer := newReducer("Hero")
	reducer.armFlags(now)
	for line := 1; line <= 80; line++ {
		update, publish := reduceTextAt(reducer, "main", "ordinary", now.Add(time.Second))
		if !publish || len(update.Display) != 1 {
			t.Fatalf("line %d was hidden: %#v", line, update)
		}
	}
	update, publish := reduceTextAt(reducer, "main", "LogOn ON", now.Add(time.Second))
	if !publish || len(update.Display) != 1 || reducer.flags["LogOn"] {
		t.Fatalf("line 81 remained captured: update %#v, flags %#v", update, reducer.flags)
	}
	update, publish = reduceTextAt(reducer, "main", "RoomNames ON", now.Add(time.Second))
	if !publish || len(update.Display) != 1 || reducer.flags["RoomNames"] {
		t.Fatalf("line-limit stop did not remain inactive: update %#v, flags %#v", update, reducer.flags)
	}
}

func TestFlagsCaptureFooterStopsCapture(t *testing.T) {
	t.Parallel()

	now := time.Unix(100, 0)
	reducer := newReducer("Hero")
	reducer.armFlags(now)
	if _, publish := reduceTextAt(reducer, "main", "Usage", now.Add(time.Second)); publish {
		t.Fatal("flags header was displayed")
	}
	if _, publish := reduceTextAt(reducer, "main", "For other setting options use HELP", now.Add(2*time.Second)); publish {
		t.Fatal("flags footer was displayed")
	}
	update, publish := reduceTextAt(reducer, "main", "LogOn ON", now.Add(3*time.Second))
	if !publish || len(update.Display) != 1 || reducer.flags["LogOn"] {
		t.Fatalf("footer did not stop capture: update %#v, flags %#v", update, reducer.flags)
	}
}

func TestFlagsCaptureStartsOnRecognizedFlag(t *testing.T) {
	t.Parallel()

	now := time.Unix(100, 0)
	reducer := newReducer("Hero")
	reducer.armFlags(now)
	if _, publish := reduceTextAt(reducer, "main", "LogOn ON", now.Add(time.Second)); publish {
		t.Fatal("recognized flag was displayed")
	}
	update, publish := reduceTextAt(reducer, "main", "ordinary", now.Add(2*time.Second))
	if !publish || len(update.Display) != 1 {
		t.Fatalf("first post-flag line was not visible: %#v", update)
	}
	update, publish = reduceTextAt(reducer, "main", "RoomNames ON", now.Add(3*time.Second))
	if !publish || len(update.Display) != 1 || reducer.flags["RoomNames"] {
		t.Fatalf("recognized flag did not start then stop capture: update %#v, flags %#v", update, reducer.flags)
	}
}

func reduceTextAt(reducer *reducer, stream, text string, now time.Time) (Update, bool) {
	return reducer.applyAt(protocolAction{events: []protocolEvent{{
		kind: eventDisplay,
		display: DisplayEvent{
			Kind:   DisplayText,
			Stream: stream,
			Text:   text,
		},
	}}}, now)
}

func TestNervousSystemPhrasesRequireMainStream(t *testing.T) {
	t.Parallel()

	tests := []struct {
		phrase   string
		kind     string
		severity int
	}{
		{phrase: "a case of uncontrollable convulsions", kind: "wound", severity: 3},
		{phrase: "a case of sporadic convulsions", kind: "wound", severity: 2},
		{phrase: "a strange case of muscle twitching", kind: "wound", severity: 1},
		{phrase: "a very difficult time with muscle control", kind: "scar", severity: 3},
		{phrase: "constant muscle spasms", kind: "scar", severity: 2},
		{phrase: "developed slurred speech", kind: "scar", severity: 1},
	}
	for _, tt := range tests {
		reducer := newReducer("Hero")
		line := "You have " + tt.phrase + "."
		reducer.apply(protocolAction{events: []protocolEvent{{kind: eventDisplay, display: DisplayEvent{Kind: DisplayText, Stream: "familiar", Text: line}}}})
		if _, ok := reducer.injuries["nsys"]; ok {
			t.Fatalf("familiar phrase %q changed nerve state", tt.phrase)
		}
		reducer.apply(protocolAction{events: []protocolEvent{{kind: eventDisplay, display: DisplayEvent{Kind: DisplayText, Stream: "main", Text: line}}}})
		if got := reducer.injuries["nsys"]; got != (injuryReading{kind: tt.kind, severity: tt.severity}) {
			t.Fatalf("phrase %q = %#v", tt.phrase, got)
		}
	}
}

func TestNewsLinksResetOnEntryAndUseRuneOffsets(t *testing.T) {
	t.Parallel()

	reducer := newReducer("Hero")
	reducer.newsCategory = "9"
	lines := []string{"Listing all news items.", "  1 - stale category", "** Category 12 - Events **", "  23 - Festivál", "END NEWS ITEM"}
	var updates []Update
	for _, line := range lines {
		if update, publish := reducer.apply(protocolAction{events: []protocolEvent{{kind: eventDisplay, display: DisplayEvent{Kind: DisplayText, Stream: "main", Text: line}}}}); publish {
			updates = append(updates, update)
		}
	}
	if len(updates) != len(lines) {
		t.Fatalf("news display count = %d", len(updates))
	}
	if len(updates[1].Display[0].Links) != 0 {
		t.Fatalf("entry did not clear stale category: %#v", updates[1])
	}
	link := updates[3].Display[0].Links
	if len(link) != 1 || link[0].Start != 2 || link[0].Length != 13 || link[0].Target != "news 12 23" {
		t.Fatalf("news link = %#v", link)
	}
	if reducer.newsCategory != "" {
		t.Fatalf("news exit retained category %q", reducer.newsCategory)
	}
}

func TestNewsEntryPrefixesClearStaleCategory(t *testing.T) {
	t.Parallel()

	for _, header := range []string{
		"Listing all news items. (current)",
		"ITEM # - HEADLINE (continued)",
	} {
		reducer := newReducer("Hero")
		reducer.newsCategory = "9"
		reducer.apply(protocolAction{events: []protocolEvent{{kind: eventDisplay, display: DisplayEvent{Kind: DisplayText, Stream: "main", Text: header}}}})
		update, publish := reducer.apply(protocolAction{events: []protocolEvent{{kind: eventDisplay, display: DisplayEvent{Kind: DisplayText, Stream: "main", Text: "  1 - must not use stale category"}}}})
		if !publish || len(update.Display) != 1 {
			t.Fatalf("numbered line after %q did not publish: %#v", header, update)
		}
		if reducer.newsCategory != "" || len(update.Display[0].Links) != 0 {
			t.Fatalf("header %q retained stale category: category %q links %#v", header, reducer.newsCategory, update.Display[0].Links)
		}
	}
}
