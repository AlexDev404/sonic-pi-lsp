package osc

import (
	"reflect"
	"testing"
	"time"
)

func TestRoundTrip(t *testing.T) {
	in := NewMessage("/log/multi_message", int32(1), `:drums`, "0.5", int32(2), int32(0), "synth :beep", 3.5, float32(1.25), []byte{1, 2, 3}, true, false)
	b, err := in.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if len(b)%4 != 0 {
		t.Fatalf("packet not aligned: %d", len(b))
	}
	out, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 {
		t.Fatalf("want 1 message, got %d", len(out))
	}
	if out[0].Address != in.Address {
		t.Fatalf("address %q", out[0].Address)
	}
	if !reflect.DeepEqual(out[0].Args, in.Args) {
		t.Fatalf("args differ:\n got %#v\nwant %#v", out[0].Args, in.Args)
	}
}

func TestIntPromotion(t *testing.T) {
	b, _ := NewMessage("/x", 7, 1<<40).Encode()
	out, _ := Decode(b)
	if out[0].Args[0] != int32(7) || out[0].Args[1] != int64(1<<40) {
		t.Fatalf("got %#v", out[0].Args)
	}
}

func TestBundle(t *testing.T) {
	m1, _ := NewMessage("/a", int32(1)).Encode()
	m2, _ := NewMessage("/b", "x").Encode()
	var b []byte
	b = append(b, []byte("#bundle\x00")...)
	b = append(b, make([]byte, 8)...)
	for _, m := range [][]byte{m1, m2} {
		n := len(m)
		b = append(b, byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
		b = append(b, m...)
	}
	out, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 2 || out[0].Address != "/a" || out[1].Str(0) != "x" {
		t.Fatalf("got %v", out)
	}
}

func TestUDP(t *testing.T) {
	srv, err := Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	got := make(chan *Message, 1)
	go srv.Serve(func(m *Message) { got <- m })
	c, err := Dial("127.0.0.1", srv.Port())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Send("/ping", int32(42), "hello"); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-got:
		if n, _ := m.Int(0); n != 42 || m.Str(1) != "hello" {
			t.Fatalf("got %v", m)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout")
	}
}
