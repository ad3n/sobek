package sobek

import (
	"reflect"
	"testing"
)

// Exporting a Map or Set that contains itself, directly or through other
// objects, must terminate and preserve the cycle, as it already does for
// plain objects and arrays. Without the cache lookup in mapObject.export and
// setObject.export these recurse until the Go stack overflows, which is a
// fatal error (the whole test binary dies), not a recoverable panic.
func TestExportSelfContainingMapSet(t *testing.T) {
	samePtr := func(a, b any) bool {
		return reflect.ValueOf(a).Pointer() == reflect.ValueOf(b).Pointer()
	}

	t.Run("set contains itself", func(t *testing.T) {
		v, err := New().RunString(`var s = new Set(); s.add(s); s`)
		if err != nil {
			t.Fatal(err)
		}
		a, ok := v.Export().([]any)
		if !ok || len(a) != 1 {
			t.Fatalf("unexpected export: %#v", v.Export())
		}
		if !samePtr(a[0], a) {
			t.Fatal("expected the exported set to contain itself")
		}
	})

	t.Run("map contains itself", func(t *testing.T) {
		v, err := New().RunString(`var m = new Map(); m.set("a", m); m`)
		if err != nil {
			t.Fatal(err)
		}
		m, ok := v.Export().([][2]any)
		if !ok || len(m) != 1 {
			t.Fatalf("unexpected export: %#v", v.Export())
		}
		if !samePtr(m[0][1], m) {
			t.Fatal("expected the exported map to contain itself")
		}
	})

	t.Run("map cycle through an object", func(t *testing.T) {
		v, err := New().RunString(`var m = new Map(); var o = {m: m}; m.set("o", o); o`)
		if err != nil {
			t.Fatal(err)
		}
		o, ok := v.Export().(map[string]any)
		if !ok {
			t.Fatalf("unexpected export: %#v", v.Export())
		}
		m := o["m"].([][2]any)
		if !samePtr(m[0][1], o) {
			t.Fatal("expected the map entry to be the exported object")
		}
	})

	t.Run("set cycle through an array", func(t *testing.T) {
		v, err := New().RunString(`var s = new Set(); s.add([s]); s`)
		if err != nil {
			t.Fatal(err)
		}
		a := v.Export().([]any)
		inner := a[0].([]any)
		if !samePtr(inner[0], a) {
			t.Fatal("expected the inner array to hold the exported set")
		}
	})

	t.Run("set inside an array", func(t *testing.T) {
		v, err := New().RunString(`var s = new Set(); s.add(s); [s]`)
		if err != nil {
			t.Fatal(err)
		}
		a := v.Export().([]any)
		s := a[0].([]any)
		if !samePtr(s[0], s) {
			t.Fatal("expected the set to contain itself")
		}
	})

	t.Run("assignment to a Go struct field", func(t *testing.T) {
		type rec struct{ After any }
		r := New()
		var target rec
		_ = r.Set("r", &target)
		if _, err := r.RunString(`var s = new Set(); s.add(s); r.After = {s: s}`); err != nil {
			t.Fatal(err)
		}
		if target.After == nil {
			t.Fatal("expected the assigned value to be stored")
		}
	})
}
