//go:build !windows

package vtinput

import (
	"io"
	"os"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

func (r *Reader) platformInit(_ io.Reader) {
	syscall.Pipe(r.stopPipe[:])
}

func (r *Reader) readConPTYEventTimeout(_ time.Duration) (*InputEvent, error) {
	return nil, nil
}

func (r *Reader) platformClose() {
	syscall.Write(r.stopPipe[1], []byte{0})
	syscall.Close(r.stopPipe[1])
}

func (r *Reader) readBytes(buf []byte, timeout time.Duration) (int, error) {
	var fd int
	isFile := false
	if f, ok := r.in.(*os.File); ok {
		fd = int(f.Fd())
		isFile = true
	}

	if !isFile {
		// For non-file readers (tests), use standard Read with optional deadline.
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

	fds := []unix.PollFd{
		{Fd: int32(fd), Events: unix.POLLIN},
		{Fd: int32(r.stopPipe[0]), Events: unix.POLLIN},
	}

	pollTimeout := -1
	if timeout > 0 {
		ms := int(timeout.Milliseconds())
		if ms > 0 {
			pollTimeout = ms
		}
	}

	_, err := unix.Poll(fds, pollTimeout)
	if err != nil {
		if err == unix.EINTR {
			return 0, nil
		}
		return 0, err
	}

	if fds[1].Revents != 0 {
		return 0, io.EOF
	}

	if fds[0].Revents&unix.POLLIN == 0 {
		return 0, nil // timeout
	}

	n, err := syscall.Read(fd, buf)
	if n > 0 {
		if r.MetricsEnabled {
			r.lastReceivedAt = time.Now()
		}
	}
	if err != nil {
		if err == syscall.EAGAIN || err == syscall.EINTR {
			return 0, nil
		}
		return 0, err
	}
	if n == 0 {
		return 0, io.EOF
	}
	return n, nil
}
