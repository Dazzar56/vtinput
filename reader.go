package vtinput

import (
	"io"
	"time"
	"unicode/utf8"
)

// Reader wraps an io.Reader (like os.Stdin) and parses input events.
// It buffers input internally to handle incomplete escape sequences.
type Reader struct {
	in       io.Reader
	buf      []byte
	dataChan chan byte
	errChan  chan error
	done     chan struct{}
	stopPipe [2]int // Used on Unix for Select unblocking
}

// Close stops the background reading goroutine instantly.
func (r *Reader) Close() {
	select {
	case <-r.done:
		return
	default:
		close(r.done)
		r.platformClose()
	}
}

// ReadEvent reads the next input event.
func (r *Reader) ReadEvent() (*InputEvent, error) {
	for {
		select {
		case <-r.done:
			return nil, io.EOF
		default:
		}

		// Greedy drain: pull all currently available bytes from the channel
		// into the buffer before attempting to parse. This is crucial for
		// correctly handling multi-byte sequences and workarounds.
	greedy:
		for {
			select {
			case b := <-r.dataChan:
				r.buf = append(r.buf, b)
			default:
				break greedy
			}
		}

		if len(r.buf) > 0 {
			// Optimization: Only attempt to parse sequences if the buffer starts with ESC.
			if r.buf[0] == 0x1B {
				// 1. Handle SS3 sequences (ESC O ...)
				if event, consumed, err := ParseLegacySS3(r.buf); err == nil {
					r.buf = r.buf[consumed:]
					return event, nil
				} else if err == ErrIncomplete {
					goto waitForMore
				}

				// 2. Handle CSI sequences (ESC [ ...)
				if terminatorIdx, command, err := scanCSI(r.buf); err == nil {
					var event *InputEvent
					var consumed int
					var pErr error

					if command == 'I' && terminatorIdx == 2 {
						event, consumed = &InputEvent{Type: FocusEventType, SetFocus: true}, 3
					} else if command == 'O' && terminatorIdx == 2 {
						event, consumed = &InputEvent{Type: FocusEventType, SetFocus: false}, 3
					} else if command == '~' && string(r.buf[2:terminatorIdx]) == "200" {
						event, consumed = &InputEvent{Type: PasteEventType, PasteStart: true}, terminatorIdx+1
					} else if command == '~' && string(r.buf[2:terminatorIdx]) == "201" {
						event, consumed = &InputEvent{Type: PasteEventType, PasteStart: false}, terminatorIdx+1
					} else {
						switch command {
						case '_': // Win32 Input Mode
							event, consumed, pErr = ParseWin32InputEvent(r.buf)
						case 'M', 'm': // SGR Mouse
							event, consumed, pErr = ParseMouseSGR(r.buf)
						default: // Kitty Protocol or Legacy CSI
							event, consumed, pErr = ParseKitty(r.buf)
							if pErr == ErrInvalidSequence {
								event, consumed, pErr = ParseLegacyCSI(r.buf)
							}
						}
					}

					if pErr == nil && event != nil {
						r.buf = r.buf[consumed:]
						return event, nil
					}
				} else if err == ErrIncomplete {
					goto waitForMore
				}

				// 3. Handle Double ESC
				if len(r.buf) >= 2 && r.buf[1] == 0x1B {
					r.buf = r.buf[2:]
					return &InputEvent{Type: KeyEventType, VirtualKeyCode: VK_ESCAPE, KeyDown: true}, nil
				}

				// 4. Handle Legacy Alt (ESC + Char)
				if len(r.buf) >= 2 && utf8.FullRune(r.buf[1:]) {
					r.buf = r.buf[1:]
					character, size := utf8.DecodeRune(r.buf)
					r.buf = r.buf[size:]
					return &InputEvent{
						Type:            KeyEventType,
						Char:            character,
						ControlKeyState: LeftAltPressed,
						KeyDown:         true,
						IsLegacy:        true,
					}, nil
				}

			waitForMore:
				select {
				case b := <-r.dataChan:
					r.buf = append(r.buf, b)
					continue
				case <-time.After(100 * time.Millisecond):
					r.buf = r.buf[1:]
					return &InputEvent{Type: KeyEventType, VirtualKeyCode: VK_ESCAPE, KeyDown: true}, nil
				case err := <-r.errChan:
					// Drain any data that arrived before the error
				drain1:
					for {
						select {
						case b := <-r.dataChan:
							r.buf = append(r.buf, b)
						default:
							break drain1
						}
					}
					if len(r.buf) == 0 {
						return nil, err
					}
					continue
				}
			}

			if r.buf[0] == 0x7F {
				r.buf = r.buf[1:]
				return &InputEvent{Type: KeyEventType, VirtualKeyCode: VK_BACK, KeyDown: true, IsLegacy: true}, nil
			}

			if utf8.FullRune(r.buf) {
				character, size := utf8.DecodeRune(r.buf)
				consumed := size

				// Far2l workaround: some versions of far2l's terminal emulator
				// send an extra space after the '=' character.
				if character == '=' && len(r.buf) > size && r.buf[size] == ' ' {
					consumed++
				}

				r.buf = r.buf[consumed:]
				if event := translateLegacyByte(character); event != nil {
					return event, nil
				}
				return &InputEvent{Type: KeyEventType, Char: character, KeyDown: true, IsLegacy: true}, nil
			}
		}

		select {
		case b := <-r.dataChan:
			r.buf = append(r.buf, b)
		case err := <-r.errChan:
			// Prioritize data over error to avoid premature EOF
		drain2:
			for {
				select {
				case b := <-r.dataChan:
					r.buf = append(r.buf, b)
				default:
					break drain2
				}
			}
			if len(r.buf) == 0 {
				return nil, err
			}
		}
	}
}

func translateLegacyByte(r rune) *InputEvent {
	evt := &InputEvent{Type: KeyEventType, KeyDown: true, IsLegacy: true}
	switch r {
	case 0x00:
		evt.VirtualKeyCode = VK_SPACE
		evt.Char = ' '
		evt.ControlKeyState = LeftCtrlPressed
		return evt
	case 0x08:
		evt.VirtualKeyCode = VK_BACK
		return evt
	case 0x09:
		evt.VirtualKeyCode = VK_TAB
		evt.Char = '\t'
		return evt
	case 0x0D:
		evt.VirtualKeyCode = VK_RETURN
		evt.Char = '\r'
		return evt
	case 0x1B:
		evt.VirtualKeyCode = VK_ESCAPE
		return evt
	case 0x1C:
		evt.VirtualKeyCode = VK_OEM_5
		evt.ControlKeyState = LeftCtrlPressed
		return evt
	case 0x1D:
		evt.VirtualKeyCode = VK_OEM_6
		evt.ControlKeyState = LeftCtrlPressed
		return evt
	case 0x1E:
		evt.VirtualKeyCode = VK_6
		evt.ControlKeyState = LeftCtrlPressed
		return evt
	case 0x1F:
		evt.VirtualKeyCode = VK_OEM_MINUS
		evt.ControlKeyState = LeftCtrlPressed
		return evt
	}
	if r >= 1 && r <= 26 {
		evt.VirtualKeyCode = uint16(VK_A + (r - 1))
		evt.ControlKeyState = LeftCtrlPressed
		return evt
	}
	return nil
}