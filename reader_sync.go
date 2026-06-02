package vtinput

import (
	"io"
	"sync"
	"time"
	"unicode/utf8"
)

// ReaderSync reads input synchronously without a background goroutine.
// It measures the time from receiving raw bytes to generating an InputEvent.
type ReaderSync struct {
	in     io.Reader
	buf    []byte
	done   chan struct{}
	useConPTY bool // Windows only
	far2lExtensionsEnabled bool
	conHandle    uintptr // Windows only: console handle
	cancelEvent  uintptr // Windows only: event handle for cancellation
	oldMode      uint32  // Windows only: saved console mode
	stopPipe     [2]int  // Unix only: pipe for interrupting Poll

	mu            sync.Mutex
	lastLatency   time.Duration
	totalLatency  time.Duration
	eventCount    int64
	lastReceivedAt time.Time

	eventChan  chan *InputEvent
	stopRead   chan struct{}
	onceStop   sync.Once

	MetricsEnabled bool
}

// NewReaderSync creates a synchronous input reader.
func NewReaderSync(in io.Reader) *ReaderSync {
	r := &ReaderSync{
		in:   in,
		buf:  make([]byte, 0, 128),
		done: make(chan struct{}),
	}
	r.platformInit(in)
	return r
}

// Close stops reading.
func (r *ReaderSync) Close() {
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
func (r *ReaderSync) EventChan() <-chan *InputEvent {
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
func (r *ReaderSync) ReadEvent() (*InputEvent, error) {
	return r.ReadEventTimeout(0)
}

// ReadEventTimeout reads the next input event with an optional timeout.
func (r *ReaderSync) ReadEventTimeout(timeout time.Duration) (*InputEvent, error) {
	// Fast path for Windows ConPTY
	if r.useConPTY {
		return r.readConPTYEventTimeout(timeout)
	}

	var deadline time.Time
	if timeout > 0 {
		deadline = time.Now().Add(timeout)
	}

	tmp := make([]byte, 1024)

	for {
		select {
		case <-r.done:
			return nil, io.EOF
		default:
		}

		if !deadline.IsZero() && time.Now().After(deadline) {
			return nil, nil
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
						r.recordLatency(time.Since(r.lastReceivedAt))
						return event, nil
					}
				}

				// 2. DSR Replies
				if len(parseBuf) > 2 && parseBuf[1] == '[' {
					if terminatorIdx, cmd, err := scanCSI(parseBuf); err == nil {
						if cmd == 'n' {
							r.buf = r.buf[terminatorIdx+1+altOffset:]
							continue
						}
					} else if err == ErrIncomplete {
						goto waitForMore
					}
				}

				// 3. APC (Far2l)
				if len(parseBuf) > 1 && parseBuf[1] == '_' {
					if event, consumed, err := ParseFar2lAPC(parseBuf); err == nil {
						r.buf = r.buf[consumed+altOffset:]
						if event != nil {
							if event.Type == Far2lEventType && event.Far2lCommand == "ok" {
								r.far2lExtensionsEnabled = true
							}
							r.recordLatency(time.Since(r.lastReceivedAt))
							return event, nil
						}
						continue
					} else if err == ErrIncomplete {
						goto waitForMore
					}
				}

				// 4. Bracketed Paste
				if len(parseBuf) >= 6 && parseBuf[1] == '[' && parseBuf[2] == '2' && parseBuf[3] == '0' && (parseBuf[4] == '0' || parseBuf[4] == '1') && parseBuf[5] == '~' {
					event := &InputEvent{Type: PasteEventType, PasteStart: parseBuf[4] == '0'}
					r.buf = r.buf[6+altOffset:]
					r.recordLatency(time.Since(r.lastReceivedAt))
					return event, nil
				}

				// 5. SGR Mouse
				if len(parseBuf) > 3 && parseBuf[1] == '[' && parseBuf[2] == '<' {
					if event, consumed, err := ParseMouseSGR(parseBuf); err == nil {
						r.buf = r.buf[consumed+altOffset:]
						if altOffset > 0 { event.ControlKeyState |= LeftAltPressed }
						r.recordLatency(time.Since(r.lastReceivedAt))
						return event, nil
					} else if err == ErrIncomplete {
						goto waitForMore
					}
				}

				// 5.5. Legacy Mouse
				if len(parseBuf) >= 3 && parseBuf[1] == '[' && parseBuf[2] == 'M' {
					if event, consumed, err := ParseMouseLegacy(parseBuf); err == nil {
						r.buf = r.buf[consumed+altOffset:]
						if altOffset > 0 { event.ControlKeyState |= LeftAltPressed }
						r.recordLatency(time.Since(r.lastReceivedAt))
						return event, nil
					} else if err == ErrIncomplete {
						goto waitForMore
					}
				}

				// 5.6. URXVT Mouse
				if len(parseBuf) > 3 && parseBuf[1] == '[' {
					if terminatorIdx, cmd, err := scanCSI(parseBuf); err == nil && cmd == 'M' && terminatorIdx > 2 {
						if event, consumed, err := ParseMouseURXVT(parseBuf); err == nil {
							r.buf = r.buf[consumed+altOffset:]
							if altOffset > 0 { event.ControlKeyState |= LeftAltPressed }
							r.recordLatency(time.Since(r.lastReceivedAt))
							return event, nil
						}
					} else if err == ErrIncomplete {
						goto waitForMore
					}
				}

				// 6. SS3 Sequences
				if (len(parseBuf) > 1 && parseBuf[1] == 'O') || (len(parseBuf) > 2 && parseBuf[1] == '[' && parseBuf[2] == 'O') {
					if event, consumed, err := ParseLegacySS3(parseBuf); err == nil {
						r.buf = r.buf[consumed+altOffset:]
						if altOffset > 0 { event.ControlKeyState |= LeftAltPressed }
						r.recordLatency(time.Since(r.lastReceivedAt))
						return event, nil
					} else if err == ErrIncomplete {
						goto waitForMore
					}
				}

				// 6.5. DA
				if len(parseBuf) > 2 && parseBuf[1] == '[' && parseBuf[2] == '?' {
					if terminatorIdx, cmd, err := scanCSI(parseBuf); err == nil && cmd == 'c' {
						r.buf = r.buf[terminatorIdx+1+altOffset:]
						continue
					} else if err == ErrIncomplete {
						goto waitForMore
					}
				}

				// 7. Other CSI Sequences
				if len(parseBuf) > 1 && parseBuf[1] == '[' {
					if terminatorIdx, cmd, err := scanCSI(parseBuf); err == nil {
						var event *InputEvent
						var consumed int
						var pErr error

						event, consumed, pErr = ParseLegacyCSI(parseBuf)

						if pErr == nil && event != nil && r.far2lExtensionsEnabled {
							r.buf = r.buf[consumed+altOffset:]
							continue
						}

						if pErr == ErrInvalidSequence || event == nil {
							if cmd == '_' {
								event, consumed, pErr = ParseWin32InputEvent(parseBuf)
							} else {
								event, consumed, pErr = ParseKitty(parseBuf)
							}
						}

						if pErr == nil && event != nil {
							r.buf = r.buf[consumed+altOffset:]
							if altOffset > 0 { event.ControlKeyState |= LeftAltPressed }
							r.recordLatency(time.Since(r.lastReceivedAt))
							return event, nil
						} else if pErr == ErrInvalidSequence {
							r.buf = r.buf[terminatorIdx+1+altOffset:]
							continue
						}
					} else if err == ErrIncomplete {
						goto waitForMore
					}
				}

				// 8. Double ESC
				if len(r.buf) >= 2 && r.buf[1] == 0x1B && altOffset == 0 {
					r.buf = r.buf[2:]
					r.recordLatency(time.Since(r.lastReceivedAt))
					return &InputEvent{Type: KeyEventType, VirtualKeyCode: VK_ESCAPE, KeyDown: true, InputSource: "legacy_esc"}, nil
				}

				// 9. Legacy Alt
				if len(r.buf) >= 2 && utf8.FullRune(r.buf[1:]) {
					r.buf = r.buf[1:]
					character, size := utf8.DecodeRune(r.buf)
					r.buf = r.buf[size:]

					if legacyEvt := translateLegacyByte(character); legacyEvt != nil {
						legacyEvt.ControlKeyState |= LeftAltPressed
						legacyEvt.InputSource = "legacy_alt_ctrl"
						r.recordLatency(time.Since(r.lastReceivedAt))
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
				waitTimeout := 100 * time.Millisecond
				if !deadline.IsZero() {
					if rem := time.Until(deadline); rem < waitTimeout {
						waitTimeout = rem
						if waitTimeout <= 0 {
							r.buf = r.buf[1:]
							r.recordLatency(time.Since(r.lastReceivedAt))
							return &InputEvent{Type: KeyEventType, VirtualKeyCode: VK_ESCAPE, KeyDown: true}, nil
						}
					}
				}
				n, err := r.readBytes(tmp, waitTimeout)
				if n > 0 {
					r.buf = append(r.buf, tmp[:n]...)
					continue
				}
				if err != nil {
					if len(r.buf) == 0 {
						return nil, err
					}
				}
				r.buf = r.buf[1:]
				r.recordLatency(time.Since(r.lastReceivedAt))
				return &InputEvent{Type: KeyEventType, VirtualKeyCode: VK_ESCAPE, KeyDown: true}, nil
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
				consumed := size
				if character == '=' && len(r.buf) > size && r.buf[size] == ' ' {
					consumed++
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

		readTimeout := time.Duration(0)
		if !deadline.IsZero() {
			readTimeout = time.Until(deadline)
			if readTimeout <= 0 {
				return nil, nil
			}
		}
		n, err := r.readBytes(tmp, readTimeout)
		if err != nil {
			if len(r.buf) == 0 {
				return nil, err
			}
			Log("ReaderSync: Read error with buffered data: %v", err)
			continue
		}
		if n > 0 {
			r.buf = append(r.buf, tmp[:n]...)
		}
	}
}

func (r *ReaderSync) recordLatency(latency time.Duration) {
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
func (r *ReaderSync) Metrics() (last time.Duration, avg time.Duration, count int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.eventCount > 0 {
		avg = r.totalLatency / time.Duration(r.eventCount)
	}
	return r.lastLatency, avg, r.eventCount
}
