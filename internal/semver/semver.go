// Package semver 实现语义化版本号（SemVer 2.0.0）的解析、比较与范围匹配。
//
// 仅依赖 Go 标准库。版本字符串遵循 MAJOR.MINOR.PATCH[-PRERELEASE][+BUILD]，
// 解析时严格校验前导零与合法字符，构建元数据在比较与范围匹配中被忽略。
package semver

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// 非法版本/范围相关哨兵错误。
var (
	ErrEmptyVersion = errors.New("版本号为空")
	ErrEmptyRange   = errors.New("范围表达式为空")
)

var (
	// coreSegRe 匹配非负整数且不含前导零（单独的 0 例外）。
	coreSegRe = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)
	// identRe 匹配预发布/构建标识段：字母数字与连字符，非空。
	identRe = regexp.MustCompile(`^[0-9A-Za-z-]+$`)
	// numericRe 判断标识段是否为纯数字。
	numericRe = regexp.MustCompile(`^[0-9]+$`)
)

// Version 表示一个语义化版本号。
type Version struct {
	Major      int      // 主版本号
	Minor      int      // 次版本号
	Patch      int      // 修订号
	Prerelease []string // 预发布标识段（点分段，原样保留，可能为空）
	Build      []string // 构建元数据段（点分段，原样保留，可能为空）
	Raw        string   // 原始输入字符串
}

// Parse 将版本字符串解析为 Version，非法输入返回错误且不做静默修正。
func Parse(s string) (Version, error) {
	if s == "" {
		return Version{}, ErrEmptyVersion
	}
	raw := s
	rest := s

	// 构建元数据以 "+" 引导，位于预发布之后；先剥离构建。
	var build []string
	if i := strings.Index(rest, "+"); i >= 0 {
		buildPart := rest[i+1:]
		rest = rest[:i]
		bs, err := parseBuildSegs(buildPart, raw)
		if err != nil {
			return Version{}, err
		}
		build = bs
	}

	// 预发布标识以 "-" 引导。
	var pre []string
	if i := strings.Index(rest, "-"); i >= 0 {
		prePart := rest[i+1:]
		rest = rest[:i]
		ps, err := parsePreSegs(prePart, raw)
		if err != nil {
			return Version{}, err
		}
		pre = ps
	}

	// 剩余部分必须为三段数值。
	core := strings.Split(rest, ".")
	if len(core) != 3 {
		return Version{}, fmt.Errorf("非法版本号 %q: 核心段数应为 3 段", raw)
	}
	major, err := parseCoreSeg(core[0], "主版本", raw)
	if err != nil {
		return Version{}, err
	}
	minor, err := parseCoreSeg(core[1], "次版本", raw)
	if err != nil {
		return Version{}, err
	}
	patch, err := parseCoreSeg(core[2], "修订号", raw)
	if err != nil {
		return Version{}, err
	}
	return Version{Major: major, Minor: minor, Patch: patch, Prerelease: pre, Build: build, Raw: raw}, nil
}

// parseCoreSeg 解析主/次/修订号：非负整数，不允许前导零。
func parseCoreSeg(seg, name, raw string) (int, error) {
	if !coreSegRe.MatchString(seg) {
		return 0, fmt.Errorf("非法版本号 %q: %s段非法 %q", raw, name, seg)
	}
	n, err := strconv.Atoi(seg)
	if err != nil {
		return 0, fmt.Errorf("非法版本号 %q: %s段越界 %q", raw, name, seg)
	}
	return n, nil
}

// parsePreSegs 解析预发布标识段：每段为字母数字与连字符；数字段不允许前导零。
func parsePreSegs(s, raw string) ([]string, error) {
	segs := strings.Split(s, ".")
	out := make([]string, 0, len(segs))
	for _, seg := range segs {
		if !identRe.MatchString(seg) {
			return nil, fmt.Errorf("非法版本号 %q: 预发布标识段非法 %q", raw, seg)
		}
		if numericRe.MatchString(seg) && !coreSegRe.MatchString(seg) {
			return nil, fmt.Errorf("非法版本号 %q: 预发布数字标识含前导零 %q", raw, seg)
		}
		out = append(out, seg)
	}
	return out, nil
}

// parseBuildSegs 解析构建元数据段：每段为字母数字与连字符；允许前导零。
func parseBuildSegs(s, raw string) ([]string, error) {
	segs := strings.Split(s, ".")
	out := make([]string, 0, len(segs))
	for _, seg := range segs {
		if !identRe.MatchString(seg) {
			return nil, fmt.Errorf("非法版本号 %q: 构建标识段非法 %q", raw, seg)
		}
		out = append(out, seg)
	}
	return out, nil
}

// Compare 按优先级比较两个版本号，返回 -1（小于）、0（等于）或 1（大于）。
// 构建元数据不参与比较。
func Compare(a, b Version) int {
	if a.Major != b.Major {
		return cmpInt(a.Major, b.Major)
	}
	if a.Minor != b.Minor {
		return cmpInt(a.Minor, b.Minor)
	}
	if a.Patch != b.Patch {
		return cmpInt(a.Patch, b.Patch)
	}
	aPre := len(a.Prerelease) > 0
	bPre := len(b.Prerelease) > 0
	if !aPre && !bPre {
		return 0
	}
	if !aPre {
		return 1 // 无预发布者优先级更高
	}
	if !bPre {
		return -1
	}
	return comparePrerelease(a.Prerelease, b.Prerelease)
}

func cmpInt(a, b int) int {
	if a < b {
		return -1
	}
	if a > b {
		return 1
	}
	return 0
}

// comparePrerelease 按段比较预发布标识：数字段按数值，非数字段按 ASCII，
// 数字段低于任意非数字段；公共前缀相同时段数更多者优先级更高。
func comparePrerelease(a, b []string) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		sa, sb := a[i], b[i]
		aNum := numericRe.MatchString(sa)
		bNum := numericRe.MatchString(sb)
		switch {
		case aNum && bNum:
			if c := compareNumeric(sa, sb); c != 0 {
				return c
			}
		case aNum:
			return -1
		case bNum:
			return 1
		default:
			if c := strings.Compare(sa, sb); c != 0 {
				return c
			}
		}
	}
	return cmpInt(len(b), len(a))
}

// compareNumeric 比较两个无前导零的非负整数字符串：先比长度，再按字典序。
func compareNumeric(a, b string) int {
	if len(a) != len(b) {
		return cmpInt(len(a), len(b))
	}
	return strings.Compare(a, b)
}

// String 将 Version 还原为规范字符串。
func (v Version) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d.%d.%d", v.Major, v.Minor, v.Patch)
	if len(v.Prerelease) > 0 {
		b.WriteByte('-')
		b.WriteString(strings.Join(v.Prerelease, "."))
	}
	if len(v.Build) > 0 {
		b.WriteByte('+')
		b.WriteString(strings.Join(v.Build, "."))
	}
	return b.String()
}
