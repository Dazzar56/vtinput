//go:build windows

package vtinput

import (
	"io"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

var procReadConsoleInputW = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReadConsoleInputW")

type inputRecord struct {
	EventType uint16
	_         uint16
	Event     [16]byte
}

func NewReader(in io.Reader) *Reader {
	r := &Reader{
		in:              in,
		buf:             make([]byte, 0, 128),
		dataChan:        make(chan byte, 1024),
		NativeEventChan: make(chan *InputEvent, 1024),
		errChan:         make(chan error, 1),
		done:            make(chan struct{}),
	}

	useWinAPI := true // Default on Windows
	if InputMode == "winapi" {
		useWinAPI = true
	} else if InputMode == "ansi" {
		useWinAPI = false
	}

	if useWinAPI {
		if f, ok := in.(*os.File); ok {
			handle := windows.Handle(f.Fd())
			var mode uint32
			if err := windows.GetConsoleMode(handle, &mode); err == nil {
				Log("Reader: Starting WinAPI loop for console input")
				go r.winAPILoop(handle)
				return r
			}
		}
		Log("Reader: InputMode winapi requested but input is not a console. Falling back to ANSI.")
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
				for i := 0; i < n; i++ {
					r.dataChan <- tmp[i]
				}
			}
			if err != nil {
				r.errChan <- err
				return
			}
		}
	}
}

func (r *Reader) winAPILoop(handle windows.Handle) {
	var mode uint32
	windows.GetConsoleMode(handle, &mode)
	mode |= 0x0008 | 0x0010 | 0x0080 // ENABLE_WINDOW_INPUT | ENABLE_MOUSE_INPUT | ENABLE_EXTENDED_FLAGS
	mode &^= 0x0001                  // Clear ENABLE_PROCESSED_INPUT to get raw Ctrl+C instead of signal
	windows.SetConsoleMode(handle, mode)

	records := make([]inputRecord, 128)
	for {
		select {
		case <-r.done:
			return
		default:
		}

		var numRead uint32
		ret, _, err := procReadConsoleInputW.Call(
			uintptr(handle),
			uintptr(unsafe.Pointer(&records[0])),
			uintptr(len(records)),
			uintptr(unsafe.Pointer(&numRead)),
		)

		if ret == 0 {
			r.errChan <- err
			return
		}

		for i := uint32(0); i < numRead; i++ {
			rec := records[i]
			switch rec.EventType {
			case 0x0001: // KEY_EVENT
				bKeyDown := *(*int32)(unsafe.Pointer(&rec.Event[0]))
				wRepeatCount := *(*uint16)(unsafe.Pointer(&rec.Event[4]))
				wVirtualKeyCode := *(*uint16)(unsafe.Pointer(&rec.Event[6]))
				wVirtualScanCode := *(*uint16)(unsafe.Pointer(&rec.Event[8]))
				unicodeChar := *(*uint16)(unsafe.Pointer(&rec.Event[10]))
				dwControlKeyState := *(*uint32)(unsafe.Pointer(&rec.Event[12]))

				if wVirtualKeyCode == 0 {
					continue
				}

				ev := &InputEvent{
					Type:            KeyEventType,
					VirtualKeyCode:  wVirtualKeyCode,
					VirtualScanCode: wVirtualScanCode,
					Char:            rune(unicodeChar),
					KeyDown:         bKeyDown != 0,
					ControlKeyState: dwControlKeyState,
					RepeatCount:     wRepeatCount,
					InputSource:     "winapi",
				}
				r.NativeEventChan <- ev

			case 0x0002: // MOUSE_EVENT
				dwMousePositionX := *(*int16)(unsafe.Pointer(&rec.Event[0]))
				dwMousePositionY := *(*int16)(unsafe.Pointer(&rec.Event[2]))
				dwButtonState := *(*uint32)(unsafe.Pointer(&rec.Event[4]))
				dwControlKeyState := *(*uint32)(unsafe.Pointer(&rec.Event[8]))
				dwEventFlags := *(*uint32)(unsafe.Pointer(&rec.Event[12]))

				ev := &InputEvent{
					Type:            MouseEventType,
					MouseX:          uint16(dwMousePositionX),
					MouseY:          uint16(dwMousePositionY),
					ButtonState:     dwButtonState,
					ControlKeyState: dwControlKeyState,
					MouseEventFlags: dwEventFlags,
					InputSource:     "winapi",
					KeyDown:         true,
				}

				if (dwEventFlags & MouseWheeled) != 0 {
					delta := *(*int16)(unsafe.Pointer(&rec.Event[6])) // High word
					if delta > 0 {
						ev.WheelDirection = 1
					} else if delta < 0 {
						ev.WheelDirection = -1
					}
				} else if dwEventFlags == MouseMoved {
					if dwButtonState == 0 {
						ev.KeyDown = false
					}
				} else if dwEventFlags == 0 || dwEventFlags == DoubleClick {
					if dwButtonState == 0 {
						ev.KeyDown = false
					}
				}
				r.NativeEventChan <- ev

			case 0x0004: // WINDOW_BUFFER_SIZE_EVENT
				r.NativeEventChan <- &InputEvent{Type: ResizeEventType, InputSource: "winapi"}
			}
		}
	}
}

func (r *Reader) platformClose() {
	// Revert to relying on done channel
}