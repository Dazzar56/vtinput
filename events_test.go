package vtinput

import (
	"strings"
	"testing"
)

func TestInputEvent_String(t *testing.T) {
	// Test Keyboard Event formatting
	eKey := InputEvent{
		Type:           KeyEventType,
		VirtualKeyCode: VK_A,
		Char:           'a',
		KeyDown:        true,
		IsLegacy:       true,
		InputSource:    "test_kb",
	}
	strKey := eKey.String()
	if !strings.Contains(strKey, "Key{VK:0x41") || !strings.Contains(strKey, "Char:'a'") || !strings.Contains(strKey, "Src:test_kb") || !strings.Contains(strKey, "[Legacy]") {
		t.Errorf("Unexpected string output for KeyEvent: %s", strKey)
	}

	// Test Mouse Event formatting
	eMouse := InputEvent{
		Type:        MouseEventType,
		MouseX:      10,
		MouseY:      20,
		ButtonState: FromLeft1stButtonPressed,
		KeyDown:     true,
		InputSource: "test_mouse",
	}
	strMouse := eMouse.String()
	if !strings.Contains(strMouse, "Mouse{Pos:10,20") || !strings.Contains(strMouse, "Btn:Left") || !strings.Contains(strMouse, "Src:test_mouse") {
		t.Errorf("Unexpected string output for MouseEvent: %s", strMouse)
	}

	// Test Focus Event formatting
	eFocus := InputEvent{
		Type:        FocusEventType,
		SetFocus:    true,
		InputSource: "test_focus",
	}
	if eFocus.String() != "Focus{IN Src:test_focus}" {
		t.Errorf("Unexpected string output for FocusEvent: %s", eFocus.String())
	}
}