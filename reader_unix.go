//go:build !windows

package vtinput

import (
	"io"
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

func NewReader(in io.Reader) *Reader {
	r := &Reader{
		in:              in,
		buf:             make([]byte, 0, 128),
		dataChan:        make(chan []byte, 16),
		NativeEventChan: nil, //таких эвентов на линуксе нет, так что память не тратим
		errChan:         make(chan error, 1),
		done:            make(chan struct{}),
	}

	if err := syscall.Pipe(r.stopPipe[:]); err != nil {
		return r
	}

	var fd int
	isAFile := false
	if f, ok := in.(*os.File); ok {
		fd = int(f.Fd())
		isAFile = true
	}

	go func() {
		defer syscall.Close(r.stopPipe[0])
		tmp := make([]byte, 1024)

		for {
			if isAFile {
				// Advanced logic for real terminals (session resurrection support)
				fds := []unix.PollFd{
					{Fd: int32(fd), Events: unix.POLLIN},
					{Fd: int32(r.stopPipe[0]), Events: unix.POLLIN},
				}

				_, err := unix.Poll(fds, -1)
				if err != nil {
					if err == unix.EINTR { continue }
					r.errChan <- err
					return
				}

				if fds[1].Revents != 0 {
					return
				}

				n, err := syscall.Read(fd, tmp)
				if n > 0 {
					buf := make([]byte, n)
					copy(buf, tmp[:n])
					r.dataChan <- buf
				}

				if err != nil {
					Log("Reader(syscall): Read error: %v", err)
				}
				if err != nil {
					if err == syscall.EAGAIN || err == syscall.EINTR { continue }
					r.errChan <- err
					return
				}
				if n == 0 {
					r.errChan <- io.EOF
					return
				}
			} else {
				// Fallback for tests (pipes, buffers)
				select {
				case <-r.done:
					return
				default:
					n, err := in.Read(tmp)
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
	}()

	return r
}

func (r *Reader) platformClose() {
	syscall.Write(r.stopPipe[1], []byte{0})
	syscall.Close(r.stopPipe[1])
}