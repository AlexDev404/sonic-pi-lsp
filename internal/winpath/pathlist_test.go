package winpath

import "testing"

func TestWith(t *testing.T) {
	long := ""
	for i := 0; i < 200; i++ {
		long += `C:\Some\Long\Tool\Directory\bin;`
	}
	got, changed := With(long, `C:\Program Files\Sonic Pi CLI`)
	if !changed || got != long+`C:\Program Files\Sonic Pi CLI` {
		t.Fatalf("long PATH not kept intact")
	}
	if _, changed := With(`C:\a;c:\program files\sonic pi cli\`, `C:\Program Files\Sonic Pi CLI`); changed {
		t.Fatal("duplicate added")
	}
	if got, _ := With(`C:\a`, `D:\b`); got != `C:\a;D:\b` {
		t.Fatalf("got %q", got)
	}
}

func TestWithout(t *testing.T) {
	got, changed := Without(`%SystemRoot%\system32;C:\Program Files\Sonic Pi CLI;C:\x;;C:\y`, `c:\program files\sonic pi cli\`)
	if !changed || got != `%SystemRoot%\system32;C:\x;;C:\y` {
		t.Fatalf("got %q", got)
	}
	if _, changed := Without(`C:\x`, `C:\Program Files\Sonic Pi CLI`); changed {
		t.Fatal("changed a PATH without the entry")
	}
}
