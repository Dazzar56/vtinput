package vtinput

import "encoding/binary"

// Far2lStack provides serialization and deserialization matching far2l's StackSerializer.
// It acts as a LIFO stack for popping (reading from the end) and a sequential buffer for pushing.
type Far2lStack []byte

func (s *Far2lStack) PopU8() uint8 {
	l := len(*s)
	if l < 1 {
		return 0
	}
	v := (*s)[l-1]
	*s = (*s)[:l-1]
	return v
}

func (s *Far2lStack) PopU16() uint16 {
	l := len(*s)
	if l < 2 {
		return 0
	}
	v := binary.LittleEndian.Uint16((*s)[l-2 : l])
	*s = (*s)[:l-2]
	return v
}

func (s *Far2lStack) PopU32() uint32 {
	l := len(*s)
	if l < 4 {
		return 0
	}
	v := binary.LittleEndian.Uint32((*s)[l-4 : l])
	*s = (*s)[:l-4]
	return v
}

func (s *Far2lStack) PopU64() uint64 {
	l := len(*s)
	if l < 8 {
		return 0
	}
	v := binary.LittleEndian.Uint64((*s)[l-8 : l])
	*s = (*s)[:l-8]
	return v
}

func (s *Far2lStack) PopBytes(n int) []byte {
	l := len(*s)
	if l < n {
		return nil
	}
	v := make([]byte, n)
	copy(v, (*s)[l-n:l])
	*s = (*s)[:l-n]
	return v
}

func (s *Far2lStack) PopString() string {
	n := s.PopU32()
	if n == 0xFFFFFFFF || n == 0 {
		return ""
	}
	return string(s.PopBytes(int(n)))
}

func (s *Far2lStack) PushU8(v uint8) {
	*s = append(*s, v)
}

func (s *Far2lStack) PushU16(v uint16) {
	*s = append(*s, 0, 0)
	binary.LittleEndian.PutUint16((*s)[len(*s)-2:], v)
}

func (s *Far2lStack) PushU32(v uint32) {
	*s = append(*s, 0, 0, 0, 0)
	binary.LittleEndian.PutUint32((*s)[len(*s)-4:], v)
}

func (s *Far2lStack) PushU64(v uint64) {
	*s = append(*s, 0, 0, 0, 0, 0, 0, 0, 0)
	binary.LittleEndian.PutUint64((*s)[len(*s)-8:], v)
}

func (s *Far2lStack) PushBytes(b []byte) {
	*s = append(*s, b...)
}

func (s *Far2lStack) PushString(str string) {
	s.PushBytes([]byte(str))
	s.PushU32(uint32(len(str)))
}