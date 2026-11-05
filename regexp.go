package sobek

import (
	"fmt"
	"io"
	"sort"
	"unicode/utf16"

	"github.com/ad3n/regexp2/v2"

	"github.com/ad3n/sobek/unistring"
)

type regexp2MatchCache struct {
	target String
	runes  []rune
	posMap []int
}

// Not goroutine-safe. Use regexp2Wrapper.clone()
type regexp2Wrapper struct {
	checkInterrupt func()
	rx             *regexp2.Regexp
	cache          *regexp2MatchCache
}

// Not goroutine-safe. Use regexpPattern.clone()
type regexpPattern struct {
	regexp2Wrapper                                         *regexp2Wrapper
	global, ignoreCase, multiline, dotAll, sticky, unicode bool
}

type regexpResult struct {
	indexes []int
	groups  []string
}

func (result *regexpResult) appendIndexesAndGroup(index1, index2 int, name string) {
	result.indexes = append(result.indexes, index1, index2)
	result.groups = append(result.groups, name)
}

func compileRegexp2(src string, multiline, dotAll, ignoreCase, unicode bool) (*regexp2Wrapper, error) {
	var opts = regexp2.ECMAScript
	if multiline {
		opts |= regexp2.Multiline
	}
	if dotAll {
		opts |= regexp2.Singleline
	}
	if ignoreCase {
		opts |= regexp2.IgnoreCase
	}
	if unicode {
		opts |= regexp2.Unicode
	}
	regexp2Pattern, err1 := regexp2.Compile(src, opts)
	if err1 != nil {
		return nil, fmt.Errorf("Invalid regular expression (regexp2): %s (%v)", src, err1)
	}

	return &regexp2Wrapper{rx: regexp2Pattern}, nil
}

func (p *regexpPattern) findAllSubmatchIndex(s String, start int, limit int, sticky bool) []regexpResult {
	return p.regexp2Wrapper.findAllSubmatchIndex(s, start, limit, sticky, p.unicode)
}

// clone creates a copy of the regexpPattern which can be used concurrently.
func (p *regexpPattern) clone() *regexpPattern {
	ret := &regexpPattern{
		global:     p.global,
		ignoreCase: p.ignoreCase,
		multiline:  p.multiline,
		dotAll:     p.dotAll,
		sticky:     p.sticky,
		unicode:    p.unicode,
	}
	if p.regexp2Wrapper != nil {
		ret.regexp2Wrapper = p.regexp2Wrapper.clone()
	}
	return ret
}

type regexpObject struct {
	source String

	pattern *regexpPattern
	baseObject

	standard bool
}

func (r *regexp2Wrapper) findSubmatchIndex(s String, start int, unicode bool, captures *regexpResult) (int, int) {
	var runes []rune
	var posMap []int
	mappedStart := start
	var splitPair bool

	if cache := r.cache; cache != nil && cache.target.SameAs(s) && (cache.posMap != nil) == unicode {
		runes, posMap = cache.runes, cache.posMap
		if unicode {
			mappedStart, splitPair = posMapReverseLookup(posMap, start)
		}
	} else if unicode {
		posMap, runes, mappedStart, splitPair = buildPosMap(&lenientUtf16Decoder{utf16Reader: s.utf16Reader()}, s.Length(), start)
	} else if ascii, ok := s.(asciiString); ok && cache != nil && len(cache.runes) == len(ascii) {
		runes = cache.runes
		for i := range len(ascii) {
			runes[i] = rune(ascii[i])
		}

		cache.target = s
		cache.posMap = nil
	} else {
		runes = s.utf16Runes()
	}

	if splitPair {
		original := runes[mappedStart]
		_, runes[mappedStart] = utf16.EncodeRune(original)
		defer func() { runes[mappedStart] = original }()
	}

	first, end := -1, 0
	err := r.rx.FindRunesMatchesWithInterrupt(runes, mappedStart, r.checkInterrupt, func(match regexp2.MatchView) bool {
		first, end = matchRange(match, 0, posMap)
		if captures != nil {
			count := match.GroupCount()
			captures.indexes = make([]int, 0, count*2)
			captures.groups = make([]string, 0, count)
			captures.appendIndexesAndGroup(first, end, r.rx.GroupNameFromNumber(0))
			for i := 1; i < count; i++ {
				lo, hi := matchRange(match, i, posMap)
				captures.appendIndexesAndGroup(lo, hi, r.rx.GroupNameFromNumber(i))
			}
		}

		return false
	})
	if err != nil {
		panic(rangeError(err.Error()))
	}

	if first < 0 {
		r.cache = nil
		return -1, 0
	}

	if r.cache == nil {
		r.cache = new(regexp2MatchCache)
	}

	*r.cache = regexp2MatchCache{target: s, runes: runes, posMap: posMap}
	return first, end
}

func matchRange(match regexp2.MatchView, group int, posMap []int) (int, int) {
	start, length, ok := match.GroupRange(group)
	if !ok {
		return -1, 0
	}

	end := start + length
	if posMap != nil {
		return posMap[start], posMap[end]
	}

	return start, end
}

func (r *regexp2Wrapper) findAllSubmatchIndexUTF16(s String, start, limit int, sticky bool) []regexpResult {
	runes := s.utf16Runes()
	if limit < 0 {
		limit = len(runes) + 1
	}
	var results []regexpResult
	var names []string
	var flat []int
	err := r.rx.FindRunesMatchesWithInterrupt(runes, start, r.checkInterrupt, func(match regexp2.MatchView) bool {
		var result regexpResult
		if groupCount := match.GroupCount(); groupCount > 1 {
			if names == nil {
				names = make([]string, groupCount)
				for i := 1; i < groupCount; i++ {
					names[i] = r.rx.GroupNameFromNumber(i)
				}
			}
			base := len(flat)
			for i := range groupCount {
				if startPos, length, ok := match.GroupRange(i); ok {
					flat = append(flat, startPos, startPos+length)
				} else {
					flat = append(flat, -1, 0)
				}
			}
			result.indexes = flat[base:len(flat):len(flat)]
			result.groups = names
		} else {
			// Reading the whole-match interval directly avoids materialising
			// Group objects which the callers would not use anyway.
			base := len(flat)
			startPos, length, _ := match.GroupRange(0)
			flat = append(flat, startPos, startPos+length)
			result.indexes = flat[base:len(flat):len(flat)]
		}

		if sticky && len(result.indexes) > 1 {
			if result.indexes[0] != start {
				return false
			}
			if result.indexes[0] == result.indexes[1] {
				start = advanceStringIndex(s, start, false)
			} else {
				start = result.indexes[1]
			}
		}

		results = append(results, result)
		limit--
		return limit > 0
	})
	if err != nil {
		panic(rangeError(err.Error()))
	}
	return results
}

func buildPosMap(rd io.RuneReader, l, start int) (posMap []int, runes []rune, mappedStart int, splitPair bool) {
	posMap = make([]int, 0, l+1)
	curPos := 0
	runes = make([]rune, 0, l)
	startFound := false
	for {
		if !startFound {
			if curPos == start {
				mappedStart = len(runes)
				startFound = true
			}
			if curPos > start {
				// start position splits a surrogate pair
				mappedStart = len(runes) - 1
				splitPair = true
				startFound = true
			}
		}
		rn, size, err := rd.ReadRune()
		if err != nil {
			break
		}
		runes = append(runes, rn)
		posMap = append(posMap, curPos)
		curPos += size
	}
	posMap = append(posMap, curPos)
	return
}

func posMapReverseLookup(posMap []int, pos int) (int, bool) {
	mapped := sort.SearchInts(posMap, pos)
	if mapped < len(posMap) && posMap[mapped] != pos {
		return mapped - 1, true
	}
	return mapped, false
}

func (r *regexp2Wrapper) findAllSubmatchIndexUnicode(s unicodeString, start, limit int, sticky bool) []regexpResult {
	var posMap []int
	var runes []rune
	var mappedStart int
	var splitPair bool
	var savedRune rune
	if cache := r.cache; cache != nil && cache.posMap != nil && cache.target.SameAs(s) {
		runes, posMap = cache.runes, cache.posMap
		mappedStart, splitPair = posMapReverseLookup(posMap, start)
	} else {
		posMap, runes, mappedStart, splitPair = buildPosMap(&lenientUtf16Decoder{utf16Reader: s.utf16Reader()}, s.Length(), start)
	}
	if splitPair {
		// temporarily set the rune at mappedStart to the second code point of the pair
		_, second := utf16.EncodeRune(runes[mappedStart])
		savedRune, runes[mappedStart] = runes[mappedStart], second
		defer func() { runes[mappedStart] = savedRune }()
	}
	if limit < 0 {
		limit = len(s) + 1
	}
	var results []regexpResult
	var names []string
	var flat []int
	err := r.rx.FindRunesMatchesWithInterrupt(runes, mappedStart, r.checkInterrupt, func(match regexp2.MatchView) bool {
		var result regexpResult
		if groupCount := match.GroupCount(); groupCount > 1 {
			if names == nil {
				names = make([]string, groupCount)
				for i := 1; i < groupCount; i++ {
					names[i] = r.rx.GroupNameFromNumber(i)
				}
			}
			base := len(flat)
			for i := range groupCount {
				if startPos, length, ok := match.GroupRange(i); ok {
					flat = append(flat, posMap[startPos], posMap[startPos+length])
				} else {
					flat = append(flat, -1, 0)
				}
			}
			result.indexes = flat[base:len(flat):len(flat)]
			result.groups = names
		} else {
			base := len(flat)
			startPos, length, _ := match.GroupRange(0)
			flat = append(flat, posMap[startPos], posMap[startPos+length])
			result.indexes = flat[base:len(flat):len(flat)]
		}

		if sticky && len(result.indexes) > 1 {
			if result.indexes[0] != start {
				return false
			}
			if result.indexes[0] == result.indexes[1] {
				start = advanceStringIndex(s, start, false)
			} else {
				start = result.indexes[1]
			}
		}

		results = append(results, result)
		limit--
		return limit > 0
	})
	if err != nil {
		panic(rangeError(err.Error()))
	}
	return results
}

func (r *regexp2Wrapper) findAllSubmatchIndex(s String, start, limit int, sticky, fullUnicode bool) []regexpResult {
	a, u := devirtualizeString(s)
	if u != nil {
		if fullUnicode {
			return r.findAllSubmatchIndexUnicode(u, start, limit, sticky)
		}
		return r.findAllSubmatchIndexUTF16(u, start, limit, sticky)
	}
	return r.findAllSubmatchIndexUTF16(a, start, limit, sticky)
}

func (r *regexp2Wrapper) clone() *regexp2Wrapper {
	return &regexp2Wrapper{
		rx: r.rx,
	}
}

func (r *Runtime) createRegexpGroupsObj(valueArray []Value, groupNames []string) *Object {
	var groups *baseObject
	for index := 1; index < len(valueArray); index++ {
		if index < len(groupNames) {
			name := groupNames[index]
			if name == "" {
				continue
			} else if groups == nil {
				groups = r.newBaseObject(nil, classObject)
			}
			groups.setOwnStr(unistring.NewFromString(name), valueArray[index], false)
		}
	}
	if groups != nil {
		return groups.val
	}
	return nil
}

func createRegexpGroupsMap(result regexpResult) (groups map[unistring.String]int) {
	if len(result.groups) > 0 {
		groups = make(map[unistring.String]int)
		for i := 1; i < len(result.groups); i++ {
			if group := result.groups[i]; group != "" {
				idx := i * 2
				if idx < len(result.indexes)-1 {
					groups[unistring.NewFromString(group)] = idx
				}
			}
		}
	}
	return
}

func (r *regexpObject) execResultToArray(target String, result regexpResult) Value {
	captureCount := len(result.indexes) >> 1
	valueArray := make([]Value, captureCount)
	matchIndex := result.indexes[0]
	valueArray[0] = target.Substring(result.indexes[0], result.indexes[1])
	lowerBound := 0
	for index := 1; index < captureCount; index++ {
		offset := index << 1
		if result.indexes[offset] >= 0 && result.indexes[offset+1] >= lowerBound {
			valueArray[index] = target.Substring(result.indexes[offset], result.indexes[offset+1])
			lowerBound = result.indexes[offset]
		} else {
			valueArray[index] = _undefined
		}
	}
	match := r.val.runtime.newArrayValues(valueArray)
	match.self.setOwnStr("input", target, false)
	match.self.setOwnStr("index", intToValue(int64(matchIndex)), false)

	groups := r.val.runtime.createRegexpGroupsObj(valueArray, result.groups)

	var groupsVal Value
	if groups != nil {
		groupsVal = groups
	} else {
		groupsVal = _undefined
	}
	match.self.defineOwnPropertyStr("groups", PropertyDescriptor{
		Value:        groupsVal,
		Writable:     FLAG_TRUE,
		Configurable: FLAG_TRUE,
		Enumerable:   FLAG_TRUE,
	}, false)

	return match
}

func (r *regexpObject) getLastIndex() int64 {
	lastIndex := toLength(r.getStr("lastIndex", nil))
	if !r.pattern.global && !r.pattern.sticky {
		return 0
	}
	return lastIndex
}

func (r *regexpObject) execRegexp(target String, captures *regexpResult) int {
	index := r.getLastIndex()
	start, end := -1, 0
	if index >= 0 && index <= int64(target.Length()) {
		start, end = r.pattern.regexp2Wrapper.findSubmatchIndex(target, int(index), r.pattern.unicode, captures)
		if r.pattern.sticky && int64(start) != index {
			start, end = -1, 0
		}
	}

	if r.pattern.global || r.pattern.sticky {
		r.setOwnStr("lastIndex", intToValue(int64(end)), true)
	}

	return start
}

func (r *regexpObject) exec(target String) Value {
	var result regexpResult
	if r.execRegexp(target, &result) < 0 {
		return _null
	}

	return r.execResultToArray(target, result)
}

func (r *regexpObject) test(target String) bool {
	return r.execRegexp(target, nil) >= 0
}

func (r *regexpObject) clone() *regexpObject {
	r1 := r.val.runtime.newRegexpObject(r.prototype)
	r1.source = r.source
	r1.pattern = r.pattern

	return r1
}

func (r *regexpObject) init() {
	r.baseObject.init()
	r.standard = true
	r._putProp("lastIndex", intToValue(0), true, false, false)
}

func (r *regexpObject) setProto(proto *Object, throw bool) bool {
	res := r.baseObject.setProto(proto, throw)
	if res {
		r.standard = false
	}
	return res
}

func (r *regexpObject) defineOwnPropertyStr(name unistring.String, desc PropertyDescriptor, throw bool) bool {
	res := r.baseObject.defineOwnPropertyStr(name, desc, throw)
	if res {
		r.standard = false
	}
	return res
}

func (r *regexpObject) defineOwnPropertySym(name *Symbol, desc PropertyDescriptor, throw bool) bool {
	res := r.baseObject.defineOwnPropertySym(name, desc, throw)
	if res && r.standard {
		switch name {
		case SymMatch, SymMatchAll, SymSearch, SymSplit, SymReplace:
			r.standard = false
		}
	}
	return res
}

func (r *regexpObject) deleteStr(name unistring.String, throw bool) bool {
	res := r.baseObject.deleteStr(name, throw)
	if res {
		r.standard = false
	}
	return res
}

func (r *regexpObject) setOwnStr(name unistring.String, value Value, throw bool) bool {
	res := r.baseObject.setOwnStr(name, value, throw)
	if res && r.standard && name == "exec" {
		r.standard = false
	}
	return res
}

func (r *regexpObject) setOwnSym(name *Symbol, value Value, throw bool) bool {
	res := r.baseObject.setOwnSym(name, value, throw)
	if res && r.standard {
		switch name {
		case SymMatch, SymMatchAll, SymSearch, SymSplit, SymReplace:
			r.standard = false
		}
	}
	return res
}
