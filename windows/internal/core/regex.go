package core

// A small backtracking regular-expression engine with ICU semantics.
//
// The Swift guards run their patterns through NSRegularExpression, i.e. ICU, where \w, \s
// and \b are Unicode-aware. Go's regexp (RE2) makes \w and \b ASCII-only and has no
// lookaround to rebuild \b with, so a Cyrillic word would count as "not a word" and
// `\b\w+(?:_\w+)+\b` would protect the wrong things. Rather than hand-translate each
// pattern, this engine implements the subset of ICU syntax the guards use, with ICU's
// definitions, so the Swift pattern strings are copied verbatim and future pattern changes
// port by copy-paste.
//
// Supported: literals and escapes, `.`, classes `[...]` / `[^...]` with ranges and \w \W
// \s \S \d \D inside, groups `(...)` / `(?:...)` (nothing is captured), alternation,
// quantifiers `* + ? {n} {n,} {n,m}` and their lazy `?` forms, anchors `^ $ \b \B`.
//
// ICU definitions used (regexst.cpp / rematch.cpp):
//   \w  [\p{Alphabetic}\p{M}\p{Nd}\p{Pc}\u200C\u200D]
//   \s  [\p{White_Space}]
//   \d  \p{Nd}
//   \b  word/non-word transition, where the character at the position being a
//       Grapheme_Extend or format (Cf) character means "no boundary", and the character
//       before is found by skipping back over Grapheme_Extend/Cf characters.
// Matching is leftmost-first with backtracking, as in ICU; FindAll returns the same
// non-overlapping sequence NSRegularExpression.matches(in:) does.

import (
	"fmt"
	"strconv"
	"unicode"
)

type reOp uint8

const (
	opChar reOp = iota
	opClass
	opAny
	opSeq
	opAlt
	opRepeat
	opBegin
	opEnd
	opWordBoundary
	opNotWordBoundary
)

type reNode struct {
	op     reOp
	r      rune
	class  *reClass
	subs   []*reNode
	min    int
	max    int // < 0: unbounded
	greedy bool
}

type reClass struct {
	negated bool
	ranges  []rune // lo, hi pairs
	sets    []func(rune) bool
}

func (c *reClass) matches(r rune) bool {
	in := false
	for i := 0; i+1 < len(c.ranges); i += 2 {
		if r >= c.ranges[i] && r <= c.ranges[i+1] {
			in = true
			break
		}
	}
	if !in {
		for _, set := range c.sets {
			if set(r) {
				in = true
				break
			}
		}
	}
	return in != c.negated
}

// ICU character sets.

func icuIsWord(r rune) bool {
	return isAlphabetic(r) || unicode.In(r, unicode.M) || unicode.Is(unicode.Nd, r) ||
		unicode.Is(unicode.Pc, r) || r == 0x200C || r == 0x200D
}

func icuIsSpace(r rune) bool { return unicode.Is(unicode.White_Space, r) }

func icuIsDigit(r rune) bool { return unicode.Is(unicode.Nd, r) }

func notSet(f func(rune) bool) func(rune) bool { return func(r rune) bool { return !f(r) } }

func isLineTerminator(r rune) bool {
	switch r {
	case '\n', 0x0B, '\f', '\r', 0x85, 0x2028, 0x2029:
		return true
	}
	return false
}

// icuRegex is a compiled pattern.
type icuRegex struct {
	source string
	root   *reNode
}

func compileICU(pattern string) (*icuRegex, error) {
	p := &reParser{src: []rune(pattern)}
	root, err := p.parseAlt()
	if err != nil {
		return nil, fmt.Errorf("%q: %w", pattern, err)
	}
	if p.pos != len(p.src) {
		return nil, fmt.Errorf("%q: unexpected %q at %d", pattern, p.src[p.pos], p.pos)
	}
	return &icuRegex{source: pattern, root: root}, nil
}

func mustCompileICU(pattern string) *icuRegex {
	re, err := compileICU(pattern)
	if err != nil {
		panic(err)
	}
	return re
}

// MARK: - Parser

type reParser struct {
	src []rune
	pos int
}

func (p *reParser) peek() (rune, bool) {
	if p.pos < len(p.src) {
		return p.src[p.pos], true
	}
	return 0, false
}

func (p *reParser) parseAlt() (*reNode, error) {
	var branches []*reNode
	for {
		seq, err := p.parseSeq()
		if err != nil {
			return nil, err
		}
		branches = append(branches, seq)
		if r, ok := p.peek(); ok && r == '|' {
			p.pos++
			continue
		}
		break
	}
	if len(branches) == 1 {
		return branches[0], nil
	}
	return &reNode{op: opAlt, subs: branches}, nil
}

func (p *reParser) parseSeq() (*reNode, error) {
	var items []*reNode
	for {
		r, ok := p.peek()
		if !ok || r == '|' || r == ')' {
			break
		}
		atom, err := p.parseAtom()
		if err != nil {
			return nil, err
		}
		atom, err = p.parseQuantifier(atom)
		if err != nil {
			return nil, err
		}
		items = append(items, atom)
	}
	if len(items) == 1 {
		return items[0], nil
	}
	return &reNode{op: opSeq, subs: items}, nil
}

func (p *reParser) parseAtom() (*reNode, error) {
	r := p.src[p.pos]
	p.pos++
	switch r {
	case '(':
		if p.pos+1 < len(p.src) && p.src[p.pos] == '?' {
			if p.src[p.pos+1] != ':' {
				return nil, fmt.Errorf("unsupported group syntax at %d", p.pos)
			}
			p.pos += 2
		}
		inner, err := p.parseAlt()
		if err != nil {
			return nil, err
		}
		if r, ok := p.peek(); !ok || r != ')' {
			return nil, fmt.Errorf("missing )")
		}
		p.pos++
		return &reNode{op: opSeq, subs: []*reNode{inner}}, nil
	case '[':
		return p.parseClass()
	case '.':
		return &reNode{op: opAny}, nil
	case '^':
		return &reNode{op: opBegin}, nil
	case '$':
		return &reNode{op: opEnd}, nil
	case '\\':
		return p.parseEscape(false)
	case '*', '+', '?', '{':
		return nil, fmt.Errorf("quantifier %q without operand at %d", r, p.pos-1)
	}
	return &reNode{op: opChar, r: r}, nil
}

// parseEscape parses after a backslash. Inside a class, \b is not an assertion.
func (p *reParser) parseEscape(inClass bool) (*reNode, error) {
	if p.pos >= len(p.src) {
		return nil, fmt.Errorf("trailing backslash")
	}
	r := p.src[p.pos]
	p.pos++
	set := func(f func(rune) bool) *reNode {
		return &reNode{op: opClass, class: &reClass{sets: []func(rune) bool{f}}}
	}
	switch r {
	case 'w':
		return set(icuIsWord), nil
	case 'W':
		return set(notSet(icuIsWord)), nil
	case 's':
		return set(icuIsSpace), nil
	case 'S':
		return set(notSet(icuIsSpace)), nil
	case 'd':
		return set(icuIsDigit), nil
	case 'D':
		return set(notSet(icuIsDigit)), nil
	case 'b':
		if !inClass {
			return &reNode{op: opWordBoundary}, nil
		}
		return &reNode{op: opChar, r: '\b'}, nil
	case 'B':
		if !inClass {
			return &reNode{op: opNotWordBoundary}, nil
		}
	case 'n':
		return &reNode{op: opChar, r: '\n'}, nil
	case 't':
		return &reNode{op: opChar, r: '\t'}, nil
	case 'r':
		return &reNode{op: opChar, r: '\r'}, nil
	case 'f':
		return &reNode{op: opChar, r: '\f'}, nil
	case 'u':
		if p.pos+4 <= len(p.src) {
			if v, err := strconv.ParseUint(string(p.src[p.pos:p.pos+4]), 16, 32); err == nil {
				p.pos += 4
				return &reNode{op: opChar, r: rune(v)}, nil
			}
		}
		return nil, fmt.Errorf("bad \\u escape")
	}
	if unicode.IsLetter(r) || unicode.IsDigit(r) {
		return nil, fmt.Errorf("unsupported escape \\%c", r)
	}
	return &reNode{op: opChar, r: r}, nil
}

func (p *reParser) parseClass() (*reNode, error) {
	class := &reClass{}
	if r, ok := p.peek(); ok && r == '^' {
		class.negated = true
		p.pos++
	}
	first := true
	for {
		r, ok := p.peek()
		if !ok {
			return nil, fmt.Errorf("missing ]")
		}
		if r == ']' && !first {
			p.pos++
			break
		}
		first = false
		var lo rune
		if r == '\\' {
			p.pos++
			node, err := p.parseEscape(true)
			if err != nil {
				return nil, err
			}
			if node.op == opClass {
				class.sets = append(class.sets, node.class.sets...)
				continue
			}
			lo = node.r
		} else {
			lo = r
			p.pos++
		}
		// A range, unless the '-' is the last thing before ']'.
		if p.pos+1 < len(p.src) && p.src[p.pos] == '-' && p.src[p.pos+1] != ']' {
			p.pos++
			hi := p.src[p.pos]
			p.pos++
			if hi == '\\' {
				node, err := p.parseEscape(true)
				if err != nil || node.op != opChar {
					return nil, fmt.Errorf("bad range end")
				}
				hi = node.r
			}
			if hi < lo {
				return nil, fmt.Errorf("inverted range")
			}
			class.ranges = append(class.ranges, lo, hi)
			continue
		}
		class.ranges = append(class.ranges, lo, lo)
	}
	return &reNode{op: opClass, class: class}, nil
}

func (p *reParser) parseQuantifier(atom *reNode) (*reNode, error) {
	r, ok := p.peek()
	if !ok {
		return atom, nil
	}
	min, max := 0, 0
	switch r {
	case '*':
		min, max = 0, -1
		p.pos++
	case '+':
		min, max = 1, -1
		p.pos++
	case '?':
		min, max = 0, 1
		p.pos++
	case '{':
		end := p.pos + 1
		for end < len(p.src) && p.src[end] != '}' {
			end++
		}
		if end >= len(p.src) {
			return nil, fmt.Errorf("missing }")
		}
		body := string(p.src[p.pos+1 : end])
		var err error
		if comma := indexRune(body, ','); comma < 0 {
			min, err = strconv.Atoi(body)
			max = min
		} else {
			min, err = strconv.Atoi(body[:comma])
			if err == nil {
				if rest := body[comma+1:]; rest == "" {
					max = -1
				} else {
					max, err = strconv.Atoi(rest)
				}
			}
		}
		if err != nil {
			return nil, fmt.Errorf("bad {} quantifier")
		}
		p.pos = end + 1
	default:
		return atom, nil
	}
	greedy := true
	if r, ok := p.peek(); ok && r == '?' {
		greedy = false
		p.pos++
	}
	return &reNode{op: opRepeat, subs: []*reNode{atom}, min: min, max: max, greedy: greedy}, nil
}

func indexRune(s string, r rune) int {
	for i, c := range s {
		if c == r {
			return i
		}
	}
	return -1
}

// MARK: - Matcher

type reMatcher struct {
	in []rune
}

func (m *reMatcher) single(n *reNode, r rune) bool {
	switch n.op {
	case opChar:
		return r == n.r
	case opClass:
		return n.class.matches(r)
	case opAny:
		return !isLineTerminator(r)
	}
	return false
}

func isSingle(n *reNode) bool { return n.op == opChar || n.op == opClass || n.op == opAny }

func (m *reMatcher) match(n *reNode, i int, k func(int) bool) bool {
	switch n.op {
	case opChar, opClass, opAny:
		if i < len(m.in) && m.single(n, m.in[i]) {
			return k(i + 1)
		}
		return false
	case opSeq:
		return m.matchSeq(n.subs, 0, i, k)
	case opAlt:
		for _, branch := range n.subs {
			if m.match(branch, i, k) {
				return true
			}
		}
		return false
	case opRepeat:
		return m.matchRepeat(n, i, k)
	case opBegin:
		return i == 0 && k(i)
	case opEnd:
		atEnd := i == len(m.in) ||
			(i == len(m.in)-1 && isLineTerminator(m.in[i])) ||
			(i == len(m.in)-2 && m.in[i] == '\r' && m.in[i+1] == '\n')
		return atEnd && k(i)
	case opWordBoundary:
		return m.isWordBoundary(i) && k(i)
	case opNotWordBoundary:
		return !m.isWordBoundary(i) && k(i)
	}
	return false
}

func (m *reMatcher) matchSeq(subs []*reNode, index, i int, k func(int) bool) bool {
	if index == len(subs) {
		return k(i)
	}
	return m.match(subs[index], i, func(j int) bool { return m.matchSeq(subs, index+1, j, k) })
}

func (m *reMatcher) matchRepeat(n *reNode, i int, k func(int) bool) bool {
	sub := n.subs[0]
	if isSingle(sub) {
		count := 0
		for j := i; j < len(m.in) && (n.max < 0 || count < n.max) && m.single(sub, m.in[j]); j++ {
			count++
		}
		if count < n.min {
			return false
		}
		if n.greedy {
			for c := count; c >= n.min; c-- {
				if k(i + c) {
					return true
				}
			}
			return false
		}
		for c := n.min; c <= count; c++ {
			if k(i + c) {
				return true
			}
		}
		return false
	}
	return m.repeatGeneral(n, i, 0, k)
}

func (m *reMatcher) repeatGeneral(n *reNode, i, count int, k func(int) bool) bool {
	more := func() bool {
		if n.max >= 0 && count >= n.max {
			return false
		}
		return m.match(n.subs[0], i, func(j int) bool {
			if j == i {
				return false // an empty iteration can never make progress
			}
			return m.repeatGeneral(n, j, count+1, k)
		})
	}
	if n.greedy {
		return more() || (count >= n.min && k(i))
	}
	return (count >= n.min && k(i)) || more()
}

func icuSkipsForBoundary(r rune) bool {
	return isGraphemeExtend(r) || unicode.Is(unicode.Cf, r)
}

// isWordBoundary is RegexMatcher::isWordBoundary from ICU.
func (m *reMatcher) isWordBoundary(pos int) bool {
	cIsWord := false
	if pos < len(m.in) {
		c := m.in[pos]
		if icuSkipsForBoundary(c) {
			return false
		}
		cIsWord = icuIsWord(c)
	}
	prevIsWord := false
	for p := pos - 1; p >= 0; p-- {
		if !icuSkipsForBoundary(m.in[p]) {
			prevIsWord = icuIsWord(m.in[p])
			break
		}
	}
	return cIsWord != prevIsWord
}

// FindAll returns the [start, end) rune ranges of all non-overlapping matches, scanning
// left to right like NSRegularExpression.matches(in:options:range:).
func (re *icuRegex) FindAll(in []rune) [][2]int {
	m := reMatcher{in: in}
	var out [][2]int
	pos := 0
	for pos <= len(in) {
		found := false
		for start := pos; start <= len(in); start++ {
			end := -1
			if m.match(re.root, start, func(e int) bool { end = e; return true }) {
				out = append(out, [2]int{start, end})
				if end == start {
					pos = end + 1
				} else {
					pos = end
				}
				found = true
				break
			}
		}
		if !found {
			break
		}
	}
	return out
}
