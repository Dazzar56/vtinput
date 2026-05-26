package vtinput

import "github.com/unxed/winkeys"

// Re-export core types for backward compatibility using Go type aliases.
type ControlKeyState = winkeys.ControlKeyState
type EventType = winkeys.EventType
type InputEvent = winkeys.InputEvent

// InputMode defines the preferred input parser method. Valid values: "", "ansi", "winapi".
var InputMode string

// Re-export modifier and mouse constants.
const (
	NoKeyPressed     = winkeys.NoKeyPressed
	RightAltPressed  = winkeys.RightAltPressed
	LeftAltPressed   = winkeys.LeftAltPressed
	RightCtrlPressed = winkeys.RightCtrlPressed
	LeftCtrlPressed  = winkeys.LeftCtrlPressed
	ShiftPressed     = winkeys.ShiftPressed
	NumLockOn        = winkeys.NumLockOn
	ScrollLockOn     = winkeys.ScrollLockOn
	CapsLockOn       = winkeys.CapsLockOn
	EnhancedKey      = winkeys.EnhancedKey

	FromLeft1stButtonPressed = winkeys.FromLeft1stButtonPressed
	RightmostButtonPressed   = winkeys.RightmostButtonPressed
	FromLeft2ndButtonPressed = winkeys.FromLeft2ndButtonPressed
	FromLeft3rdButtonPressed = winkeys.FromLeft3rdButtonPressed
	FromLeft4thButtonPressed = winkeys.FromLeft4thButtonPressed

	MouseMoved    = winkeys.MouseMoved
	DoubleClick   = winkeys.DoubleClick
	MouseWheeled  = winkeys.MouseWheeled
	MouseHWheeled = winkeys.MouseHWheeled

	KeyEventType    = winkeys.KeyEventType
	MouseEventType  = winkeys.MouseEventType
	FocusEventType  = winkeys.FocusEventType
	PasteEventType  = winkeys.PasteEventType
	Far2lEventType  = winkeys.Far2lEventType
	ResizeEventType = winkeys.ResizeEventType
)
