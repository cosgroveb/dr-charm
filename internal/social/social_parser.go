package social

import (
	"regexp"
	"strings"
	"time"
)

// SocialEvent represents a parsed social interaction
type SocialEvent struct {
	Type         string `json:"type"`
	Subtype      string `json:"subtype"`
	From         string `json:"from"`
	FromSimple   string `json:"from_simple"`
	Message      string `json:"message"`
	DirectedAtMe bool   `json:"directed_at_me"`
	Timestamp    int64  `json:"timestamp"`
	Raw          string `json:"raw"`
}

// Social event subtypes
const (
	SocialWhisper = "whisper"
	SocialSay     = "say"
	SocialAsk     = "ask"
	SocialExclaim = "exclaim"
	SocialThought = "thought"
)

// Regex patterns for parsing social interactions
var (
	// Directed patterns
	directedSayPattern     = regexp.MustCompile(`^(.+?) says to you, "(.+)"$`)
	directedAskPattern     = regexp.MustCompile(`^(.+?) asks you, "(.+)"$`)
	directedExclaimPattern = regexp.MustCompile(`^(.+?) exclaims to you, "(.+)"$`)
	directedWhisperPattern = regexp.MustCompile(`^(.+?) whispers to you, "(.+)"$`)
	
	// General patterns
	generalSayPattern     = regexp.MustCompile(`^(.+?) says, "(.+)"$`)
	generalAskPattern     = regexp.MustCompile(`^(.+?) asks, "(.+)"$`)
	generalExclaimPattern = regexp.MustCompile(`^(.+?) exclaims, "(.+)"$`)
	generalWhisperPattern = regexp.MustCompile(`^(.+?) whispers, "(.+)"$`)
	
	// Thought pattern (always directed)
	thoughtPattern = regexp.MustCompile(`^You hear the faint thoughts of (.+?) echo in your mind`)
)

// ParseSocialInteraction parses a social interaction from preset content
func ParseSocialInteraction(presetType, content, playerName string) *SocialEvent {
	if content == "" {
		return nil
	}
	
	content = strings.TrimSpace(content)
	
	// Determine subtype based on preset type and content
	var subtype string
	var matches []string
	var directedAtMe bool
	var speaker, message string
	
	switch presetType {
	case "speech":
		// Check if it's a say, ask, or exclaim
		if matches = directedSayPattern.FindStringSubmatch(content); matches != nil {
			subtype = SocialSay
			directedAtMe = true
			speaker = matches[1]
			message = matches[2]
		} else if matches = directedAskPattern.FindStringSubmatch(content); matches != nil {
			subtype = SocialAsk
			directedAtMe = true
			speaker = matches[1]
			message = matches[2]
		} else if matches = directedExclaimPattern.FindStringSubmatch(content); matches != nil {
			subtype = SocialExclaim
			directedAtMe = true
			speaker = matches[1]
			message = matches[2]
		} else if matches = generalSayPattern.FindStringSubmatch(content); matches != nil {
			subtype = SocialSay
			directedAtMe = false
			speaker = matches[1]
			message = matches[2]
		} else if matches = generalAskPattern.FindStringSubmatch(content); matches != nil {
			subtype = SocialAsk
			directedAtMe = false
			speaker = matches[1]
			message = matches[2]
		} else if matches = generalExclaimPattern.FindStringSubmatch(content); matches != nil {
			subtype = SocialExclaim
			directedAtMe = false
			speaker = matches[1]
			message = matches[2]
		}
		
	case "whisper":
		if matches = directedWhisperPattern.FindStringSubmatch(content); matches != nil {
			subtype = SocialWhisper
			directedAtMe = true
			speaker = matches[1]
			message = matches[2]
		} else if matches = generalWhisperPattern.FindStringSubmatch(content); matches != nil {
			subtype = SocialWhisper
			directedAtMe = false
			speaker = matches[1]
			message = matches[2]
		}
		
	case "thought":
		if matches = thoughtPattern.FindStringSubmatch(content); matches != nil {
			subtype = SocialThought
			directedAtMe = true // Thoughts are always directed
			speaker = matches[1]
			message = content // Full thought message
		}
	}
	
	// If we couldn't parse it, return nil
	if speaker == "" {
		return nil
	}
	
	return &SocialEvent{
		Type:         "social_interaction",
		Subtype:      subtype,
		From:         speaker,
		FromSimple:   extractSimpleName(speaker),
		Message:      message,
		DirectedAtMe: directedAtMe,
		Timestamp:    time.Now().Unix(),
		Raw:          content,
	}
}

// extractSimpleName removes titles and suffixes from a character name
func extractSimpleName(fullName string) string {
	// Remove common titles at the start
	titles := []string{"Lord", "Lady", "Sir", "Dame", "Captain", "General", "Admiral", "Commander"}
	name := fullName
	
	for _, title := range titles {
		if strings.HasPrefix(name, title+" ") {
			name = strings.TrimPrefix(name, title+" ")
			break
		}
	}
	
	// Remove suffix after comma (e.g., "Eldara, the Battle Mage" -> "Eldara")
	if idx := strings.Index(name, ","); idx != -1 {
		name = name[:idx]
	}
	
	// Trim any extra whitespace
	return strings.TrimSpace(name)
}