package dragonrealms

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const defaultMaxPendingMarkup = 1 << 20

var (
	ansiPattern       = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)
	leakedXMLPattern  = regexp.MustCompile(`(?s)</?[A-Za-z_][^>]*>`)
	fallbackAttr      = regexp.MustCompile(`([A-Za-z_:][A-Za-z0-9_.:-]*)\s*=\s*(['"])(.*?)['"]`)
	barePromptPattern = regexp.MustCompile(`^[A-Z]*>$`)
)

type tagDropClass uint8

const (
	dropSettings tagDropClass = iota
	dropData
)

var droppedTags = map[string]tagDropClass{
	"mode": dropSettings, "settings": dropSettings, "presets": dropSettings,
	"p": dropSettings, "macros": dropSettings, "keys": dropSettings,
	"k": dropSettings, "palette": dropSettings, "i": dropSettings,
	"stream": dropSettings, "w": dropSettings, "cmdline": dropSettings,
	"strings": dropSettings, "names": dropSettings, "ignores": dropSettings,
	"vars": dropSettings, "scripts": dropSettings, "dialog": dropSettings,
	"builtin": dropSettings, "panels": dropSettings, "group": dropSettings,
	"toggles": dropSettings, "misc": dropSettings, "m": dropSettings,
	"display": dropSettings, "options": dropSettings, "o": dropSettings,
	"font": dropSettings, "s": dropSettings, "switchquickbar": dropSettings,
	"link": dropSettings, "forging": dropSettings,

	"skin": dropData, "compdef": dropData, "opendialog": dropData,
	"radio": dropData, "detach": dropData, "playerid": dropData,
	"exposecontainer": dropData, "clearcontainer": dropData, "menuimage": dropData,
	"closedialog": dropData, "exposedialog": dropData, "menulink": dropData,
	"label": dropData, "cmdbutton": dropData, "closebutton": dropData,
	"checkbox": dropData, "streambox": dropData, "dropdownbox": dropData,
	"editbox": dropData, "updowneditbox": dropData,
}

type openSpan struct {
	start  int
	target string
	url    bool
}

type openPreset struct {
	start int
	id    string
}

type captureBuffer struct {
	id        string
	data      bytes.Buffer
	bold      []Span
	boldStack []int
}

type streamDecoder struct {
	pending       []byte
	maxPending    int
	overflow      bool
	overflowName  string
	overflowSize  int
	overflowQuote byte

	activeStream string
	streamStack  []string
	text         bytes.Buffer
	links        []LinkSpan
	linkStack    []openSpan
	bold         []Span
	boldStack    []int
	presets      []PresetSpan
	presetStack  []openPreset
	styleStart   int
	styleID      string
	mono         bool

	component      *captureBuffer
	spell          *bytes.Buffer
	hand           *bytes.Buffer
	handName       string
	handNoun       string
	handExist      string
	compass        bool
	compassData    bytes.Buffer
	prompt         *bytes.Buffer
	promptTime     time.Time
	inv            bool
	dialogInjuries bool

	lastEmittedStream string
	lastEmittedText   string
	emittedLine       bool
}

func newStreamDecoder() *streamDecoder {
	return &streamDecoder{
		maxPending:   defaultMaxPendingMarkup,
		activeStream: "main",
		styleStart:   -1,
	}
}

func (d *streamDecoder) feed(data []byte) []protocolAction {
	if d.overflow {
		data = d.consumeOverflow(data)
		if d.overflow {
			return nil
		}
	}
	d.pending = append(d.pending, data...)
	var actions []protocolAction
	for len(d.pending) > 0 {
		if d.pending[0] == '\n' {
			d.pending = d.pending[1:]
			if event, ok := d.flushText(true); ok {
				actions = append(actions, protocolAction{events: []protocolEvent{event}})
			}
			continue
		}
		if d.pending[0] != '<' {
			next := bytes.IndexAny(d.pending, "<\n")
			if next < 0 {
				d.appendText(d.pending)
				d.pending = nil
				break
			}
			d.appendText(d.pending[:next])
			d.pending = d.pending[next:]
			continue
		}

		markup, decided := markupCandidate(d.pending)
		if !decided {
			break
		}
		if !markup {
			d.appendText(d.pending[:1])
			d.pending = d.pending[1:]
			continue
		}
		end := markupEnd(d.pending)
		if end < 0 {
			if len(d.pending) <= d.maxPending {
				break
			}
			d.startOverflow()
			actions = append(actions, protocolAction{events: []protocolEvent{d.overflowDiagnostic()}})
			continue
		}
		if end+1 > d.maxPending {
			limit := d.maxPending
			if limit > len(d.pending) {
				limit = len(d.pending)
			}
			d.setOverflowInfo(d.pending[:limit], end+1)
			d.pending = d.pending[end+1:]
			actions = append(actions, protocolAction{events: []protocolEvent{d.overflowDiagnostic()}})
			continue
		}
		raw := append([]byte(nil), d.pending[:end+1]...)
		d.pending = d.pending[end+1:]
		if events := d.handleMarkup(raw); len(events) > 0 {
			actions = append(actions, protocolAction{events: events})
		}
	}
	return actions
}

func (d *streamDecoder) finish() []protocolAction {
	if d.overflow {
		d.resetAfterFinish()
		return nil
	}
	if d.incomplete() {
		d.resetAfterFinish()
		return []protocolAction{{events: []protocolEvent{{kind: eventDiagnostic, value: "incomplete DragonRealms stream data discarded"}}}}
	}
	return nil
}

func markupCandidate(data []byte) (bool, bool) {
	if len(data) < 2 {
		return false, false
	}
	next := data[1]
	if asciiNameStart(next) || next == '!' || next == '?' {
		return true, true
	}
	if next != '/' {
		return false, true
	}
	if len(data) < 3 {
		return false, false
	}
	return asciiNameStart(data[2]), true
}

func asciiNameStart(value byte) bool {
	return value == '_' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func markupEnd(data []byte) int {
	for _, marker := range []struct {
		prefix []byte
		suffix []byte
	}{
		{prefix: []byte("<!--"), suffix: []byte("-->")},
		{prefix: []byte("<![CDATA["), suffix: []byte("]]>")},
		{prefix: []byte("<?"), suffix: []byte("?>")},
	} {
		if !bytes.HasPrefix(data, marker.prefix) {
			continue
		}
		if end := bytes.Index(data[len(marker.prefix):], marker.suffix); end >= 0 {
			return len(marker.prefix) + end + len(marker.suffix) - 1
		}
		return -1
	}

	quote := byte(0)
	for i := 1; i < len(data); i++ {
		switch data[i] {
		case '\'', '"':
			if quote == 0 {
				quote = data[i]
			} else if quote == data[i] {
				quote = 0
			}
		case '>':
			if quote == 0 {
				return i
			}
		}
	}
	return -1
}

func (d *streamDecoder) startOverflow() {
	limit := d.maxPending
	if limit > len(d.pending) {
		limit = len(d.pending)
	}
	d.setOverflowInfo(d.pending[:limit], len(d.pending))
	end, quote := overflowEnd(d.pending, 0)
	if end >= 0 {
		d.pending = d.pending[end+1:]
		d.overflow = false
	} else {
		d.pending = nil
		d.overflow = true
		d.overflowQuote = quote
	}
}

func (d *streamDecoder) consumeOverflow(data []byte) []byte {
	end, quote := overflowEnd(data, d.overflowQuote)
	if end >= 0 {
		d.overflow = false
		d.overflowQuote = 0
		return data[end+1:]
	}
	d.overflowQuote = quote
	return nil
}

func overflowEnd(data []byte, quote byte) (int, byte) {
	for i, value := range data {
		switch value {
		case '\'', '"':
			if quote == 0 {
				quote = value
			} else if quote == value {
				quote = 0
			}
		case '>':
			if quote == 0 {
				return i, 0
			}
		case '\n':
			return i, 0
		}
	}
	return -1, quote
}

func (d *streamDecoder) handleMarkup(raw []byte) []protocolEvent {
	d.emittedLine = false
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, "<!--") || strings.HasPrefix(trimmed, "<?") || strings.HasPrefix(trimmed, "<!") {
		return nil
	}
	name, attrs, closing, self, ok := parseTag(trimmed)
	if !ok {
		return []protocolEvent{{kind: eventDiagnostic, value: "malformed DragonRealms tag discarded"}}
	}
	name = strings.ToLower(name)
	if closing {
		return d.handleClose(name)
	}
	if class, dropped := droppedTags[name]; dropped {
		kind := "dropped settings"
		if class == dropData {
			kind = "dropped data"
		}
		return []protocolEvent{{kind: eventDiagnostic, value: kind + " tag: " + sanitizeDiagnostic(name)}}
	}

	var events []protocolEvent
	flush := func() {
		if event, ok := d.flushText(false); ok {
			events = append(events, event)
		}
	}
	switch name {
	case "component":
		flush()
		if self {
			events = append(events, protocolEvent{kind: eventComponent, name: attrs["id"]})
		} else {
			d.component = &captureBuffer{id: attrs["id"]}
		}
	case "roundtime", "casttime", "spelltime":
		stamp, valid := parseEpoch(attrs["value"])
		if !valid || name == "spelltime" && stamp.Unix() <= 0 {
			events = append(events, protocolEvent{kind: eventDiagnostic, value: "invalid " + name + " timestamp"})
			break
		}
		kind := eventRoundTime
		if name == "casttime" {
			kind = eventCastTime
		} else if name == "spelltime" {
			kind = eventSpellTime
		}
		events = append(events, protocolEvent{kind: kind, timestamp: stamp})
	case "progressbar":
		value, err := strconv.Atoi(attrs["value"])
		if attrs["id"] == "" || err != nil {
			events = append(events, protocolEvent{kind: eventDiagnostic, value: "invalid progress bar"})
		} else {
			events = append(events, protocolEvent{kind: eventProgress, name: attrs["id"], number: value})
		}
	case "resource":
		if picture, ok := attrs["picture"]; ok {
			events = append(events, protocolEvent{kind: eventRoomImage, value: picture})
		} else if id := attrs["id"]; id != "" {
			value, _ := strconv.Atoi(attrs["value"])
			events = append(events, protocolEvent{kind: eventResource, name: id, number: value})
		}
	case "indicator":
		events = append(events, protocolEvent{kind: eventIndicator, name: attrs["id"], flag: attrs["visible"] == "y"})
	case "crtrstatus":
		if exist := attrs["exist"]; exist != "" {
			events = append(events, protocolEvent{kind: eventCreatureStatus, name: exist, flag: attrs["hostile"] == "1", flag2: attrs["disengaged"] == "1", flag3: attrs["flying"] == "1"})
		}
	case "left", "right":
		flush()
		if self {
			events = append(events, protocolEvent{kind: eventHand, name: name, value: "", aux: attrs["noun"] + "\x00" + attrs["exist"]})
		} else {
			d.hand = &bytes.Buffer{}
			d.handName, d.handNoun, d.handExist = name, attrs["noun"], attrs["exist"]
		}
	case "spell":
		flush()
		if self {
			events = append(events, protocolEvent{kind: eventSpell})
		} else {
			d.spell = &bytes.Buffer{}
		}
	case "pushstream":
		flush()
		d.streamStack = append(d.streamStack, d.activeStream)
		d.activeStream = attrs["id"]
		if d.activeStream == "" {
			d.activeStream = "main"
		}
	case "popstream":
		flush()
		if len(d.streamStack) > 0 {
			d.activeStream = d.streamStack[len(d.streamStack)-1]
			d.streamStack = d.streamStack[:len(d.streamStack)-1]
		} else {
			d.activeStream = "main"
		}
	case "prompt":
		flush()
		d.activeStream = "main"
		d.streamStack = nil
		stamp, valid := parseEpoch(attrs["time"])
		if !valid {
			events = append(events, protocolEvent{kind: eventDiagnostic, value: "invalid prompt timestamp"})
			stamp = time.Time{}
		}
		if self {
			events = append(events, protocolEvent{kind: eventPrompt, timestamp: stamp})
		} else {
			d.prompt = &bytes.Buffer{}
			d.promptTime = stamp
		}
	case "compass":
		flush()
		d.compassData.Reset()
		if self {
			events = append(events, protocolEvent{kind: eventCompass})
		} else {
			d.compass = true
		}
	case "dir":
		if d.compass && attrs["value"] != "" {
			if d.compassData.Len() > 0 {
				d.compassData.WriteByte(' ')
			}
			d.compassData.WriteString(attrs["value"])
		} else if !d.compass {
			events = append(events, protocolEvent{kind: eventDiagnostic, value: "unknown DragonRealms dir outside compass"})
		}
	case "d":
		if !self {
			d.linkStack = append(d.linkStack, openSpan{start: d.currentRuneLen(), target: attrs["cmd"]})
		}
	case "a":
		if !self {
			d.linkStack = append(d.linkStack, openSpan{start: d.currentRuneLen(), target: attrs["href"], url: true})
		}
	case "pushbold":
		d.pushBold()
	case "popbold":
		d.popBold()
	case "b":
		if !self {
			d.pushBold()
		}
	case "inv":
		flush()
		if !self {
			d.streamStack = append(d.streamStack, d.activeStream)
			d.activeStream = "inv"
			d.inv = true
		}
	case "preset":
		if !self {
			d.presetStack = append(d.presetStack, openPreset{start: d.currentRuneLen(), id: attrs["id"]})
		}
	case "style":
		if id := attrs["id"]; id != "" {
			d.styleStart, d.styleID = d.currentRuneLen(), id
		} else if d.styleStart >= 0 {
			d.presets = append(d.presets, PresetSpan{Span: Span{Start: d.styleStart, Length: d.currentRuneLen() - d.styleStart}, ID: d.styleID})
			d.styleStart, d.styleID = -1, ""
		}
	case "nav":
		if attrs["rm"] != "" {
			events = append(events, protocolEvent{kind: eventNav, value: attrs["rm"]})
		}
	case "app":
		if attrs["char"] != "" {
			events = append(events, protocolEvent{kind: eventApp, name: attrs["char"], value: attrs["game"], aux: attrs["title"]})
		}
	case "container":
		if attrs["target"] != "" {
			events = append(events, protocolEvent{kind: eventContainer, name: attrs["id"], value: attrs["title"], aux: attrs["target"]})
		}
	case "dialogdata":
		d.dialogInjuries = strings.EqualFold(attrs["id"], "injuries") && !self
	case "image":
		if d.dialogInjuries && attrs["id"] != "" {
			kind, severity := parseInjury(attrs["id"], attrs["name"])
			events = append(events, protocolEvent{kind: eventInjury, name: attrs["id"], value: kind, number: severity})
		}
	case "settingsinfo":
		events = append(events, protocolEvent{kind: eventSettingsInfo})
	case "endsetup":
		events = append(events, protocolEvent{kind: eventEndSetup})
	case "clearstream", "cleardynastream":
		flush()
		events = append(events, protocolEvent{kind: eventDisplay, display: DisplayEvent{Kind: DisplayClear, Stream: attrs["id"], ID: attrs["id"]}})
	case "streamwindow", "openwindow":
		flush()
		events = append(events, protocolEvent{kind: eventDisplay, display: DisplayEvent{Kind: DisplayWindow, ID: attrs["id"], Title: attrs["title"]}})
		if strings.EqualFold(attrs["id"], "room") {
			title, id := roomSubtitle(attrs["subtitle"])
			if title != "" {
				events = append(events, protocolEvent{kind: eventComponent, name: "room title", value: title})
			}
			if id != "" {
				events = append(events, protocolEvent{kind: eventNav, value: id})
			}
		}
	case "output":
		d.mono = strings.EqualFold(attrs["class"], "mono")
	default:
		events = append(events, protocolEvent{kind: eventDiagnostic, value: "unknown DragonRealms tag: " + sanitizeDiagnostic(name)})
	}
	return events
}

func (d *streamDecoder) handleClose(name string) []protocolEvent {
	switch name {
	case "component":
		if d.component == nil {
			return nil
		}
		content := strings.TrimSpace(cleanText(d.component.data.String()))
		event := protocolEvent{kind: eventComponent, name: d.component.id, value: content, items: componentCreatures(d.component)}
		d.component = nil
		d.resetSpans()
		return []protocolEvent{event}
	case "spell":
		if d.spell == nil {
			return nil
		}
		body, appended := splitMergeSeam(d.spell.String())
		value := strings.TrimSpace(cleanText(body))
		d.spell = nil
		events := []protocolEvent{{kind: eventSpell, value: value}}
		if appended != "" {
			d.text.WriteString(appended)
			if event, ok := d.flushText(false); ok {
				events = append(events, event)
			}
		}
		return events
	case "left", "right":
		if d.hand == nil {
			return nil
		}
		body, appended := splitMergeSeam(d.hand.String())
		value := strings.TrimSpace(cleanText(body))
		event := protocolEvent{kind: eventHand, name: d.handName, value: value, aux: d.handNoun + "\x00" + d.handExist}
		d.hand = nil
		events := []protocolEvent{event}
		if appended != "" {
			d.text.WriteString(appended)
			if display, ok := d.flushText(false); ok {
				events = append(events, display)
			}
		}
		return events
	case "compass":
		if !d.compass {
			return nil
		}
		d.compass = false
		items := strings.Fields(cleanInline(d.compassData.String()))
		d.compassData.Reset()
		return []protocolEvent{{kind: eventCompass, items: items}}
	case "prompt":
		if d.prompt == nil {
			return nil
		}
		value := strings.TrimSpace(html.UnescapeString(strings.ToValidUTF8(d.prompt.String(), "�")))
		d.prompt = nil
		return []protocolEvent{{kind: eventPrompt, value: value, timestamp: d.promptTime}}
	case "d", "a":
		if len(d.linkStack) == 0 {
			return nil
		}
		open := d.linkStack[len(d.linkStack)-1]
		d.linkStack = d.linkStack[:len(d.linkStack)-1]
		end := d.currentRuneLen()
		if end > open.start {
			target := open.target
			if target == "" && !open.url {
				target = runeSlice(d.currentText(), open.start, end)
			}
			if target != "" {
				d.links = append(d.links, LinkSpan{Span: Span{Start: open.start, Length: end - open.start}, Target: target, URL: open.url})
			}
		}
	case "b":
		d.popBold()
	case "preset":
		presetID := ""
		if len(d.presetStack) > 0 {
			open := d.presetStack[len(d.presetStack)-1]
			d.presetStack = d.presetStack[:len(d.presetStack)-1]
			presetID = open.id
			if end := d.currentRuneLen(); end > open.start {
				d.presets = append(d.presets, PresetSpan{Span: Span{Start: open.start, Length: end - open.start}, ID: open.id})
			}
		}
		if presetID == "roomDesc" || presetID == "inv" {
			if event, ok := d.flushText(false); ok {
				return []protocolEvent{event}
			}
		}
	case "inv":
		if d.inv {
			var events []protocolEvent
			body, appended := splitMergeSeam(d.text.String())
			d.text.Reset()
			d.text.WriteString(body)
			if event, ok := d.flushText(false); ok {
				events = append(events, event)
			}
			d.inv = false
			if len(d.streamStack) > 0 {
				d.activeStream = d.streamStack[len(d.streamStack)-1]
				d.streamStack = d.streamStack[:len(d.streamStack)-1]
			} else {
				d.activeStream = "main"
			}
			if appended != "" {
				d.text.WriteString(appended)
				if event, ok := d.flushText(false); ok {
					events = append(events, event)
				}
			}
			return events
		}
	case "dialogdata":
		d.dialogInjuries = false
	}
	return nil
}

func parseTag(raw string) (string, map[string]string, bool, bool, bool) {
	raw = strings.TrimSpace(raw)
	if len(raw) < 3 || raw[0] != '<' || raw[len(raw)-1] != '>' {
		return "", nil, false, false, false
	}
	body := strings.TrimSpace(raw[1 : len(raw)-1])
	if body == "" {
		return "", nil, false, false, false
	}
	closing := body[0] == '/'
	if closing {
		name := strings.TrimSpace(body[1:])
		if !validTagName(name) {
			return "", nil, true, false, false
		}
		return name, map[string]string{}, true, false, true
	}
	self := strings.HasSuffix(body, "/")
	if self {
		body = strings.TrimSpace(strings.TrimSuffix(body, "/"))
	}
	fields := strings.Fields(body)
	if len(fields) == 0 || !validTagName(fields[0]) {
		return "", nil, false, self, false
	}
	decoder := xml.NewDecoder(strings.NewReader(raw))
	decoder.Strict = false
	token, err := decoder.Token()
	if err == nil {
		if start, ok := token.(xml.StartElement); ok {
			attrs := make(map[string]string, len(start.Attr))
			for _, attr := range start.Attr {
				attrs[strings.ToLower(attr.Name.Local)] = attr.Value
			}
			return start.Name.Local, attrs, false, self, true
		}
	}
	attrs := make(map[string]string)
	for _, match := range fallbackAttr.FindAllStringSubmatch(body, -1) {
		attrs[strings.ToLower(match[1])] = match[3]
	}
	return fields[0], attrs, false, self, true
}

func validTagName(name string) bool {
	if name == "" || !asciiNameStart(name[0]) {
		return false
	}
	for i := 1; i < len(name); i++ {
		value := name[i]
		if asciiNameStart(value) || value >= '0' && value <= '9' || value == '.' || value == '-' || value == ':' {
			continue
		}
		return false
	}
	return true
}

func (d *streamDecoder) appendText(data []byte) {
	if len(data) == 0 {
		return
	}
	switch {
	case d.component != nil:
		d.component.data.Write(data)
	case d.spell != nil:
		d.spell.Write(data)
	case d.hand != nil:
		d.hand.Write(data)
	case d.compass:
		d.compassData.Write(data)
	case d.prompt != nil:
		d.prompt.Write(data)
	default:
		d.text.Write(data)
	}
}

func (d *streamDecoder) flushText(newline bool) (protocolEvent, bool) {
	if d.component != nil || d.spell != nil || d.hand != nil || d.prompt != nil {
		if newline {
			d.appendText([]byte{'\n'})
		}
		return protocolEvent{}, false
	}
	raw := d.text.String()
	d.text.Reset()
	if strings.HasSuffix(raw, "\r") {
		raw = strings.TrimSuffix(raw, "\r")
	}
	styleOpen := d.styleStart >= 0
	styleID := d.styleID
	if styleOpen {
		end := utf8.RuneCountInString(strings.ToValidUTF8(raw, "�"))
		if end > d.styleStart {
			d.presets = append(d.presets, PresetSpan{Span: Span{Start: d.styleStart, Length: end - d.styleStart}, ID: styleID})
		}
	}
	defer func() {
		d.resetSpans()
		if styleOpen {
			d.styleStart = 0
			d.styleID = styleID
		}
	}()
	text := cleanText(raw)
	if text == "" {
		if newline && d.emittedLine {
			d.emittedLine = false
			return protocolEvent{kind: eventDisplay, display: DisplayEvent{Kind: DisplayText, Stream: d.activeStream}}, true
		}
		return protocolEvent{}, false
	}
	if barePromptPattern.MatchString(strings.TrimLeft(text, " \t")) {
		prompt := strings.TrimLeft(text, " \t")
		d.emittedLine = false
		return protocolEvent{kind: eventPrompt, value: prompt}, true
	}

	display := DisplayEvent{
		Kind:    DisplayText,
		Stream:  d.activeStream,
		Text:    text,
		Mono:    d.mono,
		Links:   adjustLinks(raw, d.links),
		Bold:    adjustSpans(raw, d.bold),
		Presets: adjustPresets(raw, d.presets),
	}
	if d.activeStream != "talk" && d.activeStream != "whispers" && (d.lastEmittedStream == "talk" || d.lastEmittedStream == "whispers") && text == d.lastEmittedText {
		display.DuplicateEcho = true
	}
	if !display.DuplicateEcho {
		d.lastEmittedStream = d.activeStream
		d.lastEmittedText = text
	}
	d.emittedLine = true
	return protocolEvent{kind: eventDisplay, display: display}, true
}

func (d *streamDecoder) resetSpans() {
	d.links = nil
	d.bold = nil
	d.presets = nil
	d.linkStack = nil
	d.boldStack = nil
	d.presetStack = nil
}

func (d *streamDecoder) currentText() string {
	if d.component != nil {
		return d.component.data.String()
	}
	return d.text.String()
}

func (d *streamDecoder) currentRuneLen() int {
	return utf8.RuneCountInString(strings.ToValidUTF8(d.currentText(), "�"))
}

func (d *streamDecoder) pushBold() {
	if d.component != nil {
		d.component.boldStack = append(d.component.boldStack, utf8.RuneCountInString(strings.ToValidUTF8(d.component.data.String(), "�")))
		return
	}
	d.boldStack = append(d.boldStack, d.currentRuneLen())
}

func (d *streamDecoder) popBold() {
	if d.component != nil {
		if len(d.component.boldStack) == 0 {
			return
		}
		start := d.component.boldStack[len(d.component.boldStack)-1]
		d.component.boldStack = d.component.boldStack[:len(d.component.boldStack)-1]
		end := utf8.RuneCountInString(strings.ToValidUTF8(d.component.data.String(), "�"))
		if end > start {
			d.component.bold = append(d.component.bold, Span{Start: start, Length: end - start})
		}
		return
	}
	if len(d.boldStack) == 0 {
		return
	}
	start := d.boldStack[len(d.boldStack)-1]
	d.boldStack = d.boldStack[:len(d.boldStack)-1]
	if end := d.currentRuneLen(); end > start {
		d.bold = append(d.bold, Span{Start: start, Length: end - start})
	}
}

func cleanText(value string) string {
	value = strings.ToValidUTF8(value, "�")
	value = ansiPattern.ReplaceAllString(value, "")
	value = leakedXMLPattern.ReplaceAllString(value, "")
	value = html.UnescapeString(value)
	return strings.TrimRight(value, " \t\r\n")
}

func cleanInline(value string) string {
	value = strings.ToValidUTF8(value, "�")
	value = ansiPattern.ReplaceAllString(value, "")
	value = leakedXMLPattern.ReplaceAllString(value, "")
	return html.UnescapeString(value)
}

func adjustLinks(raw string, spans []LinkSpan) []LinkSpan {
	result := make([]LinkSpan, 0, len(spans))
	for _, span := range spans {
		start, length := adjustSpan(raw, span.Start, span.Length)
		span.Start, span.Length = start, length
		if length > 0 {
			result = append(result, span)
		}
	}
	return result
}

func adjustSpans(raw string, spans []Span) []Span {
	result := make([]Span, 0, len(spans))
	for _, span := range spans {
		span.Start, span.Length = adjustSpan(raw, span.Start, span.Length)
		if span.Length > 0 {
			result = append(result, span)
		}
	}
	return result
}

func adjustPresets(raw string, spans []PresetSpan) []PresetSpan {
	result := make([]PresetSpan, 0, len(spans))
	for _, span := range spans {
		span.Start, span.Length = adjustSpan(raw, span.Start, span.Length)
		if span.Length > 0 {
			result = append(result, span)
		}
	}
	return result
}

func adjustSpan(raw string, start, length int) (int, int) {
	runes := []rune(strings.ToValidUTF8(raw, "�"))
	if start < 0 {
		start = 0
	}
	if start > len(runes) {
		start = len(runes)
	}
	end := start + length
	if end > len(runes) {
		end = len(runes)
	}
	cleanStart := utf8.RuneCountInString(cleanInline(string(runes[:start])))
	cleanEnd := utf8.RuneCountInString(cleanInline(string(runes[:end])))
	visibleLength := utf8.RuneCountInString(cleanText(raw))
	if cleanStart > visibleLength {
		cleanStart = visibleLength
	}
	if cleanEnd > visibleLength {
		cleanEnd = visibleLength
	}
	if cleanEnd < cleanStart {
		cleanEnd = cleanStart
	}
	return cleanStart, cleanEnd - cleanStart
}

func parseEpoch(value string) (time.Time, bool) {
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil || seconds < 0 || seconds > 253402300799 {
		return time.Time{}, false
	}
	return time.Unix(seconds, 0), true
}

func componentCreatures(component *captureBuffer) []string {
	if len(component.bold) == 0 {
		return nil
	}
	raw := component.data.String()
	text := []rune(cleanText(raw))
	spans := make([]Span, 0, len(component.bold))
	for _, span := range component.bold {
		start, length := adjustSpan(raw, span.Start, span.Length)
		spans = append(spans, Span{Start: start, Length: length})
	}
	var result []string
	for _, span := range spans {
		start, length := span.Start, span.Length
		if start > len(text) {
			continue
		}
		end := start + length
		if end > len(text) {
			end = len(text)
		}
		for end < len(text) && text[end] != ',' && text[end] != '.' {
			end++
		}
		for _, other := range spans {
			if other.Start > start && other.Start < end {
				end = other.Start
			}
		}
		name := trimTrailingConnector(string(text[start:end]))
		if name != "" {
			result = append(result, name)
		}
	}
	return result
}

func roomSubtitle(subtitle string) (string, string) {
	lastOpen := strings.LastIndex(subtitle, "[")
	lastClose := strings.LastIndex(subtitle, "]")
	title := ""
	if lastOpen >= 0 && lastClose > lastOpen {
		title = subtitle[lastOpen : lastClose+1]
	} else if marker := strings.Index(subtitle, " - "); marker >= 0 {
		title = strings.TrimSpace(subtitle[marker+3:])
	} else {
		title = strings.Trim(strings.TrimSpace(subtitle), "-")
	}
	tail := subtitle
	if lastClose >= 0 {
		tail = subtitle[lastClose+1:]
	}
	start := strings.IndexByte(tail, '(')
	end := strings.IndexByte(tail, ')')
	id := ""
	if start >= 0 && end > start+1 {
		candidate := tail[start+1 : end]
		if _, err := strconv.Atoi(candidate); err == nil {
			id = candidate
		}
	}
	return title, id
}

func parseInjury(area, image string) (string, int) {
	if strings.EqualFold(area, image) {
		return "none", 0
	}
	if suffix, ok := injurySuffix(image, "Injury"); ok {
		severity, _ := strconv.Atoi(suffix)
		return "wound", severity
	}
	if suffix, ok := injurySuffix(image, "Scar"); ok {
		severity, _ := strconv.Atoi(suffix)
		return "scar", severity
	}
	if suffix, ok := injurySuffix(image, "Nsys"); ok {
		severity, err := strconv.Atoi(suffix)
		if err == nil && severity > 0 {
			return "damage", severity
		}
		return "none", 0
	}
	return "none", 0
}

func injurySuffix(value, prefix string) (string, bool) {
	if len(value) <= len(prefix) || !strings.EqualFold(value[:len(prefix)], prefix) {
		return "", false
	}
	return value[len(prefix):], true
}

func splitMergeSeam(value string) (string, string) {
	var previous rune
	for index, current := range value {
		if index > 0 && unicode.IsLower(previous) && unicode.IsUpper(current) {
			return value[:index], value[index:]
		}
		previous = current
	}
	return value, ""
}

func trimTrailingConnector(value string) string {
	value = strings.TrimSpace(value)
	for {
		switch {
		case strings.HasSuffix(value, ","):
			value = strings.TrimSpace(strings.TrimSuffix(value, ","))
		case len(value) >= 4 && strings.EqualFold(value[len(value)-4:], " and"):
			value = strings.TrimSpace(value[:len(value)-4])
		default:
			return value
		}
	}
}

func runeSlice(value string, start, end int) string {
	runes := []rune(value)
	if start < 0 || start > len(runes) {
		return ""
	}
	if end > len(runes) {
		end = len(runes)
	}
	return string(runes[start:end])
}

func sanitizeDiagnostic(value string) string {
	value = strings.ToValidUTF8(value, "�")
	runes := []rune(value)
	for i, r := range runes {
		if r < 32 || r == 127 {
			runes[i] = ' '
		}
	}
	if len(runes) > 256 {
		runes = runes[:256]
	}
	return string(runes)
}

func (d *streamDecoder) incomplete() bool {
	return len(d.pending) > 0 || d.text.Len() > 0 || d.component != nil || d.spell != nil || d.hand != nil || d.prompt != nil || d.compass || d.inv || d.dialogInjuries || len(d.streamStack) > 0 || len(d.linkStack) > 0 || len(d.boldStack) > 0 || len(d.presetStack) > 0 || d.styleStart >= 0
}

func (d *streamDecoder) resetAfterFinish() {
	maxPending := d.maxPending
	*d = *newStreamDecoder()
	d.maxPending = maxPending
}

func (d *streamDecoder) setOverflowInfo(raw []byte, size int) {
	d.overflowName = markupName(raw)
	d.overflowSize = size
}

func markupName(raw []byte) string {
	index := 1
	if index < len(raw) && raw[index] == '/' {
		index++
	}
	start := index
	for index < len(raw) {
		value := raw[index]
		if !(asciiNameStart(value) || index > start && (value >= '0' && value <= '9' || value == '-' || value == '.' || value == ':')) {
			break
		}
		index++
	}
	if start == index {
		return "markup"
	}
	return string(raw[start:index])
}

func (d *streamDecoder) overflowDiagnostic() protocolEvent {
	return protocolEvent{kind: eventDiagnostic, value: sanitizeDiagnostic(fmt.Sprintf("stream overflow: %s markup exceeded %d bytes", d.overflowName, d.overflowSize))}
}
