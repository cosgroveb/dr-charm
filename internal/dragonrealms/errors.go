package dragonrealms

import "errors"

var (
	ErrUnavailable    = errors.New("DragonRealms session unavailable")
	ErrClosed         = errors.New("DragonRealms session closed")
	ErrInvalidCommand = errors.New("invalid DragonRealms command")

	errCharacterNotFound = errors.New("DragonRealms character not found")
)

type sgeAuthError struct {
	kind string
}

func (e *sgeAuthError) Error() string {
	switch e.kind {
	case "password":
		return "DragonRealms authentication rejected the password"
	case "unknown":
		return "DragonRealms authentication rejected the login"
	case "problem-1":
		return "DragonRealms account cannot access the game"
	case "problem-2":
		return "DragonRealms character is already logged in or the login mode was rejected"
	case "problem-3":
		return "DragonRealms character is currently in game"
	case "problem-4":
		return "DragonRealms is unavailable"
	default:
		return "DragonRealms authentication failed"
	}
}
