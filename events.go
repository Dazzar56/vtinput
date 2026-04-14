package vtinput

import (
	"fmt"
	"strings"
)

// ControlKeyState flags (dwControlKeyState)
type ControlKeyState uint32

const (
	NoKeyPressed     ControlKeyState = 0x0000
	RightAltPressed  ControlKeyState = 0x0001
	LeftAltPressed   ControlKeyState = 0x0002
	RightCtrlPressed ControlKeyState = 0x0004
	LeftCtrlPressed  ControlKeyState = 0x0008
	ShiftPressed     ControlKeyState = 0x0010
	NumLockOn        ControlKeyState = 0x0020
	ScrollLockOn     ControlKeyState = 0x0040
	CapsLockOn       ControlKeyState = 0x0080
	EnhancedKey      ControlKeyState = 0x0100
)

// Mouse Button States (dwButtonState)
const (
	FromLeft1stButtonPressed = 0x0001
	RightmostButtonPressed   = 0x0002
	FromLeft2ndButtonPressed = 0x0004
	FromLeft3rdButtonPressed = 0x0008
	FromLeft4thButtonPressed = 0x0010
)

// Mouse Event Flags (dwEventFlags)
const (
	MouseMoved      = 0x0001
	DoubleClick     = 0x0002
	MouseWheeled    = 0x0004
	MouseHWheeled   = 0x0008
)

// InputMode defines the preferred input parser method. Valid values: "", "ansi", "winapi".
var InputMode string

// EventType constants
type EventType uint16

const (
	KeyEventType   EventType = 0x0001
	MouseEventType EventType = 0x0002
	FocusEventType EventType = 0x0010
	PasteEventType EventType = 0x0020
	Far2lEventType EventType = 0x0040
	ResizeEventType EventType = 0x0080
)

// InputEvent is a generic container for any event (Key, Mouse, Focus).
// Currently, our parser only produces Key events, but the structure is ready for more.
type InputEvent struct {
	Type EventType

	// Key Event Data
	VirtualKeyCode  uint16
	VirtualScanCode uint16
	Char            rune
	UnshiftedChar   rune
	KeyDown         bool
	RepeatCount     uint16

	// Mouse Event Data (Future proofing)
	MouseX          uint16
	MouseY          uint16
	ButtonState     uint32
	MouseEventFlags uint32
	WheelDirection  int // 1 (forward/right), -1 (backward/left)

	// Focus Event Data
	SetFocus bool

	// Paste Event Data
	PasteStart bool
	// Far2l Extension Event Data
	Far2lCommand string
	Far2lData    []byte

	// Shared
	ControlKeyState ControlKeyState

	// InputSource tracks which parser generated this event (e.g., "win32", "kitty", "legacy_csi").
	// Extremely useful for debugging terminal compatibilities.
	InputSource string

	// IsLegacy indicates that this event comes from a protocol that does not support
	// explicit KeyUp events (e.g. standard ANSI). The application may need to
	// simulate KeyUp after a timeout.
	IsLegacy bool
}

// String ensures that ControlKeyState satisfies the fmt.Stringer interface.
func (cks ControlKeyState) String() string {
	controlKeys := strings.Builder{}

	if cks == NoKeyPressed {
		controlKeys.WriteString("None")
	} else {
		if cks.Contains(RightAltPressed)  {controlKeys.WriteString("RightAlt,")}
		if cks.Contains(LeftAltPressed)   {controlKeys.WriteString("Alt,")}
		if cks.Contains(RightCtrlPressed) {controlKeys.WriteString("RightCtrl,")}
		if cks.Contains(LeftCtrlPressed)  {controlKeys.WriteString("Ctrl,")}
		if cks.Contains(ShiftPressed)     {controlKeys.WriteString("Shift,")}
		if cks.Contains(NumLockOn)        {controlKeys.WriteString("NumLock,")}
		if cks.Contains(ScrollLockOn)     {controlKeys.WriteString("ScrollLock,")}
		if cks.Contains(CapsLockOn)       {controlKeys.WriteString("CapsLock,")}
		if cks.Contains(EnhancedKey)      {controlKeys.WriteString("Enhanced,")}
	}

	return strings.TrimRight(controlKeys.String(), ",")
}

// Contains is little helper for Control State check
func (cks ControlKeyState) Contains(state ControlKeyState) bool {
	return cks&state != 0
}

// String implements the Stringer interface for easy debugging.
func (e InputEvent) String() string {
	legacyStr := ""
	if e.IsLegacy {
		legacyStr = " [Legacy]"
	}

	if e.Type == KeyEventType {
		state := "UP"
		if e.KeyDown {
			state = "DOWN"
		}
		charStr := ""
		if e.Char > 0 {
			if e.Char < 32 {
				charStr = fmt.Sprintf(" Char:\\x%02X", e.Char)
			} else {
				charStr = fmt.Sprintf(" Char:'%c'", e.Char)
			}
		}

		baseStr := ""
		if e.UnshiftedChar > 0 {
			if e.UnshiftedChar < 32 {
				baseStr = fmt.Sprintf(" Base:\\x%02X", e.UnshiftedChar)
			} else {
				baseStr = fmt.Sprintf(" Base:'%c'", e.UnshiftedChar)
			}
		}

		return fmt.Sprintf("Key{VK:0x%X Scan:0x%X%s%s %s Mods:%s Src:%s}%s",
			e.VirtualKeyCode, e.VirtualScanCode, charStr, baseStr, state, e.ControlKeyState.String(), e.InputSource, legacyStr)
	}

	if e.Type == MouseEventType {
		btn := "None"
		switch e.ButtonState {
		case FromLeft1stButtonPressed:
			btn = "Left"
		case FromLeft2ndButtonPressed:
			btn = "Middle"
		case RightmostButtonPressed:
			btn = "Right"
		}

		action := "UP"
		if e.KeyDown {
			action = "DOWN"
		}
		if (e.MouseEventFlags & MouseMoved) != 0 {
			action = "MOVE"
		}

		wheel := ""
		if e.WheelDirection > 0 {
			wheel = " WHEEL_UP"
		}
		if e.WheelDirection < 0 {
			wheel = " WHEEL_DOWN"
		}

		return fmt.Sprintf("Mouse{Pos:%d,%d Btn:%s %s%s Mods:%s Src:%s}%s",
			e.MouseX, e.MouseY, btn, action, wheel, e.ControlKeyState.String(), e.InputSource, legacyStr)
	}

	if e.Type == FocusEventType {
		state := "OUT"
		if e.SetFocus {
			state = "IN"
		}
		return fmt.Sprintf("Focus{%s Src:%s}", state, e.InputSource)
	}

	if e.Type == PasteEventType {
		state := "END"
		if e.PasteStart {
			state = "START"
		}
		return fmt.Sprintf("Paste{%s Src:%s}", state, e.InputSource)
	}
	if e.Type == Far2lEventType {
		return fmt.Sprintf("Far2l{%s len:%d}", e.Far2lCommand, len(e.Far2lData))
	}
	if e.Type == ResizeEventType {
		return "TerminalResized{}"
	}

	return fmt.Sprintf("Event{Type:%d Mods:0x%X}%s", e.Type, e.ControlKeyState, legacyStr)
}
