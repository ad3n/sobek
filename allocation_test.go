package sobek

import (
	"fmt"
	"math"
	"math/rand"
	"testing"
)

func BenchmarkMathHypotScratch(b *testing.B) {
	for _, size := range []int{2, 16, 64, 1024} {
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			r := New()
			args := make([]Value, size)
			for i := range args {
				args[i] = valueInt(0)
			}

			args[0], args[1] = valueInt(3), valueInt(4)
			call := FunctionCall{Arguments: args}
			b.ReportAllocs()
			for b.Loop() {
				if result := r.math_hypot(call); result != valueInt(5) {
					b.Fatal(result)
				}
			}
		})
	}
}

func BenchmarkJSONStringifyScratch(b *testing.B) {
	for _, source := range []string{`42`, `({a: 1, b: "text"})`, `({a: [1, 2, 3], b: {c: "text"}})`} {
		b.Run(source, func(b *testing.B) {
			r := New()
			value, err := r.RunString(source)
			if err != nil {
				b.Fatal(err)
			}

			call := FunctionCall{Arguments: []Value{value}}
			b.ReportAllocs()
			for b.Loop() {
				r.builtinJSON_stringify(call)
			}
		})
	}
}

func TestJSONScratchIsolation(t *testing.T) {
	for i := 0; i < 8; i++ {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			t.Parallel()
			testScriptWithTestLib(`
    const saved = [];
    for (let i = 0; i < 30; i++) {
     saved.push(JSON.stringify({text: "hello", n: i}));
     assert.sameValue(JSON.stringify({toJSON() { return JSON.stringify({nested: i}); }}),
      '"{\\"nested\\":' + i + '}"');
     const cyclic = {}; cyclic.self = cyclic;
     assert.throws(TypeError, () => JSON.stringify(cyclic));
     assert.throws(Error, () => JSON.stringify({get x() { throw new Error(); }}));
     assert.throws(Error, () => JSON.stringify({x: 1}, () => { throw new Error(); }));
     JSON.stringify({x: "é", y: 2}, ["x"], "  ");
     assert.sameValue(JSON.stringify({x: 1, y: 2}), '{"x":1,"y":2}');
     assert.sameValue(JSON.stringify(undefined), undefined);
    }
    const large = "a".repeat(100000);
    const largeResult = JSON.stringify(large);
    JSON.stringify({x: "other"});
    assert.sameValue(JSON.parse(largeResult), large);
    for (let i = 0; i < saved.length; i++) {
     assert.sameValue(saved[i], '{"text":"hello","n":' + i + '}');
    }
    for (let i = 0; i < 128; i++) {
     const s = String.fromCharCode(i);
     assert.sameValue(JSON.parse(JSON.stringify(s)), s);
    }
    assert.sameValue(JSON.stringify("\ud800"), '"\\ud800"');
    assert.sameValue(JSON.stringify("é😀"), '"é😀"');
   `, _undefined, t)
		})
	}
}

func TestMathHypotScratchReentry(t *testing.T) {
	testScriptWithTestLib(`
  const args = Array(64).fill(0);
  args[0] = 3;
  args[1] = {valueOf() {
   const inner = Array(64).fill(0);
   inner[0] = 5; inner[1] = 12;
   assert.sameValue(Math.hypot(...inner), 13);
   return 4;
  }};
  assert.sameValue(Math.hypot(...args), 5);
  args[2] = {valueOf() { throw new Error(); }};
  assert.throws(Error, () => Math.hypot(...args));
  args[2] = 0;
  assert.sameValue(Math.hypot(...args), 5);
  let calls = 0;
  args[0] = NaN; args[1] = Infinity;
  args[63] = {valueOf() { calls++; return 0; }};
  assert.sameValue(Math.hypot(...args), Infinity);
  assert.sameValue(calls, 1);
  const large = Array(5000).fill(0);
  large[0] = 3; large[1] = 4;
  assert.sameValue(Math.hypot(...large), 5);
  assert.sameValue(Math.hypot(3, 4), 5);
 `, _undefined, t)
}

func TestMathHypotTwoArgumentAgreement(t *testing.T) {
	r := New()
	random := rand.New(rand.NewSource(1))
	for i := 0; i < 10000; i++ {
		args := []Value{floatToValue(math.Float64frombits(random.Uint64())), floatToValue(math.Float64frombits(random.Uint64()))}
		var scratch [2]float64
		want := mathHypot(args, scratch[:0])
		got := r.math_hypot(FunctionCall{Arguments: args})
		if !got.SameAs(want) {
			t.Fatalf("hypot(%v, %v): got %v, want %v", args[0], args[1], got, want)
		}
	}
}
