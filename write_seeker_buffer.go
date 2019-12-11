package bindiff

import "errors"

var (
	InvalidWhence      = errors.New("bindiff: invalid whence")
	NegativePosition   = errors.New("bindiff: negative position")
	PositionOutOfRange = errors.New("bindiff: position out of range")
)

type seekBuffer struct {
	buffer   []byte
	position int
}

func (s *seekBuffer) Write(p []byte) (n int, err error) {
	n = copy(s.buffer[s.position:], p)

	if n == len(p) {
		s.position += n
		return n, nil
	}

	s.buffer = append(s.buffer, p[n:]...)
	s.position += len(p)

	return len(p), nil
}

func (s *seekBuffer) Seek(offset int64, whence int) (int64, error) {
	var absolute int64

	if whence == 0 {
		absolute = offset
	} else if whence == 1 {
		absolute = int64(s.position) + offset
	} else if whence == 2 {
		absolute = int64(len(s.buffer)) + offset
	} else {
		return 0, InvalidWhence
	}

	if absolute < 0 {
		return 0, NegativePosition
	}
	if absolute >= 1<<31 {
		return 0, PositionOutOfRange
	}
	s.position = int(absolute)

	return absolute, nil
}
