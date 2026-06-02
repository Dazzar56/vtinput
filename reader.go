package vtinput

import (
	"io"
	"sync"
	"time"
	"unicode/utf8"
)

// Reader wraps an io.Reader (like os.Stdin) and parses input events.
// It buffers input internally to handle incomplete escape sequences.
type timedChunk struct {
	data []byte
	at   time.Time
}

type timedEvent struct {
	event *InputEvent
	at    time.Time
}

type Reader struct {
	in                     io.Reader
	buf                    []byte
	dataChan               chan timedChunk
	NativeEventChan        chan timedEvent
	errChan                chan error
	done                   chan struct{}
	stopPipe               [2]int // Used on Unix for Select unblocking
	far2lExtensionsEnabled bool
	conHandle              uintptr // Windows: console handle for CancelIoEx

	mu             sync.Mutex
	lastLatency    time.Duration
	totalLatency   time.Duration
	eventCount     int64
	lastReceivedAt time.Time

	eventChan  chan *InputEvent
	stopRead   chan struct{}
	onceStop   sync.Once

	MetricsEnabled bool
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
	if r.stopRead != nil {
		r.onceStop.Do(func() { close(r.stopRead) })
	}
}

// EventChan returns a channel that yields input events.
// It starts a background goroutine that calls ReadEvent() in a loop.
// The channel is closed when the reader is closed or an error occurs.
func (r *Reader) EventChan() <-chan *InputEvent {
	if r.eventChan != nil {
		return r.eventChan
	}
	r.eventChan = make(chan *InputEvent, 1024)
	r.stopRead = make(chan struct{})
	go func() {
		for {
			select {
			case <-r.stopRead:
				return
			default:
			}
			e, err := r.ReadEvent()
			if err != nil {
				r.onceStop.Do(func() { close(r.eventChan) })
				return
			}
			select {
			case r.eventChan <- e:
			case <-r.stopRead:
				return
			}
		}
	}()
	return r.eventChan
}

// ReadEvent reads the next input event.
func (r *Reader) ReadEvent() (*InputEvent, error) {
	return r.ReadEventTimeout(0)
}

// ReadEventTimeout reads the next input event with an optional maximum blocking time.
func (r *Reader) ReadEventTimeout(timeout time.Duration) (*InputEvent, error) {
	Log("READER_LOOP: Entering ReadEventTimeout. BufLen: %d", len(r.buf))
	var timer <-chan time.Time
	if timeout > 0 {
		timer = time.After(timeout)
	}

	for {
		select {
		case <-r.done:
			Log("Reader[%p]: Done signal received, exiting.", r)
			return nil, io.EOF
		case <-timer:
			return nil, nil // Timeout
		case te := <-r.NativeEventChan:
			ev := te.event
			r.lastReceivedAt = te.at
			Log("READER_TRACE: Recv NativeEvent: %s", ev.String())
			// FIX: Apply VK=0 buffering ONLY to ConPTY (Windows).
			// Native X11 events must go straight to the app to avoid byte truncation.
			if ev.Type == KeyEventType && ev.VirtualKeyCode == 0 && ev.Char != 0 && ev.InputSource == "ConPTY" {
				if ev.KeyDown {
					Log("READER_TRACE: VK is 0, ENQUEUEING Char '%c' (%d) to buf. Current buf: %q", ev.Char, ev.Char, string(r.buf))
					r.buf = append(r.buf, string(ev.Char)...)
				} else {
					Log("READER_TRACE: VK is 0, KeyUp for '%c' ignored.", ev.Char)
				}
				continue
			}
			Log("Reader[%p]: Returning native event: %s", r, ev.String())
			r.recordLatency(time.Since(r.lastReceivedAt))
			return ev, nil
		default:
		}

		// Greedy drain
	greedy:
		for {
			select {
			case te := <-r.NativeEventChan:
				ev := te.event
				r.lastReceivedAt = te.at
				if ev.Type == KeyEventType && ev.VirtualKeyCode == 0 && ev.Char != 0 && ev.InputSource == "ConPTY" {
					if ev.KeyDown {
						Log("READER_TRACE: (greedy) VK is 0, ENQUEUEING Char '%c' (%d) to buf. Current buf: %q", ev.Char, ev.Char, string(r.buf))
						r.buf = append(r.buf, string(ev.Char)...)
						break greedy // Go process buffer
					} else {
						Log("READER_TRACE: (greedy) VK is 0, KeyUp for '%c' ignored.", ev.Char)
						continue
					}
				}
				Log("Reader: Returning native event: %s", ev.String())
				r.recordLatency(time.Since(r.lastReceivedAt))
				return ev, nil
			case tc := <-r.dataChan:
				r.lastReceivedAt = tc.at
				r.buf = append(r.buf, tc.data...)
			default:
				break greedy
			}
		}

		if len(r.buf) > 0 {
			if r.buf[0] == 0x1B {
				altOffset := 0
				parseBuf := r.buf
				if len(r.buf) >= 3 && r.buf[0] == 0x1B && r.buf[1] == 0x1B {
					c := r.buf[2]
					if c == '[' || c == 'O' || c == '<' || c == '_' {
						altOffset = 1
						parseBuf = r.buf[1:]
					}
				}

				// 1. Focus
				if len(parseBuf) >= 3 && parseBuf[1] == '[' && (parseBuf[2] == 'I' || parseBuf[2] == 'O') {
					// Workaround for VTE bug: Alt+F1..F4 sent as ESC [ O 3 P instead of ESC O 3 P
					isVteBrokenSS3 := false
					if parseBuf[2] == 'O' && len(parseBuf) > 3 {
						c := parseBuf[3]
						if (c >= '0' && c <= '9') || c == 'P' || c == 'Q' || c == 'R' || c == 'S' {
							isVteBrokenSS3 = true
						}
					}
					if !isVteBrokenSS3 {
						event := &InputEvent{Type: FocusEventType, SetFocus: parseBuf[2] == 'I'}
						r.buf = r.buf[3+altOffset:]
						Log("Reader: Parsed Focus %v.", event.SetFocus)
						r.recordLatency(time.Since(r.lastReceivedAt))
						return event, nil
					}
				}

				// 2. DSR Replies (ESC [ ... n)
				if len(parseBuf) > 2 && parseBuf[1] == '[' {
					if terminatorIdx, cmd, err := scanCSI(parseBuf); err == nil {
						if cmd == 'n' {
							r.buf = r.buf[terminatorIdx+1+altOffset:]
							Log("Reader: Parsed DSR reply. Continuing.")
							continue
						}
					} else if err == ErrIncomplete {
						goto waitForMore
					}
				}

				// 3. APC (Far2l)
				if len(parseBuf) > 1 && parseBuf[1] == '_' {
					Log("Reader: Attempting ParseFar2lAPC...")
					if event, consumed, err := ParseFar2lAPC(parseBuf); err == nil {
						Log("Reader: ParseFar2lAPC successful, consumed %d bytes.", consumed)
						r.buf = r.buf[consumed+altOffset:]
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
				if len(parseBuf) >= 6 && parseBuf[1] == '[' && parseBuf[2] == '2' && parseBuf[3] == '0' && (parseBuf[4] == '0' || parseBuf[4] == '1') && parseBuf[5] == '~' {
					event := &InputEvent{Type: PasteEventType, PasteStart: parseBuf[4] == '0'}
					r.buf = r.buf[6+altOffset:]
					Log("Reader: Parsed Paste event.")
					r.recordLatency(time.Since(r.lastReceivedAt))
					return event, nil
				}

				// 5. SGR Mouse (ESC [ < ... M/m)
				if len(parseBuf) > 3 && parseBuf[1] == '[' && parseBuf[2] == '<' {
					Log("Reader: Attempting ParseMouseSGR...")
					if event, consumed, err := ParseMouseSGR(parseBuf); err == nil {
						Log("Reader: ParseMouseSGR successful.")
						r.buf = r.buf[consumed+altOffset:]
						if altOffset > 0 { event.ControlKeyState |= LeftAltPressed }
						r.recordLatency(time.Since(r.lastReceivedAt))
						return event, nil
					} else if err == ErrIncomplete {
						goto waitForMore
					}
				}

				// 5.5. Legacy Mouse (ESC [ M Cb Cx Cy)
				if len(parseBuf) >= 3 && parseBuf[1] == '[' && parseBuf[2] == 'M' {
					Log("Reader: Attempting ParseMouseLegacy...")
					if event, consumed, err := ParseMouseLegacy(parseBuf); err == nil {
						Log("Reader: ParseMouseLegacy successful.")
						r.buf = r.buf[consumed+altOffset:]
						if altOffset > 0 { event.ControlKeyState |= LeftAltPressed }
						r.recordLatency(time.Since(r.lastReceivedAt))
						return event, nil
					} else if err == ErrIncomplete {
						goto waitForMore
					}
				}

				// 5.6. URXVT Mouse (ESC [ Cb ; Cx ; Cy M)
				if len(parseBuf) > 3 && parseBuf[1] == '[' {
					if terminatorIdx, cmd, err := scanCSI(parseBuf); err == nil && cmd == 'M' && terminatorIdx > 2 {
						Log("Reader: Attempting ParseMouseURXVT...")
						if event, consumed, err := ParseMouseURXVT(parseBuf); err == nil {
							Log("Reader: ParseMouseURXVT successful.")
							r.buf = r.buf[consumed+altOffset:]
							if altOffset > 0 { event.ControlKeyState |= LeftAltPressed }
							r.recordLatency(time.Since(r.lastReceivedAt))
							return event, nil
						}
					} else if err == ErrIncomplete {
						goto waitForMore
					}
				}

				// 6. SS3 Sequences (ESC O ... or broken VTE ESC [ O ...)
				if (len(parseBuf) > 1 && parseBuf[1] == 'O') || (len(parseBuf) > 2 && parseBuf[1] == '[' && parseBuf[2] == 'O') {
					Log("Reader: Attempting ParseLegacySS3...")
					if event, consumed, err := ParseLegacySS3(parseBuf); err == nil {
						Log("Reader: ParseLegacySS3 successful.")
						r.buf = r.buf[consumed+altOffset:]
						if altOffset > 0 { event.ControlKeyState |= LeftAltPressed }
						r.recordLatency(time.Since(r.lastReceivedAt))
						return event, nil
					} else if err == ErrIncomplete {
						goto waitForMore
					}
				}

				// 6.5. DA (Device Attributes) - consume \x1b[?...c
				if len(parseBuf) > 2 && parseBuf[1] == '[' && parseBuf[2] == '?' {
					if terminatorIdx, cmd, err := scanCSI(parseBuf); err == nil && cmd == 'c' {
						r.buf = r.buf[terminatorIdx+1+altOffset:]
						Log("Reader: Consumed DA response.")
						continue
					} else if err == ErrIncomplete {
						goto waitForMore
					}
				}

				// 7. Other CSI Sequences (Legacy, Win32, Kitty)
				if len(parseBuf) > 1 && parseBuf[1] == '[' {
					Log("Reader: Attempting generic CSI parsing...")
					if terminatorIdx, cmd, err := scanCSI(parseBuf); err == nil {
						var event *InputEvent
						var consumed int
						var pErr error

						// Parse Legacy CSI FIRST to match far2l priority
						event, consumed, pErr = ParseLegacyCSI(parseBuf)

						if pErr == nil && event != nil && r.far2lExtensionsEnabled {
							// If legacy handled it, but Far2l is on, we ignore it to prevent
							// duplicates, as we expect the primary event via Far2l APC.
							r.buf = r.buf[consumed+altOffset:]
							continue
						}

						if pErr == ErrInvalidSequence || event == nil {
							// Modern sequences (Win32/Kitty) are always allowed, even if
							// Far2l is on, because they don't collide and are used for
							// high-precision input in nested sessions.
							if cmd == '_' {
								event, consumed, pErr = ParseWin32InputEvent(parseBuf)
							} else {
								event, consumed, pErr = ParseKitty(parseBuf)
							}
						}

						if pErr == nil && event != nil {
							r.buf = r.buf[consumed+altOffset:]
							if altOffset > 0 { event.ControlKeyState |= LeftAltPressed }
							Log("Reader: Returning CSI event: %s", event.String())
							r.recordLatency(time.Since(r.lastReceivedAt))
							return event, nil
						} else if pErr == ErrInvalidSequence {
							Log("Reader: Unsupported CSI sequence: %q", string(parseBuf[:terminatorIdx+1]))
							r.buf = r.buf[terminatorIdx+1+altOffset:]
							continue
						} else {
							Log("Reader: Unhandled error parsing CSI: %v", pErr)
						}
					} else if err == ErrIncomplete {
						goto waitForMore
					}
				}

				// 8. Double ESC
				if len(r.buf) >= 2 && r.buf[1] == 0x1B && altOffset == 0 {
					Log("Reader: Parsed Double ESC.")
					r.buf = r.buf[2:]
					return &InputEvent{Type: KeyEventType, VirtualKeyCode: VK_ESCAPE, KeyDown: true, InputSource: "legacy_esc"}, nil
				}

				// 9. Legacy Alt (ESC + Char)
				if len(r.buf) >= 2 && utf8.FullRune(r.buf[1:]) {
					Log("Reader: Parsed Legacy Alt.")
					r.buf = r.buf[1:]
					character, size := utf8.DecodeRune(r.buf)
					r.buf = r.buf[size:]

					// Translate hidden Ctrl modifiers inside ASCII control characters (e.g. \x01 for Ctrl+A)
					if legacyEvt := translateLegacyByte(character); legacyEvt != nil {
						legacyEvt.ControlKeyState |= LeftAltPressed
						legacyEvt.InputSource = "legacy_alt_ctrl"
						return legacyEvt, nil
					}

						r.recordLatency(time.Since(r.lastReceivedAt))
						return &InputEvent{
							Type:            KeyEventType,
							Char:            character,
							ControlKeyState: LeftAltPressed,
							KeyDown:         true,
							IsLegacy:        true,
							InputSource:     "legacy_alt",
						}, nil
				}

			waitForMore:
				select {
				case te := <-r.NativeEventChan:
					ev := te.event
					r.lastReceivedAt = te.at
					if ev.Type == KeyEventType && ev.VirtualKeyCode == 0 && ev.Char != 0 && ev.InputSource == "ConPTY" {
						if ev.KeyDown {
							r.buf = append(r.buf, byte(ev.Char))
						}
						continue
					}
					Log("Reader: Returning native event: %s", ev.String())
					r.recordLatency(time.Since(r.lastReceivedAt))
					return ev, nil
				case tc := <-r.dataChan:
					r.lastReceivedAt = tc.at
					r.buf = append(r.buf, tc.data...)
					continue
				case <-time.After(100 * time.Millisecond):
					Log("READER_LOOP: ESC timeout (100ms) triggered. Buffer tail: %q", string(r.buf))
					r.buf = r.buf[1:] // Consume the initial ESC byte
					return &InputEvent{Type: KeyEventType, VirtualKeyCode: VK_ESCAPE, KeyDown: true}, nil
				case err := <-r.errChan:
					Log("Reader: Error in dataChan (drain1): %v", err)
					// Drain any data that arrived before the error
				drain1:
					for {
						select {
						case tc := <-r.dataChan:
							r.lastReceivedAt = tc.at
							r.buf = append(r.buf, tc.data...)
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
				r.recordLatency(time.Since(r.lastReceivedAt))
				return &InputEvent{Type: KeyEventType, VirtualKeyCode: VK_BACK, KeyDown: true, IsLegacy: true, InputSource: "legacy_char", RepeatCount: 1}, nil
			}

			// Handle regular UTF-8 characters
			if utf8.FullRune(r.buf) {
				character, size := utf8.DecodeRune(r.buf)
				Log("READER_LOOP: Decoded UTF-8 rune from buffer: '%c' (%d), size: %d", character, character, size)
				consumed := size

				// Far2l workaround: some versions of far2l's terminal emulator
				// send an extra space after the '=' character.
				if character == '=' && len(r.buf) > size && r.buf[size] == ' ' {
					consumed++
					Log("Reader: Applied Far2l '=' workaround, consumed extra space.")
				}

				r.buf = r.buf[consumed:]
				if event := translateLegacyByte(character); event != nil {
					event.InputSource = "legacy_ctrl"
					r.recordLatency(time.Since(r.lastReceivedAt))
					return event, nil
				}
				r.recordLatency(time.Since(r.lastReceivedAt))
				return &InputEvent{Type: KeyEventType, Char: character, KeyDown: true, IsLegacy: true, InputSource: "legacy_char", RepeatCount: 1}, nil
			}
		}

		select {
		case te := <-r.NativeEventChan:
			ev := te.event
			r.lastReceivedAt = te.at
			if ev.Type == KeyEventType && ev.VirtualKeyCode == 0 && ev.Char != 0 && ev.InputSource == "ConPTY" {
				if ev.KeyDown {
					r.buf = append(r.buf, byte(ev.Char))
				}
				continue
			}
			Log("Reader: Returning native event: %s", ev.String())
			r.recordLatency(time.Since(r.lastReceivedAt))
			return ev, nil
		case tc := <-r.dataChan:
			r.lastReceivedAt = tc.at
			r.buf = append(r.buf, tc.data...)
		case err := <-r.errChan:
			Log("Reader: Error in dataChan (drain2): %v", err)
			// Prioritize data over error to avoid premature EOF
		drain2:
			for {
				select {
				case tc := <-r.dataChan:
					r.lastReceivedAt = tc.at
					r.buf = append(r.buf, tc.data...)
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

func (r *Reader) recordLatency(latency time.Duration) {
	if !r.MetricsEnabled {
		return
	}
	r.mu.Lock()
	r.lastLatency = latency
	r.totalLatency += latency
	r.eventCount++
	r.mu.Unlock()
}

// Metrics returns the last event latency, average latency, and total event count.
func (r *Reader) Metrics() (last time.Duration, avg time.Duration, count int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.eventCount > 0 {
		avg = r.totalLatency / time.Duration(r.eventCount)
	}
	return r.lastLatency, avg, r.eventCount
}

func translateLegacyByte(r rune) *InputEvent {
	evt := &InputEvent{Type: KeyEventType, KeyDown: true, IsLegacy: true, RepeatCount: 1}
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
