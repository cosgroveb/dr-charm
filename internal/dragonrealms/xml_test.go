package dragonrealms

import (
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestXMLTagDisposition(t *testing.T) {
	t.Parallel()

	textEvent := func(stream, text string) protocolEvent {
		return protocolEvent{kind: eventDisplay, display: DisplayEvent{
			Kind: DisplayText, Stream: stream, Text: text,
			Links: []LinkSpan{}, Bold: []Span{}, Presets: []PresetSpan{},
		}}
	}
	site := textEvent("main", "site")
	site.display.Links = []LinkSpan{{Span: Span{Start: 0, Length: 4}, Target: "https://example.invalid", URL: true}}
	bold := textEvent("main", "bold")
	bold.display.Bold = []Span{{Start: 0, Length: 4}}
	direction := textEvent("main", "north")
	direction.display.Links = []LinkSpan{{Span: Span{Start: 0, Length: 5}, Target: "north"}}
	mono := textEvent("main", "mono")
	mono.display.Mono = true
	preset := textEvent("main", "text")
	preset.display.Presets = []PresetSpan{{Span: Span{Start: 0, Length: 4}, ID: "speech"}}
	styled := textEvent("main", "text")
	styled.display.Presets = []PresetSpan{{Span: Span{Start: 0, Length: 4}, ID: "roomName"}}

	handled := []struct {
		name  string
		input string
		want  []protocolEvent
	}{
		{name: "a", input: "<a href='https://example.invalid'>site</a>\n", want: []protocolEvent{site}},
		{name: "app", input: "<app char='Hero'/>", want: []protocolEvent{{kind: eventApp, name: "Hero"}}},
		{name: "b", input: "<b>bold</b>\n", want: []protocolEvent{bold}},
		{name: "casttime", input: "<castTime value='1'/>", want: []protocolEvent{{kind: eventCastTime, timestamp: time.Unix(1, 0)}}},
		{name: "cleardynastream", input: "<clearDynaStream id='spellInfo'/>", want: []protocolEvent{{kind: eventDisplay, display: DisplayEvent{Kind: DisplayClear, Stream: "spellInfo", ID: "spellInfo"}}}},
		{name: "clearstream", input: "<clearStream id='familiar'/>", want: []protocolEvent{{kind: eventDisplay, display: DisplayEvent{Kind: DisplayClear, Stream: "familiar", ID: "familiar"}}}},
		{name: "compass", input: "<compass/>", want: []protocolEvent{{kind: eventCompass}}},
		{name: "component", input: "<component id='room title'>Square</component>", want: []protocolEvent{{kind: eventComponent, name: "room title", value: "Square"}}},
		{name: "container", input: "<container id='stow' target='#1'/>", want: []protocolEvent{{kind: eventContainer, name: "stow", aux: "#1"}}},
		{name: "crtrstatus", input: "<crtrStatus exist='1'/>", want: []protocolEvent{{kind: eventCreatureStatus, name: "1"}}},
		{name: "d", input: "<d cmd='north'>north</d>\n", want: []protocolEvent{direction}},
		{name: "dialogdata", input: "<dialogData id='injuries'><image id='head' name='Injury1'/></dialogData>", want: []protocolEvent{{kind: eventInjury, name: "head", value: "wound", number: 1}}},
		{name: "dir", input: "<compass><dir value='north'/></compass>", want: []protocolEvent{{kind: eventCompass, items: []string{"north"}}}},
		{name: "endsetup", input: "<endSetup/>", want: []protocolEvent{{kind: eventEndSetup}}},
		{name: "image", input: "<image id='head' name='head'/>", want: nil},
		{name: "indicator", input: "<indicator id='IconSTANDING' visible='y'/>", want: []protocolEvent{{kind: eventIndicator, name: "IconSTANDING", flag: true}}},
		{name: "inv", input: "<inv>item</inv>", want: []protocolEvent{textEvent("inv", "item")}},
		{name: "left", input: "<left>item</left>", want: []protocolEvent{{kind: eventHand, name: "left", value: "item", aux: "\x00"}}},
		{name: "nav", input: "<nav rm='1'/>", want: []protocolEvent{{kind: eventNav, value: "1"}}},
		{name: "openwindow", input: "<openWindow id='main'/>", want: []protocolEvent{{kind: eventDisplay, display: DisplayEvent{Kind: DisplayWindow, ID: "main"}}}},
		{name: "output", input: "<output class='mono'/>mono\n", want: []protocolEvent{mono}},
		{name: "popbold", input: "<pushBold/>bold<popBold/>\n", want: []protocolEvent{bold}},
		{name: "popstream", input: "<pushStream id='talk'/>text\n<popStream/>", want: []protocolEvent{textEvent("talk", "text")}},
		{name: "preset", input: "<preset id='speech'>text</preset>\n", want: []protocolEvent{preset}},
		{name: "progressbar", input: "<progressBar id='health' value='1'/>", want: []protocolEvent{{kind: eventProgress, name: "health", number: 1}}},
		{name: "prompt", input: "<prompt time='1'>&gt;</prompt>", want: []protocolEvent{{kind: eventPrompt, value: ">", timestamp: time.Unix(1, 0)}}},
		{name: "pushbold", input: "<pushBold/>bold<popBold/>\n", want: []protocolEvent{bold}},
		{name: "pushstream", input: "<pushStream id='talk'/>text\n<popStream/>", want: []protocolEvent{textEvent("talk", "text")}},
		{name: "resource", input: "<resource id='mana' value='1'/>", want: []protocolEvent{{kind: eventResource, name: "mana", number: 1}}},
		{name: "right", input: "<right>item</right>", want: []protocolEvent{{kind: eventHand, name: "right", value: "item", aux: "\x00"}}},
		{name: "roundtime", input: "<roundTime value='1'/>", want: []protocolEvent{{kind: eventRoundTime, timestamp: time.Unix(1, 0)}}},
		{name: "settingsinfo", input: "<settingsInfo/>", want: []protocolEvent{{kind: eventSettingsInfo}}},
		{name: "spell", input: "<spell>Fire Ball</spell>", want: []protocolEvent{{kind: eventSpell, value: "Fire Ball"}}},
		{name: "spelltime", input: "<spellTime value='1'/>", want: []protocolEvent{{kind: eventSpellTime, timestamp: time.Unix(1, 0)}}},
		{name: "streamwindow", input: "<streamWindow id='main'/>", want: []protocolEvent{{kind: eventDisplay, display: DisplayEvent{Kind: DisplayWindow, ID: "main"}}}},
		{name: "style", input: "<style id='roomName'/>text<style id=''/>\n", want: []protocolEvent{styled}},
	}
	for _, tt := range handled {
		got := decodeEvents(tt.input)
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("handled tag %s events = %#v, want %#v", tt.name, got, tt.want)
		}
	}

	droppedData := map[string]bool{
		"skin": true, "compdef": true, "opendialog": true, "radio": true, "detach": true,
		"playerid": true, "exposecontainer": true, "clearcontainer": true, "menuimage": true,
		"closedialog": true, "exposedialog": true, "menulink": true, "label": true,
		"cmdbutton": true, "closebutton": true, "checkbox": true, "streambox": true,
		"dropdownbox": true, "editbox": true, "updowneditbox": true,
	}
	skipped := strings.Fields("mode settings presets p macros keys k palette i stream w cmdline strings names ignores vars scripts dialog builtin panels group toggles misc m display options o font s playerid opendialog detach skin radio menuimage closedialog exposedialog label cmdbutton closebutton checkbox streambox dropdownbox editbox updowneditbox switchquickbar link menulink forging exposecontainer clearcontainer compdef")
	for _, name := range skipped {
		events := decodeEvents("<" + name + ">body</" + name + ">\n")
		want := "dropped settings"
		if droppedData[name] {
			want = "dropped data"
		}
		if !hasDiagnostic(events, want) {
			t.Errorf("skipped tag %s lacks %q diagnostic: %#v", name, want, events)
		}
		if !hasText(events, "body") {
			t.Errorf("skipped tag %s swallowed body text", name)
		}
	}

	for _, tt := range []struct {
		name       string
		input      string
		diagnostic string
	}{
		{name: "mixed-case dropped settings", input: "<MoDe>body</mOdE>\n", diagnostic: "dropped settings tag: mode"},
		{name: "mixed-case dropped data", input: "<SkIn>body</sKiN>\n", diagnostic: "dropped data tag: skin"},
		{name: "mixed-case unknown open and close", input: "<MyStErY>body</mYsTeRy>\n", diagnostic: "unknown DragonRealms tag: mystery"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			events := decodeEvents(tt.input)
			if !hasDiagnostic(events, tt.diagnostic) {
				t.Fatalf("events lack %q: %#v", tt.diagnostic, events)
			}
			if !hasText(events, "body") {
				t.Fatalf("tag swallowed body text: %#v", events)
			}
			diagnostics := 0
			for _, event := range events {
				if event.kind == eventDiagnostic && strings.Contains(event.value, tt.diagnostic) {
					diagnostics++
				}
			}
			if diagnostics != 1 {
				t.Fatalf("matching diagnostics = %d: %#v", diagnostics, events)
			}
		})
	}
}

func TestXMLStreamingIsIndependentOfByteSplits(t *testing.T) {
	t.Parallel()

	input := "<pushStream id='talk'/>Jalapeño says, <b>\"hi\"</b>.\n<popStream/><component id='room title'>[The Plaza]</component><prompt time='1710000000'>&gt;</prompt>"
	want := decodeEvents(input)
	for split := 0; split <= len(input); split++ {
		decoder := newStreamDecoder()
		var got []protocolEvent
		for _, chunk := range [][]byte{[]byte(input[:split]), []byte(input[split:])} {
			got = appendActionEvents(got, decoder.feed(chunk))
		}
		if diff := eventsDifference(want, got); diff != "" {
			t.Fatalf("split %d: %s", split, diff)
		}
	}

	decoder := newStreamDecoder()
	var got []protocolEvent
	for i := 0; i < len(input); i++ {
		got = appendActionEvents(got, decoder.feed([]byte{input[i]}))
	}
	if diff := eventsDifference(want, got); diff != "" {
		t.Fatal(diff)
	}
}

func TestXMLLiteralAnglesQuotedGreaterThanAndEOF(t *testing.T) {
	t.Parallel()

	events := decodeEvents("Range <1-20>, I <3 you, a < b, and <>.\n<d cmd='say > now'>label</d>\n")
	if !hasText(events, "Range <1-20>, I <3 you, a < b, and <>.") {
		t.Fatalf("literal angle text missing: %#v", events)
	}
	var linked DisplayEvent
	for _, event := range events {
		if event.kind == eventDisplay && strings.Contains(event.display.Text, "label") {
			linked = event.display
		}
	}
	if len(linked.Links) != 1 || linked.Links[0].Target != "say > now" {
		t.Fatalf("quoted > ended tag early: %#v", linked)
	}

	decoder := newStreamDecoder()
	decoder.feed([]byte("<component id='room title'"))
	if !hasDiagnostic(flattenActions(decoder.finish()), "incomplete") {
		t.Fatal("incomplete markup did not produce a diagnostic")
	}
}

func TestXMLPendingMarkupIsBounded(t *testing.T) {
	t.Parallel()

	decoder := newStreamDecoder()
	decoder.maxPending = 32
	events := flattenActions(decoder.feed([]byte("<component " + strings.Repeat("x", 64) + ">after\n")))
	if !hasDiagnostic(events, "overflow") {
		t.Fatalf("overflow diagnostic missing: %#v", events)
	}
	if !hasText(events, "after") {
		t.Fatalf("decoder did not resume: %#v", events)
	}
}

func TestTextCleanupAndRuneSpans(t *testing.T) {
	t.Parallel()
	if got := cleanText("<_name>body</_name> </3>"); got != "body </3>" {
		t.Fatalf("name-start cleanup = %q", got)
	}

	events := decodeEvents("\x1b[31mé<d cmd='north'>N</d> <a href='https://example.invalid'>site</a> <pushBold/>bold<popBold/> <preset id='speech'>say</preset>&amp;\n")
	var display DisplayEvent
	for _, event := range events {
		if event.kind == eventDisplay {
			display = event.display
		}
	}
	if display.Text != "éN site bold say&" {
		t.Fatalf("text = %q", display.Text)
	}
	if len(display.Links) != 2 || display.Links[0].Start != 1 || display.Links[0].Length != 1 || display.Links[1].URL != true {
		t.Fatalf("links = %#v", display.Links)
	}
	if len(display.Bold) != 1 || display.Bold[0] != (Span{Start: 8, Length: 4}) {
		t.Fatalf("bold = %#v", display.Bold)
	}
	if len(display.Presets) != 1 || display.Presets[0].ID != "speech" {
		t.Fatalf("presets = %#v", display.Presets)
	}
}

func TestTextSpansStayWithinTrimmedDisplayText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  Span
	}{
		{name: "link", input: "<d cmd='x'>north </d>\n", want: Span{Start: 0, Length: 5}},
		{name: "bold", input: "<pushBold/>bold <popBold/>\n", want: Span{Start: 0, Length: 4}},
		{name: "preset", input: "<preset id='speech'>say </preset>\n", want: Span{Start: 0, Length: 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := decodeEvents(tt.input)
			if len(events) != 1 || events[0].kind != eventDisplay {
				t.Fatalf("events = %#v", events)
			}
			display := events[0].display
			var got Span
			switch tt.name {
			case "link":
				if len(display.Links) != 1 {
					t.Fatalf("links = %#v", display.Links)
				}
				got = display.Links[0].Span
			case "bold":
				if len(display.Bold) != 1 {
					t.Fatalf("bold spans = %#v", display.Bold)
				}
				got = display.Bold[0]
			case "preset":
				if len(display.Presets) != 1 {
					t.Fatalf("preset spans = %#v", display.Presets)
				}
				got = display.Presets[0].Span
			}
			if got != tt.want {
				t.Fatalf("span = %#v, want %#v for text %q", got, tt.want, display.Text)
			}
			assertDisplaySpansInBounds(t, display)
		})
	}
}

func TestTextDuplicateEchoAndBlankRules(t *testing.T) {
	t.Parallel()

	events := decodeEvents("\n<pushStream id='talk'/>A says hi\n<popStream/>A says hi\n\n\nB replies\n")
	var displays []DisplayEvent
	for _, event := range events {
		if event.kind == eventDisplay && event.display.Kind == DisplayText {
			displays = append(displays, event.display)
		}
	}
	if len(displays) != 4 {
		t.Fatalf("display count = %d: %#v", len(displays), displays)
	}
	if displays[0].Stream != "talk" || displays[1].Text != "A says hi" || !displays[1].DuplicateEcho || displays[2].Text != "" || displays[3].Text != "B replies" {
		t.Fatalf("displays = %#v", displays)
	}
}

func TestSelfClosingContextsDoNotStrandText(t *testing.T) {
	t.Parallel()

	input := "<component id='room title'/><spell/><left/><right/><inv/><prompt time='1'/><preset id='speech'/>plain\n"
	events := decodeEvents(input)
	for _, event := range events {
		if event.kind == eventDisplay && event.display.Text == "plain" && event.display.Stream == "main" {
			return
		}
	}
	t.Fatalf("plain text stranded by context: %#v", events)
}

func TestXMLControlMarkupIsDiscardedAndUnknownCloseIsSilent(t *testing.T) {
	t.Parallel()

	events := decodeEvents("visible<!-- hidden > tail --> ok<?target value='>'?>!<!DOCTYPE game>done<![CDATA[secret > data]]><mystery>body</mystery>\n")
	if !hasText(events, "visible ok!donebody") {
		t.Fatalf("control markup leaked into text: %#v", events)
	}
	unknown := 0
	for _, event := range events {
		if event.kind == eventDiagnostic && strings.Contains(event.value, "unknown DragonRealms tag: mystery") {
			unknown++
		}
	}
	if unknown != 1 {
		t.Fatalf("unknown diagnostics = %d: %#v", unknown, events)
	}
}

func TestXMLBarePromptAndTagAdjacentBlank(t *testing.T) {
	t.Parallel()

	events := decodeEvents("line\n<b></b>\n\n H>\n")
	var displays []DisplayEvent
	var prompts []protocolEvent
	for _, event := range events {
		if event.kind == eventDisplay {
			displays = append(displays, event.display)
		}
		if event.kind == eventPrompt {
			prompts = append(prompts, event)
		}
	}
	if len(displays) != 1 || displays[0].Text != "line" {
		t.Fatalf("tag-adjacent blank was displayed: %#v", displays)
	}
	if len(prompts) != 1 || prompts[0].value != "H>" {
		t.Fatalf("bare prompt = %#v", prompts)
	}
}

func TestXMLOverflowTracksQuotesAcrossFeeds(t *testing.T) {
	t.Parallel()

	decoder := newStreamDecoder()
	decoder.maxPending = 16
	actions := decoder.feed([]byte("<component a='" + strings.Repeat("x", 20)))
	actions = append(actions, decoder.feed([]byte("still>quoted'>after\n"))...)
	events := flattenActions(actions)
	if !hasDiagnostic(events, "overflow") || !hasText(events, "after") {
		t.Fatalf("quoted overflow did not resume after the tag: %#v", events)
	}
	for _, event := range events {
		if event.kind == eventDisplay && strings.Contains(event.display.Text, "quoted") {
			t.Fatalf("quoted overflow data leaked into text: %#v", events)
		}
	}
}

func TestXMLComponentSpansDoNotLeakAndEmptyURLIsNotLinked(t *testing.T) {
	t.Parallel()

	events := decodeEvents("<component id='room exits'><d cmd='north'>north</d></component>plain\n<a>label</a>\n")
	var displays []DisplayEvent
	for _, event := range events {
		if event.kind == eventDisplay {
			displays = append(displays, event.display)
		}
	}
	if len(displays) != 2 {
		t.Fatalf("display events = %#v", displays)
	}
	if len(displays[0].Links) != 0 || len(displays[1].Links) != 0 {
		t.Fatalf("invalid links escaped component or empty URL: %#v", displays)
	}
}

func TestXMLDisplayActionOrderingAndToggles(t *testing.T) {
	t.Parallel()

	decoder := newStreamDecoder()
	actions := decoder.feed([]byte("before<clearStream id='main'/><output class='mono'/>mono\n<output class=''/><style id='roomName'/>éx<style id=''/>\n"))
	if len(actions) < 1 || len(actions[0].events) != 2 || actions[0].events[0].display.Text != "before" || actions[0].events[1].display.Kind != DisplayClear {
		t.Fatalf("flush and clear were not one ordered action: %#v", actions)
	}
	events := flattenActions(actions)
	var mono, styled DisplayEvent
	for _, event := range events {
		if event.kind != eventDisplay {
			continue
		}
		if event.display.Text == "mono" {
			mono = event.display
		}
		if event.display.Text == "éx" {
			styled = event.display
		}
	}
	if !mono.Mono || styled.Mono || len(styled.Presets) != 1 || styled.Presets[0] != (PresetSpan{Span: Span{Start: 0, Length: 2}, ID: "roomName"}) {
		t.Fatalf("display toggles = mono %#v, styled %#v", mono, styled)
	}
}

func TestXMLComponentCreatureSpansUseCleanedRuneOffsets(t *testing.T) {
	t.Parallel()

	events := decodeEvents("<component id='room objs'>&amp; <pushBold/>a goblín<popBold/>, a bench</component>")
	for _, event := range events {
		if event.kind == eventComponent {
			if len(event.items) != 1 || event.items[0] != "a goblín" {
				t.Fatalf("component creatures = %#v", event.items)
			}
			return
		}
	}
	t.Fatal("room objects component was not emitted")
}

func TestXMLTagLookaheadAndEOFAreChunkIndependent(t *testing.T) {
	t.Parallel()

	input := "</3> stays literal\n<_name>body</_name>\n"
	want := decodeFinished(input)
	if !hasText(want, "</3> stays literal") || !hasDiagnostic(want, "_name") {
		t.Fatalf("lookahead events = %#v", want)
	}
	assertFinishedAtEverySplit(t, input, want)

	for _, partial := range []string{
		"unterminated line",
		"<compass>north",
		"<inv>item",
		"<style id='roomName'/>title",
		"<d cmd='look'>label",
		"<dialogData id='injuries'><image id='head' name='Injury2'/>",
	} {
		events := decodeFinished(partial)
		diagnostics := 0
		for _, event := range events {
			if event.kind == eventDiagnostic {
				diagnostics++
			}
			if event.kind == eventDisplay {
				t.Fatalf("partial %q emitted display %#v", partial, event.display)
			}
		}
		if diagnostics != 1 || !hasDiagnostic(events, "incomplete") {
			t.Fatalf("partial %q diagnostics = %#v", partial, events)
		}
		assertFinishedAtEverySplit(t, partial, events)
	}
}

func TestParseTagAcceptsProtocolFormsAndRejectsMalformedNames(t *testing.T) {
	t.Parallel()

	name, attrs, closing, self, ok := parseTag("<PrOgReSsBaR ID='health' VALUE=\"7\"/>")
	if !ok || name != "PrOgReSsBaR" || closing || !self || attrs["id"] != "health" || attrs["value"] != "7" {
		t.Fatalf("mixed-case tag = name %q attrs %#v closing %v self %v ok %v", name, attrs, closing, self, ok)
	}

	name, attrs, closing, self, ok = parseTag("<component id='room title' ignored>")
	if !ok || name != "component" || closing || self || attrs["id"] != "room title" {
		t.Fatalf("fallback tag = name %q attrs %#v closing %v self %v ok %v", name, attrs, closing, self, ok)
	}

	for _, raw := range []string{"", "component", "<>", "<123>", "</component extra>"} {
		if _, _, _, _, ok := parseTag(raw); ok {
			t.Errorf("parseTag(%q) accepted malformed tag", raw)
		}
	}
}

func TestDuplicateEchoTracksLastNonduplicateStream(t *testing.T) {
	t.Parallel()

	events := decodeEvents("same\nsame\n<pushStream id='whispers'/>OOC message\n<popStream/><pushStream id='ooc'/>OOC message\n<popStream/>OOC message\n")
	var displays []DisplayEvent
	for _, event := range events {
		if event.kind == eventDisplay && event.display.Text != "" {
			displays = append(displays, event.display)
		}
	}
	if len(displays) != 5 {
		t.Fatalf("display events = %#v", displays)
	}
	for i, want := range []bool{false, false, false, true, true} {
		if displays[i].DuplicateEcho != want {
			t.Fatalf("display %d duplicate = %v, want %v: %#v", i, displays[i].DuplicateEcho, want, displays)
		}
	}
}

func TestXMLCompassCapturesBodyAndDirectionTags(t *testing.T) {
	t.Parallel()

	events := decodeEvents("<compass>north <dir value='east'/> southwest</compass>")
	if len(events) != 1 || events[0].kind != eventCompass || !reflect.DeepEqual(events[0].items, []string{"north", "east", "southwest"}) {
		t.Fatalf("compass events = %#v", events)
	}
}

func TestXMLMergeSeamsPreserveEventOrderAndInterruptedStream(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		assert func(*testing.T, []protocolEvent)
	}{
		{
			name:  "hand",
			input: "<left noun='ledger' exist='1'> black ledgerYou open it.</left>",
			assert: func(t *testing.T, events []protocolEvent) {
				if len(events) != 2 || events[0].kind != eventHand || events[0].value != "black ledger" || events[1].kind != eventDisplay || events[1].display.Text != "You open it." || events[1].display.Stream != "main" {
					t.Fatalf("hand events = %#v", events)
				}
			},
		},
		{
			name:  "spell",
			input: "<spell> Fire ShardsYou feel ready.</spell>",
			assert: func(t *testing.T, events []protocolEvent) {
				if len(events) != 2 || events[0].kind != eventSpell || events[0].value != "Fire Shards" || events[1].kind != eventDisplay || events[1].display.Text != "You feel ready." {
					t.Fatalf("spell events = %#v", events)
				}
			},
		},
		{
			name:  "inventory",
			input: "<pushStream id='ooc'/><inv> a leather bellowsYou put it away.</inv>",
			assert: func(t *testing.T, events []protocolEvent) {
				if len(events) != 2 || events[0].kind != eventDisplay || events[0].display.Stream != "inv" || events[0].display.Text != " a leather bellows" || events[1].kind != eventDisplay || events[1].display.Stream != "ooc" || events[1].display.Text != "You put it away." {
					t.Fatalf("inventory events = %#v", events)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := decodeEvents(tt.input)
			tt.assert(t, events)
		})
	}
}

func TestXMLMultilineStyleReanchorsUntilClosed(t *testing.T) {
	t.Parallel()

	events := decodeEvents("<style id='roomName'/>First\nSecond<style id=''/>\n")
	var displays []DisplayEvent
	for _, event := range events {
		if event.kind == eventDisplay {
			displays = append(displays, event.display)
		}
	}
	if len(displays) != 2 {
		t.Fatalf("style displays = %#v", displays)
	}
	for i, want := range []PresetSpan{{Span: Span{Start: 0, Length: 5}, ID: "roomName"}, {Span: Span{Start: 0, Length: 6}, ID: "roomName"}} {
		if len(displays[i].Presets) != 1 || displays[i].Presets[0] != want {
			t.Fatalf("display %d presets = %#v, want %#v", i, displays[i].Presets, want)
		}
	}
}

func TestPresetCloseFlushesOnlyLineBoundaryPresets(t *testing.T) {
	t.Parallel()

	type displaySummary struct {
		stream  string
		text    string
		presets []PresetSpan
	}
	tests := []struct {
		name  string
		input string
		want  []displaySummary
	}{
		{
			name:  "room description",
			input: "<preset id='roomDesc'>A broad room.</preset>Obvious exits: north\n",
			want: []displaySummary{
				{stream: "main", text: "A broad room.", presets: []PresetSpan{{Span: Span{Start: 0, Length: 13}, ID: "roomDesc"}}},
				{stream: "main", text: "Obvious exits: north"},
			},
		},
		{
			name:  "inventory items",
			input: "<pushStream id='inv'/><preset id='inv'>a sword</preset><preset id='inv'>a shield</preset><popStream/>",
			want: []displaySummary{
				{stream: "inv", text: "a sword", presets: []PresetSpan{{Span: Span{Start: 0, Length: 7}, ID: "inv"}}},
				{stream: "inv", text: "a shield", presets: []PresetSpan{{Span: Span{Start: 0, Length: 8}, ID: "inv"}}},
			},
		},
		{
			name:  "speech continuation",
			input: "<preset id='speech'>Renucci says,</preset> \"Hello.\"\n",
			want: []displaySummary{{
				stream: "main", text: "Renucci says, \"Hello.\"",
				presets: []PresetSpan{{Span: Span{Start: 0, Length: 13}, ID: "speech"}},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []displaySummary
			for _, event := range decodeEvents(tt.input) {
				if event.kind == eventDisplay {
					got = append(got, displaySummary{stream: event.display.Stream, text: event.display.Text, presets: append([]PresetSpan(nil), event.display.Presets...)})
				}
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("display events = %#v, want %#v", got, tt.want)
			}
			assertFinishedAtEverySplit(t, tt.input, decodeFinished(tt.input))
		})
	}
}

func TestXMLTagParityValidationAndTrimming(t *testing.T) {
	t.Parallel()

	events := decodeEvents("<container id='ignored' title='No target'/><container id='stow' title='Pack' target='#1'/><component id='room desc'>  room text  </component><left>  blade  </left><spell>  Fire Ball  </spell><spellTime value='0'/><spellTime value='2'/><dir value='north'/>")
	var containers, components, hands, spells, spellTimes, diagnostics []protocolEvent
	for _, event := range events {
		switch event.kind {
		case eventContainer:
			containers = append(containers, event)
		case eventComponent:
			components = append(components, event)
		case eventHand:
			hands = append(hands, event)
		case eventSpell:
			spells = append(spells, event)
		case eventSpellTime:
			spellTimes = append(spellTimes, event)
		case eventDiagnostic:
			diagnostics = append(diagnostics, event)
		}
	}
	if len(containers) != 1 || containers[0].aux != "#1" {
		t.Fatalf("containers = %#v", containers)
	}
	if len(components) != 1 || components[0].value != "room text" || len(hands) != 1 || hands[0].value != "blade" || len(spells) != 1 || spells[0].value != "Fire Ball" {
		t.Fatalf("trimmed events: components=%#v hands=%#v spells=%#v", components, hands, spells)
	}
	if len(spellTimes) != 1 || !spellTimes[0].timestamp.Equal(time.Unix(2, 0)) {
		t.Fatalf("spell times = %#v", spellTimes)
	}
	if len(diagnostics) != 2 || !hasDiagnostic(diagnostics, "spelltime") || !hasDiagnostic(diagnostics, "dir") {
		t.Fatalf("outside-compass dir diagnostics = %#v", diagnostics)
	}
}

func TestXMLInjuryParsingAndCreaturePhraseBoundaries(t *testing.T) {
	t.Parallel()

	events := decodeEvents("<dialogData id='injuries'><image name='Injury2'/><image id='arm' name='Injury12'/><image id='leg' name='Scar10'/><image id='nsys' name='Nsys0'/><image id='nsys2' name='Nsys-1'/><image id='nsys3' name='Nsys4'/></dialogData><component id='room objs'><pushBold/>a kobold<popBold/> that appears dead and <pushBold/>a viper<popBold/>.</component>")
	var injuries []protocolEvent
	var creatures []string
	for _, event := range events {
		if event.kind == eventInjury {
			injuries = append(injuries, event)
		}
		if event.kind == eventComponent {
			creatures = event.items
		}
	}
	wantInjuries := []protocolEvent{
		{kind: eventInjury, name: "arm", value: "wound", number: 12},
		{kind: eventInjury, name: "leg", value: "scar", number: 10},
		{kind: eventInjury, name: "nsys", value: "none", number: 0},
		{kind: eventInjury, name: "nsys2", value: "none", number: 0},
		{kind: eventInjury, name: "nsys3", value: "damage", number: 4},
	}
	if !reflect.DeepEqual(injuries, wantInjuries) {
		t.Fatalf("injuries = %#v", injuries)
	}
	if !reflect.DeepEqual(creatures, []string{"a kobold that appears dead", "a viper"}) {
		t.Fatalf("creatures = %#v", creatures)
	}
}

func TestXMLOverflowDiagnosticDoesNotExposeMarkup(t *testing.T) {
	t.Parallel()

	decoder := newStreamDecoder()
	decoder.maxPending = 24
	secret := "sensitive-token"
	events := flattenActions(decoder.feed([]byte("<component id='" + secret + "' " + strings.Repeat("x", 40) + ">")))
	if !hasDiagnostic(events, "overflow") {
		t.Fatalf("overflow events = %#v", events)
	}
	for _, event := range events {
		if event.kind == eventDiagnostic && (strings.Contains(event.value, secret) || strings.Contains(event.value, "id=")) {
			t.Fatalf("overflow diagnostic exposed markup: %q", event.value)
		}
	}
}

func decodeFinished(input string) []protocolEvent {
	decoder := newStreamDecoder()
	events := flattenActions(decoder.feed([]byte(input)))
	return append(events, flattenActions(decoder.finish())...)
}

func assertFinishedAtEverySplit(t *testing.T, input string, want []protocolEvent) {
	t.Helper()
	for split := 0; split <= len(input); split++ {
		decoder := newStreamDecoder()
		var got []protocolEvent
		got = appendActionEvents(got, decoder.feed([]byte(input[:split])))
		got = appendActionEvents(got, decoder.feed([]byte(input[split:])))
		got = appendActionEvents(got, decoder.finish())
		if diff := eventsDifference(want, got); diff != "" {
			t.Fatalf("split %d: %s", split, diff)
		}
	}
	decoder := newStreamDecoder()
	var got []protocolEvent
	for i := range len(input) {
		got = appendActionEvents(got, decoder.feed([]byte{input[i]}))
	}
	got = appendActionEvents(got, decoder.finish())
	if diff := eventsDifference(want, got); diff != "" {
		t.Fatalf("one byte at a time: %s", diff)
	}
}

func decodeEvents(input string) []protocolEvent {
	decoder := newStreamDecoder()
	return flattenActions(decoder.feed([]byte(input)))
}

func appendActionEvents(dst []protocolEvent, actions []protocolAction) []protocolEvent {
	for _, action := range actions {
		dst = append(dst, action.events...)
	}
	return dst
}

func flattenActions(actions []protocolAction) []protocolEvent {
	return appendActionEvents(nil, actions)
}

func eventsDifference(want, got []protocolEvent) string {
	if reflect.DeepEqual(want, got) {
		return ""
	}
	return "events differ\nwant: " + formatEvents(want) + "\n got: " + formatEvents(got)
}

func formatEvents(events []protocolEvent) string {
	var values []string
	for _, event := range events {
		values = append(values, event.String())
	}
	return strings.Join(values, ", ")
}

func hasDiagnostic(events []protocolEvent, contains string) bool {
	for _, event := range events {
		if event.kind == eventDiagnostic && strings.Contains(event.value, contains) {
			return true
		}
	}
	return false
}

func hasText(events []protocolEvent, text string) bool {
	for _, event := range events {
		if event.kind == eventDisplay && event.display.Kind == DisplayText && event.display.Text == text {
			return true
		}
	}
	return false
}

func assertDisplaySpansInBounds(t *testing.T, display DisplayEvent) {
	t.Helper()
	textLength := utf8.RuneCountInString(display.Text)
	check := func(kind string, span Span) {
		t.Helper()
		if span.Start < 0 || span.Length < 0 || span.Start+span.Length > textLength {
			t.Errorf("%s span %#v exceeds %d-rune text %q", kind, span, textLength, display.Text)
		}
	}
	for _, link := range display.Links {
		check("link", link.Span)
	}
	for _, span := range display.Bold {
		check("bold", span)
	}
	for _, preset := range display.Presets {
		check("preset", preset.Span)
	}
}
