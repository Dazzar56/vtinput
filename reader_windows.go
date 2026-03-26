//go:build windows

package vtinput

import (
	"io"
)

// NewReader creates a new Reader instance for Windows.
// Since Windows resurrect/daemon logic is not yet implemented using
// terminal FD passing, we use a simpler reading loop.
func NewReader(in io.Reader) *Reader {
	r := &Reader{
		in:       in,
		buf:      make([]byte, 0, 128),
		dataChan: make(chan byte, 1024),
		errChan:  make(chan error, 1),
		done:     make(chan struct{}),
	}

	go func() {
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
	}()

	return r
}

func (r *Reader) platformClose() {
	// done channel is enough for Windows fallback
}