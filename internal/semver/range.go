package semver

import (
	"fmt"
	"strconv"
	"strings"
)

// Comparator 表示一个原始比较项：操作符 + 基准版本。Op 为 "ANY" 时表示通配 *。
type Comparator struct {
	Op     string  // ">=", ">", "<=", "<", "=", "ANY"
	Semver Version // 比较基准版本
}

// ComparatorSet 为一组以空格连接（逻辑与）的比较项。
type ComparatorSet []Comparator

// RangeSet 为多组以 "||" 连接（逻辑或）的比较项集合。
type RangeSet []ComparatorSet

// partial 表示范围项中的部分版本：缺省或通配的段为 nil。
type partial struct {
	major *int
	minor *int
	patch *int
	pre   []string
}

// ParseRange 将范围表达式解析为 RangeSet。
//
// 同一组内多个约束以空格连接表示“与”，多组之间以双竖线 || 连接表示“或”。
// 支持精确版本、比较操作符 >= > <= < =、兼容版本 ^、邻近版本 ~ 与通配符 x/X/*。
func ParseRange(s string) (RangeSet, error) {
	if strings.TrimSpace(s) == "" {
		return nil, ErrEmptyRange
	}
	groups := strings.Split(s, "||")
	rs := make(RangeSet, 0, len(groups))
	for _, g := range groups {
		toks := strings.Fields(g)
		if len(toks) == 0 {
			return nil, fmt.Errorf("非法范围表达式 %q: 存在空的范围组", s)
		}
		set := make(ComparatorSet, 0, len(toks))
		for _, tok := range toks {
			op, vspec, err := splitOp(tok)
			if err != nil {
				return nil, err
			}
			if vspec == "" {
				return nil, fmt.Errorf("非法范围项 %q: 缺少版本号", tok)
			}
			p, err := parsePartial(vspec, tok)
			if err != nil {
				return nil, err
			}
			cs, err := desugar(op, tok, p)
			if err != nil {
				return nil, err
			}
			set = append(set, cs...)
		}
		rs = append(rs, set)
	}
	return rs, nil
}

// splitOp 从范围项中分离操作符前缀与版本说明。
func splitOp(tok string) (op, vspec string, err error) {
	switch {
	case strings.HasPrefix(tok, "^"):
		return "^", tok[1:], nil
	case strings.HasPrefix(tok, "~"):
		return "~", tok[1:], nil
	case strings.HasPrefix(tok, ">="):
		return ">=", tok[2:], nil
	case strings.HasPrefix(tok, "<="):
		return "<=", tok[2:], nil
	case strings.HasPrefix(tok, ">"):
		return ">", tok[1:], nil
	case strings.HasPrefix(tok, "<"):
		return "<", tok[1:], nil
	case strings.HasPrefix(tok, "="):
		return "=", tok[1:], nil
	default:
		return "", tok, nil
	}
}

// parsePartial 解析版本说明为部分版本，允许缺省或通配段。
func parsePartial(s, tok string) (partial, error) {
	var p partial
	rest := s
	// 预发布标识以 "-" 引导。
	if i := strings.Index(rest, "-"); i >= 0 {
		prePart := rest[i+1:]
		rest = rest[:i]
		ps, err := parsePreSegs(prePart, tok)
		if err != nil {
			return p, err
		}
		p.pre = ps
	}
	// 范围项中的构建元数据允许出现但被忽略。
	if i := strings.Index(rest, "+"); i >= 0 {
		rest = rest[:i]
	}
	core := strings.Split(rest, ".")
	if len(core) == 0 || len(core) > 3 {
		return p, fmt.Errorf("非法范围项 %q: 版本段数非法", tok)
	}
	segToInt := func(seg string) (*int, error) {
		if seg == "x" || seg == "X" || seg == "*" {
			return nil, nil // nil 表示通配/缺省
		}
		if !coreSegRe.MatchString(seg) {
			return nil, fmt.Errorf("非法范围项 %q: 版本段非法 %q", tok, seg)
		}
		n, err := strconv.Atoi(seg)
		if err != nil {
			return nil, fmt.Errorf("非法范围项 %q: 版本段越界 %q", tok, seg)
		}
		return &n, nil
	}
	m, err := segToInt(core[0])
	if err != nil {
		return p, err
	}
	p.major = m
	if len(core) >= 2 {
		mn, err := segToInt(core[1])
		if err != nil {
			return p, err
		}
		p.minor = mn
	}
	if len(core) >= 3 {
		pa, err := segToInt(core[2])
		if err != nil {
			return p, err
		}
		p.patch = pa
	}
	return p, nil
}

// desugar 将带操作符的部分版本展开为原始比较项集合。
func desugar(op, tok string, p partial) (ComparatorSet, error) {
	isXM := p.major == nil
	isXm := p.minor == nil
	isXp := p.patch == nil
	M, m, pa := 0, 0, 0
	if p.major != nil {
		M = *p.major
	}
	if p.minor != nil {
		m = *p.minor
	}
	if p.patch != nil {
		pa = *p.patch
	}
	pre := p.pre

	mk := func(op string, maj, min, pat int, pre []string) Comparator {
		return Comparator{Op: op, Semver: Version{Major: maj, Minor: min, Patch: pat, Prerelease: pre}}
	}
	any := Comparator{Op: "ANY"}

	switch op {
	case "^":
		// 允许不改变最左非零位的变更。
		if isXM {
			return ComparatorSet{any}, nil
		}
		if isXm {
			return ComparatorSet{mk(">=", M, 0, 0, pre), mk("<", M+1, 0, 0, nil)}, nil
		}
		if isXp {
			if M == 0 {
				return ComparatorSet{mk(">=", 0, m, 0, pre), mk("<", 0, m+1, 0, nil)}, nil
			}
			return ComparatorSet{mk(">=", M, m, 0, pre), mk("<", M+1, 0, 0, nil)}, nil
		}
		if M == 0 {
			if m == 0 {
				return ComparatorSet{mk(">=", 0, 0, pa, pre), mk("<", 0, m+1, 0, nil)}, nil
			}
			return ComparatorSet{mk(">=", 0, m, pa, pre), mk("<", 0, m+1, 0, nil)}, nil
		}
		return ComparatorSet{mk(">=", M, m, pa, pre), mk("<", M+1, 0, 0, nil)}, nil
	case "~":
		// 给出次版本时允许修订号层变更；仅给出主版本时允许次版本层变更。
		if isXM {
			return ComparatorSet{any}, nil
		}
		if isXm {
			return ComparatorSet{mk(">=", M, 0, 0, pre), mk("<", M+1, 0, 0, nil)}, nil
		}
		if isXp {
			return ComparatorSet{mk(">=", M, m, 0, pre), mk("<", M, m+1, 0, nil)}, nil
		}
		return ComparatorSet{mk(">=", M, m, pa, pre), mk("<", M, m+1, 0, nil)}, nil
	case "", "=":
		if isXM {
			return ComparatorSet{any}, nil
		}
		if isXm {
			return ComparatorSet{mk(">=", M, 0, 0, nil), mk("<", M+1, 0, 0, nil)}, nil
		}
		if isXp {
			return ComparatorSet{mk(">=", M, m, 0, nil), mk("<", M, m+1, 0, nil)}, nil
		}
		return ComparatorSet{mk("=", M, m, pa, pre)}, nil
	case ">=":
		if isXM {
			return ComparatorSet{any}, nil
		}
		if isXm {
			return ComparatorSet{mk(">=", M, 0, 0, nil)}, nil
		}
		if isXp {
			return ComparatorSet{mk(">=", M, m, 0, nil)}, nil
		}
		return ComparatorSet{mk(">=", M, m, pa, pre)}, nil
	case ">":
		if isXM {
			return nil, fmt.Errorf("非法范围项 %q: > 与通配符组合无下界", tok)
		}
		if isXm {
			return ComparatorSet{mk(">=", M+1, 0, 0, nil)}, nil
		}
		if isXp {
			return ComparatorSet{mk(">=", M, m+1, 0, nil)}, nil
		}
		return ComparatorSet{mk(">", M, m, pa, pre)}, nil
	case "<=":
		if isXM {
			return ComparatorSet{any}, nil
		}
		if isXm {
			return ComparatorSet{mk("<", M+1, 0, 0, nil)}, nil
		}
		if isXp {
			return ComparatorSet{mk("<", M, m+1, 0, nil)}, nil
		}
		return ComparatorSet{mk("<=", M, m, pa, pre)}, nil
	case "<":
		if isXM {
			return nil, fmt.Errorf("非法范围项 %q: < 与通配符组合无意义", tok)
		}
		if isXm {
			return ComparatorSet{mk("<", M, 0, 0, nil)}, nil
		}
		if isXp {
			return ComparatorSet{mk("<", M, m, 0, nil)}, nil
		}
		return ComparatorSet{mk("<", M, m, pa, pre)}, nil
	}
	return nil, fmt.Errorf("未知操作符 %q", op)
}

// Satisfies 判断版本是否满足任一范围组。
func (rs RangeSet) Satisfies(v Version) bool {
	for _, set := range rs {
		if setSatisfies(set, v) {
			return true
		}
	}
	return false
}

// setSatisfies 判断版本是否满足单组比较项（组内逻辑与）。
//
// 预发布版本默认被排除：仅当组内存在一个与待判版本主.次.修订完全相同、
// 且自身也带预发布标识的比较项时，带预发布标识的版本才被允许。
func setSatisfies(set ComparatorSet, v Version) bool {
	for _, c := range set {
		if !c.test(v) {
			return false
		}
	}
	if len(v.Prerelease) > 0 {
		allowed := false
		for _, c := range set {
			if c.Op == "ANY" {
				continue
			}
			if len(c.Semver.Prerelease) == 0 {
				continue
			}
			if c.Semver.Major == v.Major && c.Semver.Minor == v.Minor && c.Semver.Patch == v.Patch {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}
	return true
}

func (c Comparator) test(v Version) bool {
	switch c.Op {
	case "ANY":
		return true
	case "=":
		return Compare(v, c.Semver) == 0
	case ">":
		return Compare(v, c.Semver) > 0
	case ">=":
		return Compare(v, c.Semver) >= 0
	case "<":
		return Compare(v, c.Semver) < 0
	case "<=":
		return Compare(v, c.Semver) <= 0
	}
	return false
}

// SatisfiesRange 解析范围表达式并判断版本是否满足。
func SatisfiesRange(v Version, rangeStr string) (bool, error) {
	rs, err := ParseRange(rangeStr)
	if err != nil {
		return false, err
	}
	return rs.Satisfies(v), nil
}
