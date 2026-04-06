package vtinput

import (
	"reflect"
	"testing"
)

func TestFar2lStack_Integrity(t *testing.T) {
	stk := Far2lStack{}

	// Test LIFO property with mixed sizes
	stk.PushU8(0xAA)
	stk.PushU16(0xBBCC)
	stk.PushU32(0x11223344)
	stk.PushString("f4")

	if s := stk.PopString(); s != "f4" {
		t.Errorf("PopString failed: got %q", s)
	}
	if v := stk.PopU32(); v != 0x11223344 {
		t.Errorf("PopU32 failed: got %X", v)
	}
	if v := stk.PopU16(); v != 0xBBCC {
		t.Errorf("PopU16 failed: got %X", v)
	}
	if v := stk.PopU8(); v != 0xAA {
		t.Errorf("PopU8 failed: got %X", v)
	}

	// Test underflow safety
	if v := stk.PopU8(); v != 0 {
		t.Error("Pop on empty stack should return 0")
	}
}

func TestFar2lStack_Bytes(t *testing.T) {
	stk := Far2lStack{}
	payload := []byte{1, 2, 3, 4, 5}
	stk.PushBytes(payload)

	got := stk.PopBytes(3)
	if !reflect.DeepEqual(got, []byte{3, 4, 5}) {
		t.Errorf("PopBytes (tail) failed: %v", got)
	}

	if len(stk) != 2 {
		t.Errorf("Stack size mismatch: expected 2, got %d", len(stk))
	}
}