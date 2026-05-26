package vtinput

import "github.com/unxed/winkeys"

// Forward virtual key code constants to the new winkeys package
const (
	VK_LBUTTON    = winkeys.VK_LBUTTON
	VK_RBUTTON    = winkeys.VK_RBUTTON
	VK_CANCEL     = winkeys.VK_CANCEL
	VK_MBUTTON    = winkeys.VK_MBUTTON
	VK_XBUTTON1   = winkeys.VK_XBUTTON1
	VK_XBUTTON2   = winkeys.VK_XBUTTON2
	VK_BACK       = winkeys.VK_BACK
	VK_TAB        = winkeys.VK_TAB
	VK_CLEAR      = winkeys.VK_CLEAR
	VK_RETURN     = winkeys.VK_RETURN
	VK_SHIFT      = winkeys.VK_SHIFT
	VK_CONTROL    = winkeys.VK_CONTROL
	VK_MENU       = winkeys.VK_MENU
	VK_PAUSE      = winkeys.VK_PAUSE
	VK_CAPITAL    = winkeys.VK_CAPITAL
	VK_KANA       = winkeys.VK_KANA
	VK_HANGUL     = winkeys.VK_HANGUL
	VK_IME_ON     = winkeys.VK_IME_ON
	VK_JUNJA      = winkeys.VK_JUNJA
	VK_FINAL      = winkeys.VK_FINAL
	VK_HANJA      = winkeys.VK_HANJA
	VK_KANJI      = winkeys.VK_KANJI
	VK_IME_OFF    = winkeys.VK_IME_OFF
	VK_CONVERT    = winkeys.VK_CONVERT
	VK_NONCONVERT = winkeys.VK_NONCONVERT
	VK_ACCEPT     = winkeys.VK_ACCEPT
	VK_MODECHANGE = winkeys.VK_MODECHANGE
	VK_ESCAPE     = winkeys.VK_ESCAPE
	VK_SPACE      = winkeys.VK_SPACE
	VK_PRIOR      = winkeys.VK_PRIOR
	VK_NEXT       = winkeys.VK_NEXT
	VK_END        = winkeys.VK_END
	VK_HOME       = winkeys.VK_HOME
	VK_LEFT       = winkeys.VK_LEFT
	VK_UP         = winkeys.VK_UP
	VK_RIGHT      = winkeys.VK_RIGHT
	VK_DOWN       = winkeys.VK_DOWN
	VK_SELECT     = winkeys.VK_SELECT
	VK_PRINT      = winkeys.VK_PRINT
	VK_EXECUTE    = winkeys.VK_EXECUTE
	VK_SNAPSHOT   = winkeys.VK_SNAPSHOT
	VK_INSERT     = winkeys.VK_INSERT
	VK_DELETE     = winkeys.VK_DELETE
	VK_HELP       = winkeys.VK_HELP

	VK_0 = winkeys.VK_0
	VK_1 = winkeys.VK_1
	VK_2 = winkeys.VK_2
	VK_3 = winkeys.VK_3
	VK_4 = winkeys.VK_4
	VK_5 = winkeys.VK_5
	VK_6 = winkeys.VK_6
	VK_7 = winkeys.VK_7
	VK_8 = winkeys.VK_8
	VK_9 = winkeys.VK_9

	VK_A = winkeys.VK_A
	VK_B = winkeys.VK_B
	VK_C = winkeys.VK_C
	VK_D = winkeys.VK_D
	VK_E = winkeys.VK_E
	VK_F = winkeys.VK_F
	VK_G = winkeys.VK_G
	VK_H = winkeys.VK_H
	VK_I = winkeys.VK_I
	VK_J = winkeys.VK_J
	VK_K = winkeys.VK_K
	VK_L = winkeys.VK_L
	VK_M = winkeys.VK_M
	VK_N = winkeys.VK_N
	VK_O = winkeys.VK_O
	VK_P = winkeys.VK_P
	VK_Q = winkeys.VK_Q
	VK_R = winkeys.VK_R
	VK_S = winkeys.VK_S
	VK_T = winkeys.VK_T
	VK_U = winkeys.VK_U
	VK_V = winkeys.VK_V
	VK_W = winkeys.VK_W
	VK_X = winkeys.VK_X
	VK_Y = winkeys.VK_Y
	VK_Z = winkeys.VK_Z

	VK_LWIN       = winkeys.VK_LWIN
	VK_RWIN       = winkeys.VK_RWIN
	VK_APPS       = winkeys.VK_APPS
	VK_SLEEP      = winkeys.VK_SLEEP
	VK_NUMPAD0    = winkeys.VK_NUMPAD0
	VK_NUMPAD1    = winkeys.VK_NUMPAD1
	VK_NUMPAD2    = winkeys.VK_NUMPAD2
	VK_NUMPAD3    = winkeys.VK_NUMPAD3
	VK_NUMPAD4    = winkeys.VK_NUMPAD4
	VK_NUMPAD5    = winkeys.VK_NUMPAD5
	VK_NUMPAD6    = winkeys.VK_NUMPAD6
	VK_NUMPAD7    = winkeys.VK_NUMPAD7
	VK_NUMPAD8    = winkeys.VK_NUMPAD8
	VK_NUMPAD9    = winkeys.VK_NUMPAD9
	VK_MULTIPLY   = winkeys.VK_MULTIPLY
	VK_ADD        = winkeys.VK_ADD
	VK_SEPARATOR  = winkeys.VK_SEPARATOR
	VK_SUBTRACT   = winkeys.VK_SUBTRACT
	VK_DECIMAL    = winkeys.VK_DECIMAL
	VK_DIVIDE     = winkeys.VK_DIVIDE
	VK_F1         = winkeys.VK_F1
	VK_F2         = winkeys.VK_F2
	VK_F3         = winkeys.VK_F3
	VK_F4         = winkeys.VK_F4
	VK_F5         = winkeys.VK_F5
	VK_F6         = winkeys.VK_F6
	VK_F7         = winkeys.VK_F7
	VK_F8         = winkeys.VK_F8
	VK_F9         = winkeys.VK_F9
	VK_F10        = winkeys.VK_F10
	VK_F11        = winkeys.VK_F11
	VK_F12        = winkeys.VK_F12
	VK_F13        = winkeys.VK_F13
	VK_F14        = winkeys.VK_F14
	VK_F15        = winkeys.VK_F15
	VK_F16        = winkeys.VK_F16
	VK_F17        = winkeys.VK_F17
	VK_F18        = winkeys.VK_F18
	VK_F19        = winkeys.VK_F19
	VK_F20        = winkeys.VK_F20
	VK_F21        = winkeys.VK_F21
	VK_F22        = winkeys.VK_F22
	VK_F23        = winkeys.VK_F23
	VK_F24        = winkeys.VK_F24
	VK_NUMLOCK    = winkeys.VK_NUMLOCK
	VK_SCROLL     = winkeys.VK_SCROLL
	VK_LSHIFT     = winkeys.VK_LSHIFT
	VK_RSHIFT     = winkeys.VK_RSHIFT
	VK_LCONTROL   = winkeys.VK_LCONTROL
	VK_RCONTROL   = winkeys.VK_RCONTROL
	VK_LMENU      = winkeys.VK_LMENU
	VK_RMENU      = winkeys.VK_RMENU
	VK_OEM_1      = winkeys.VK_OEM_1
	VK_OEM_PLUS   = winkeys.VK_OEM_PLUS
	VK_OEM_COMMA  = winkeys.VK_OEM_COMMA
	VK_OEM_MINUS  = winkeys.VK_OEM_MINUS
	VK_OEM_PERIOD = winkeys.VK_OEM_PERIOD
	VK_OEM_2      = winkeys.VK_OEM_2
	VK_OEM_3      = winkeys.VK_OEM_3
	VK_OEM_4      = winkeys.VK_OEM_4
	VK_OEM_5      = winkeys.VK_OEM_5
	VK_OEM_6      = winkeys.VK_OEM_6
	VK_OEM_7      = winkeys.VK_OEM_7
	VK_OEM_102    = winkeys.VK_OEM_102
	VK_UNASSIGNED = winkeys.VK_UNASSIGNED

	ScanCodeLeftShift  = winkeys.ScanCodeLeftShift
	ScanCodeRightShift = winkeys.ScanCodeRightShift
)

// VKString outputs mapped names from winkeys implementation
func VKString(vk uint16) string {
	return winkeys.VKString(vk)
}
