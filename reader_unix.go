//go:build !windows

package vtinput

import (
	"io"
	"os"
	"syscall"
)

// NewReader creates a new Reader instance for Unix-like systems.
// It uses the "Self-Pipe Trick" with syscall.Select to allow the background
// reading goroutine to be interrupted instantly without "stealing" bytes
// from the terminal buffer, which is critical for session resurrection.
func NewReader(in io.Reader) *Reader {
	r := &Reader{
		in:       in,
		buf:      make([]byte, 0, 128),
		dataChan: make(chan byte, 1024),
		errChan:  make(chan error, 1),
		done:     make(chan struct{}),
	}

	if err := syscall.Pipe(r.stopPipe[:]); err != nil {
		// Fallback to basic behavior if pipe fails
		return r
	}

	var fd int
	if f, ok := in.(*os.File); ok {
		fd = int(f.Fd())
	} else {
		close(r.done)
		return r
	}

	go func() {
		defer syscall.Close(r.stopPipe[0])
		tmp := make([]byte, 1024)

		for {
			readFds := &syscall.FdSet{}
			readFds.Bits[fd/64] |= 1 << (uint(fd) % 64)
			readFds.Bits[r.stopPipe[0]/64] |= 1 << (uint(r.stopPipe[0]) % 64)

			// Wait for data on Stdin OR a signal on the stopPipe
			_, err := syscall.Select(max(fd, r.stopPipe[0])+1, readFds, nil, nil, nil)
			if err != nil {
				if err == syscall.EINTR { continue }
				r.errChan <- err
				return
			}

			// Check if we were told to stop via Close()
			if (readFds.Bits[r.stopPipe[0]/64] & (1 << (uint(r.stopPipe[0]) % 64))) != 0 {
				return
			}

			n, err := syscall.Read(fd, tmp)
			if err != nil {
				if err == syscall.EAGAIN || err == syscall.EINTR { continue }
				r.errChan <- err
				return
			}
			if n == 0 {
				r.errChan <- io.EOF
				return
			}
			for i := 0; i < n; i++ {
				r.dataChan <- tmp[i]
			}
		}
	}()

	return r
}

func (r *Reader) platformClose() {
	// Signal the Select loop to exit via the pipe
	syscall.Write(r.stopPipe[1], []byte{0})
	syscall.Close(r.stopPipe[1])
}