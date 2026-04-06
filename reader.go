package vtinput

import (
	"io"
	"time"
	"unicode/utf8"
)

// Reader wraps an io.Reader (like os.Stdin) and parses input events.
// It buffers input internally to handle incomplete escape sequences.
type Reader struct {
	in                     io.Reader
	buf                    []byte
	dataChan               chan byte
	errChan                chan error
	done                   chan struct{}
	stopPipe               [2]int // Used on Unix for Select unblocking
	far2lExtensionsEnabled bool
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
	return r.ReadEventTimeout(0)
}

// ReadEventTimeout reads the next input event with an optional maximum blocking time.
func (r *Reader) ReadEventTimeout(timeout time.Duration) (*InputEvent, error) {
	var timer <-chan time.Time
	if timeout > 0 {
		timer = time.After(timeout)
	}

	for {
		select {
		case <-r.done:
			Log("Reader: Done signal received, exiting.")
			return nil, io.EOF
		case <-timer:
			Log("Reader: Timeout (%.2fms) reached, no event.", float64(timeout)/float64(time.Millisecond))
			return nil, nil // Timeout
		default:
		}

		// Greedy drain
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
			if r.buf[0] == 0x1B {
				// 1. Focus
				if len(r.buf) >= 3 && r.buf[1] == '[' && (r.buf[2] == 'I' || r.buf[2] == 'O') {
					event := &InputEvent{Type: FocusEventType, SetFocus: r.buf[2] == 'I'}
					r.buf = r.buf[3:]
					Log("Reader: Parsed Focus %v.", event.SetFocus)
					return event, nil
				}

				// 2. DSR Replies (ESC [ ... n)
				if len(r.buf) > 2 && r.buf[1] == '[' {
					if terminatorIdx, cmd, err := scanCSI(r.buf); err == nil {
						if cmd == 'n' {
							r.buf = r.buf[terminatorIdx+1:]
							Log("Reader: Parsed DSR reply. Continuing.")
							continue
						}
					} else if err == ErrIncomplete {
						goto waitForMore
					}
				}

				// 3. APC (Far2l)
				if len(r.buf) > 1 && r.buf[1] == '_' {
					Log("Reader: Attempting ParseFar2lAPC...")
					if event, consumed, err := ParseFar2lAPC(r.buf); err == nil {
						Log("Reader: ParseFar2lAPC successful, consumed %d bytes.", consumed)
						r.buf = r.buf[consumed:]
						if event != nil {
							if event.Type == Far2lEventType && event.Far2lCommand == "ok" {
								r.far2lExtensionsEnabled = true
								Log("Reader: Far2l extensions successfully negotiated with host.")
							}
							Log("Reader: Returning event: %s", event.String())
							return event, nil
						}
						Log("Reader: Parsed Far2l APC was ignored. Continuing.")
						continue
					} else if err == ErrIncomplete {
						goto waitForMore
					}
				}

				// 4. Bracketed Paste (ESC [ 2 0 0 ~ / ESC [ 2 0 1 ~)
				if len(r.buf) >= 6 && r.buf[1] == '[' && r.buf[2] == '2' && r.buf[3] == '0' && (r.buf[4] == '0' || r.buf[4] == '1') && r.buf[5] == '~' {
					event := &InputEvent{Type: PasteEventType, PasteStart: r.buf[4] == '0'}
					r.buf = r.buf[6:]
					Log("Reader: Parsed Paste event.")
					return event, nil
				}

				// 5. SGR Mouse (ESC [ < ... M/m)
				if len(r.buf) > 3 && r.buf[1] == '[' && r.buf[2] == '<' {
					Log("Reader: Attempting ParseMouseSGR...")
					if event, consumed, err := ParseMouseSGR(r.buf); err == nil {
						Log("Reader: ParseMouseSGR successful.")
						r.buf = r.buf[consumed:]
						return event, nil
					} else if err == ErrIncomplete {
						goto waitForMore
					}
				}

				// 6. SS3 Sequences (ESC O ...)
				if len(r.buf) > 1 && r.buf[1] == 'O' {
					Log("Reader: Attempting ParseLegacySS3...")
					if event, consumed, err := ParseLegacySS3(r.buf); err == nil {
						Log("Reader: ParseLegacySS3 successful.")
						r.buf = r.buf[consumed:]
						return event, nil
					} else if err == ErrIncomplete {
						goto waitForMore
					}
				}

				// 7. Other CSI Sequences (Legacy, Win32, Kitty)
				if len(r.buf) > 1 && r.buf[1] == '[' {
					Log("Reader: Attempting generic CSI parsing...")
					if terminatorIdx, cmd, err := scanCSI(r.buf); err == nil {
						var event *InputEvent
						var consumed int
						var pErr error

						// Parse Legacy CSI FIRST to match far2l priority
						event, consumed, pErr = ParseLegacyCSI(r.buf)

						if pErr == ErrInvalidSequence || event == nil {
							if !r.far2lExtensionsEnabled {
								if cmd == '_' {
									event, consumed, pErr = ParseWin32InputEvent(r.buf)
								} else {
									event, consumed, pErr = ParseKitty(r.buf)
								}
							} else {
								pErr = ErrInvalidSequence // Force skip Win32/Kitty
							}
						}

						if pErr == nil && event != nil {
							r.buf = r.buf[consumed:]
							Log("Reader: Returning CSI event: %s", event.String())
							return event, nil
						} else if pErr == ErrInvalidSequence {
							Log("Reader: Unsupported CSI sequence: %q", string(r.buf[:terminatorIdx+1]))
							r.buf = r.buf[terminatorIdx+1:]
							continue
						} else {
							Log("Reader: Unhandled error parsing CSI: %v", pErr)
						}
					} else if err == ErrIncomplete {
						goto waitForMore
					}
				}

				// 8. Double ESC
				if len(r.buf) >= 2 && r.buf[1] == 0x1B {
					Log("Reader: Parsed Double ESC.")
					r.buf = r.buf[2:]
					return &InputEvent{Type: KeyEventType, VirtualKeyCode: VK_ESCAPE, KeyDown: true}, nil
				}

				// 9. Legacy Alt (ESC + Char)
				if len(r.buf) >= 2 && utf8.FullRune(r.buf[1:]) {
					Log("Reader: Parsed Legacy Alt.")
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
					Log("Reader: ESC timeout (100ms) for ambiguous sequence. Returning ESC key.")
					r.buf = r.buf[1:] // Consume the initial ESC byte
					return &InputEvent{Type: KeyEventType, VirtualKeyCode: VK_ESCAPE, KeyDown: true}, nil
				case err := <-r.errChan:
					Log("Reader: Error in dataChan (drain1): %v", err)
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
					Log("Reader: Buffer not empty after drain, continuing parsing.")
					continue
				}
			}

			// Handle standalone BACK (0x7F)
			if r.buf[0] == 0x7F {
				r.buf = r.buf[1:]
				return &InputEvent{Type: KeyEventType, VirtualKeyCode: VK_BACK, KeyDown: true, IsLegacy: true}, nil
			}

			// Handle regular UTF-8 characters
			if utf8.FullRune(r.buf) {
				character, size := utf8.DecodeRune(r.buf)
				consumed := size

				// Far2l workaround: some versions of far2l's terminal emulator
				// send an extra space after the '=' character.
				if character == '=' && len(r.buf) > size && r.buf[size] == ' ' {
					consumed++
					Log("Reader: Applied Far2l '=' workaround, consumed extra space.")
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
			Log("Reader: Error in dataChan (drain2): %v", err)
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
			Log("Reader: Buffer not empty after drain, continuing parsing.")
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
