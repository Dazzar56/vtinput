//go:build windows

package vtinput

import (
	"encoding/binary"
	"io"
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procReadConsoleInputW = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReadConsoleInputW")

type inputRecord struct {
	EventType EventType
	_         uint16
	Event     [16]byte
}

func (r *Reader) platformInit(in io.Reader) {
	useConPTY := true
	switch InputMode {
	case "ConPTY":
		useConPTY = true
	case "ansi":
		useConPTY = false
	}

	if useConPTY {
		if f, ok := in.(*os.File); ok {
			handle := windows.Handle(f.Fd())
			var mode uint32
			if err := windows.GetConsoleMode(handle, &mode); err == nil {
				r.conHandle = uintptr(handle)
				r.oldMode = mode
				r.useConPTY = true

				// We need to set some flags and, crucially, CLEAR others that interfere with raw input.
				// Set: WINDOW_INPUT (0x8), MOUSE_INPUT (0x10), EXTENDED_FLAGS (0x80)
				newMode := mode | 0x0008 | 0x0010 | 0x0080

				// Clear:
				// 0x0001: PROCESSED_INPUT (to get raw Ctrl+C)
				// 0x0002: LINE_INPUT (get keys immediately)
				// 0x0004: ECHO_INPUT
				// 0x0040: QUICK_EDIT_MODE (to allow mouse events instead of selection)
				// 0x0200: VIRTUAL_TERMINAL_INPUT (CRITICAL: if this is ON, ReadConsoleInputW gets no keys!)
				newMode &^= (0x0001 | 0x0002 | 0x0004 | 0x0040 | 0x0200)

				if err := windows.SetConsoleMode(handle, newMode); err != nil {
					Log("Reader: ConPTY SetConsoleMode failure: %v", err)
				}

				event, err := windows.CreateEvent(nil, 0, 0, nil)
				if err != nil {
					Log("Reader: CreateEvent failed: %v", err)
				} else {
					r.cancelEvent = uintptr(event)
				}
				return
			}
		}
	}

	r.useConPTY = false
}

func (r *Reader) readBytes(buf []byte, timeout time.Duration) (int, error) {
	if timeout > 0 {
		if d, ok := r.in.(interface{ SetReadDeadline(time.Time) error }); ok {
			d.SetReadDeadline(time.Now().Add(timeout))
			defer d.SetReadDeadline(time.Time{})
		}
	}
	n, err := r.in.Read(buf)
	if n > 0 {
		if r.MetricsEnabled {
			r.lastReceivedAt = time.Now()
		}
	}
	return n, err
}

func (r *Reader) readConPTYEventTimeout(timeout time.Duration) (*InputEvent, error) {
	if r.conHandle == 0 {
		return nil, io.ErrClosedPipe
	}

	var timeoutMs uint32 = windows.INFINITE
	if timeout > 0 {
		ms := uint32(timeout.Milliseconds())
		if ms > 0 {
			timeoutMs = ms
		}
	}

	handles := []windows.Handle{windows.Handle(r.conHandle)}
	if r.cancelEvent != 0 {
		handles = append(handles, windows.Handle(r.cancelEvent))
	}

	ret, err := windows.WaitForMultipleObjects(handles, false, timeoutMs)
	if err != nil {
		return nil, err
	}
	if ret == 0x00000102 { // WAIT_TIMEOUT
		return nil, nil
	}
	if len(handles) > 1 && ret == windows.WAIT_OBJECT_0+1 {
		return nil, io.EOF
	}

	var numRead uint32
	var rec inputRecord
	ret2, _, err := procReadConsoleInputW.Call(
		uintptr(r.conHandle),
		uintptr(unsafe.Pointer(&rec)),
		1,
		uintptr(unsafe.Pointer(&numRead)),
	)
	if ret2 == 0 {
		return nil, err
	}
	if numRead == 0 {
		return nil, nil
	}

	if r.MetricsEnabled {
		r.lastReceivedAt = time.Now()
	}

	switch rec.EventType {
	case 0x0001: // KEY_EVENT
		ev := &InputEvent{
			Type:            KeyEventType,
			KeyDown:         binary.LittleEndian.Uint32(rec.Event[0:4]) > 0,
			RepeatCount:     binary.LittleEndian.Uint16(rec.Event[4:6]),
			VirtualKeyCode:  binary.LittleEndian.Uint16(rec.Event[6:8]),
			VirtualScanCode: binary.LittleEndian.Uint16(rec.Event[8:10]),
			Char:            rune(binary.LittleEndian.Uint16(rec.Event[10:12])),
			ControlKeyState: ControlKeyState(binary.LittleEndian.Uint32(rec.Event[12:16])),
			InputSource:     "ConPTY",
		}
		r.recordLatency(time.Since(r.lastReceivedAt))
		return ev, nil

	case 0x0002: // MOUSE_EVENT
		ev := &InputEvent{
			Type:            MouseEventType,
			MouseX:          binary.LittleEndian.Uint16(rec.Event[0:2]),
			MouseY:          binary.LittleEndian.Uint16(rec.Event[2:4]),
			ButtonState:     binary.LittleEndian.Uint32(rec.Event[4:8]),
			ControlKeyState: ControlKeyState(binary.LittleEndian.Uint32(rec.Event[8:12])),
			MouseEventFlags: binary.LittleEndian.Uint32(rec.Event[12:16]),
			InputSource:     "ConPTY",
			KeyDown:         true,
		}
		if (ev.MouseEventFlags&MouseWheeled > 0) || (ev.MouseEventFlags&MouseHWheeled > 0) {
			if int16(highWord(ev.ButtonState)) > 0 {
				ev.WheelDirection = 1
			} else {
				ev.WheelDirection = -1
			}
		}
		r.recordLatency(time.Since(r.lastReceivedAt))
		return ev, nil

	case 0x0004: // WINDOW_BUFFER_SIZE_EVENT
		r.recordLatency(time.Since(r.lastReceivedAt))
		return &InputEvent{Type: ResizeEventType, InputSource: "ConPTY"}, nil

	case 0x0010: // FOCUS_EVENT
		setFocus := binary.LittleEndian.Uint32(rec.Event[0:4]) > 0
		r.recordLatency(time.Since(r.lastReceivedAt))
		return &InputEvent{Type: FocusEventType, SetFocus: setFocus, InputSource: "ConPTY"}, nil

	default:
		return nil, nil
	}
}

func (r *Reader) platformClose() {
	if r.cancelEvent != 0 {
		windows.SetEvent(windows.Handle(r.cancelEvent))
	}
	if r.conHandle != 0 {
		windows.CancelIoEx(windows.Handle(r.conHandle), nil)
		if r.oldMode != 0 {
			windows.SetConsoleMode(windows.Handle(r.conHandle), r.oldMode)
		}
	}
}

func highWord(data uint32) uint16 {
	return uint16((data & 0xFFFF0000) >> 16)
}
