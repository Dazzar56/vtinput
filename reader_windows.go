//go:build windows

package vtinput

import (
	"io"
	"os"
	"unsafe"
	"time"
	"encoding/binary"

	"golang.org/x/sys/windows"
)

var procReadConsoleInputW = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReadConsoleInputW")

type inputRecord struct {
	EventType EventType
	_         uint16
	Event     [16]byte
}

func NewReader(in io.Reader) *Reader {
	r := &Reader{
		in:              in,
		buf:             make([]byte, 0, 128),
		dataChan:        make(chan []byte, 16),
		NativeEventChan: make(chan *InputEvent, 1024),
		errChan:         make(chan error, 1),
		done:            make(chan struct{}),
	}

	Log("Reader: NewReader init, InputMode global setting: %q", InputMode)

	useConPTY := true // Default on Windows
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
				Log("Reader: Successfully identified console handle (FD %d). Mode: 0x%X", f.Fd(), mode)
				Log("Reader: Starting ConPTY loop for native event queue")
				go r.conPTYLoop(handle)
				return r
			} else {
				Log("Reader: GetConsoleMode failed for FD %d: %v", f.Fd(), err)
			}
		} else {
			Log("Reader: Input is not an *os.File, cannot use ConPTY")
		}
		Log("Reader: Falling back to ANSI byte-stream parser.")
	}

	Log("Reader: Starting ANSI byte-stream loop")
	go r.ansiLoop()
	return r
}

func (r *Reader) ansiLoop() {
	tmp := make([]byte, 1024)
	for {
		select {
		case <-r.done:
			return
		default:
			n, err := r.in.Read(tmp)
			if n > 0 {
				buf := make([]byte, n)
				copy(buf, tmp[:n])
				r.dataChan <- buf
			}
			if err != nil {
				r.errChan <- err
				return
			}
		}
	}
}

func (r *Reader) conPTYLoop(handle windows.Handle) {
	var oldMode uint32
	windows.GetConsoleMode(handle, &oldMode)

	// We need to set some flags and, crucially, CLEAR others that interfere with raw input.
	// Set: WINDOW_INPUT (0x8), MOUSE_INPUT (0x10), EXTENDED_FLAGS (0x80)
	newMode := oldMode | 0x0008 | 0x0010 | 0x0080

	// Clear:
	// 0x0001: PROCESSED_INPUT (to get raw Ctrl+C)
	// 0x0002: LINE_INPUT (get keys immediately)
	// 0x0004: ECHO_INPUT
	// 0x0040: QUICK_EDIT_MODE (to allow mouse events instead of selection)
	// 0x0200: VIRTUAL_TERMINAL_INPUT (CRITICAL: if this is ON, ReadConsoleInputW gets no keys!)
	newMode &^= (0x0001 | 0x0002 | 0x0004 | 0x0040 | 0x0200)

	if err := windows.SetConsoleMode(handle, newMode); err != nil {
		Log("Reader: ConPTY SetConsoleMode failure: %v", err)
	} else {
		Log("Reader: ConPTY ConsoleMode updated: 0x%X -> 0x%X (Cleared VT_INPUT and QuickEdit)", oldMode, newMode)
	}

	records := make([]inputRecord, 128)
	for {
		select {
		case <-r.done:
			return
		default:
		}

		var numRead uint32
		callStart := time.Now()
		ret, _, err := procReadConsoleInputW.Call(
			uintptr(handle),
			uintptr(unsafe.Pointer(&records[0])),
			uintptr(len(records)),
			uintptr(unsafe.Pointer(&numRead)),
		)
		callDur := time.Since(callStart)

		if callDur > 100*time.Millisecond && numRead > 0 {
			Log("PROFILE: WinAPI Read Wait: %v (Events: %d)", callDur, numRead)
		}

		if ret == 0 {
			r.errChan <- err
			return
		}

		for i := uint32(0); i < numRead; i++ {
			rec := records[i]
			switch rec.EventType {
			case KeyEventType: // KEY_EVENT
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
				r.NativeEventChan <- ev

			case MouseEventType: // MOUSE_EVENT
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

				if (ev.MouseEventFlags & MouseWheeled > 0) || (ev.MouseEventFlags & MouseHWheeled > 0) {
					if int16(highWord(ev.ButtonState)) > 0 {
						ev.WheelDirection = 1
					} else {
						ev.WheelDirection = -1
					}
				}
				r.NativeEventChan <- ev

			case ResizeEventType: // WINDOW_BUFFER_SIZE_EVENT
				r.NativeEventChan <- &InputEvent{Type: ResizeEventType, InputSource: "ConPTY"}
			default:
				// Log other events like FOCUS_EVENT (0x10) or MENU_EVENT (0x8)
				Log("Reader: ConPTY ignored event type 0x%04X", rec.EventType)
			}
		}
	}
}

func (r *Reader) platformClose() {
	// Revert to relying on done channel
}

func highWord(data uint32) uint16 {
	return uint16((data & 0xFFFF0000) >> 16)
}
