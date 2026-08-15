package semver

import (
	"errors"
	"testing"
)

func TestParseValid(t *testing.T) {
	cases := []struct {
		in   string
		maj  int
		min  int
		pat  int
		pre  []string
		blt  []string
	}{
		{"1.2.3", 1, 2, 3, nil, nil},
		{"0.0.0", 0, 0, 0, nil, nil},
		{"1.0.0", 1, 0, 0, nil, nil},
		{"1.2.3-alpha", 1, 2, 3, []string{"alpha"}, nil},
		{"1.2.3-alpha.1", 1, 2, 3, []string{"alpha", "1"}, nil},
		{"1.2.3-alpha.beta", 1, 2, 3, []string{"alpha", "beta"}, nil},
		{"1.2.3+build", 1, 2, 3, nil, []string{"build"}},
		{"1.2.3-alpha+build.123", 1, 2, 3, []string{"alpha"}, []string{"build", "123"}},
		{"1.0.0-0", 1, 0, 0, []string{"0"}, nil},
		{"1.0.0-x.7.z.92", 1, 0, 0, []string{"x", "7", "z", "92"}, nil},
		{"1.0.0+0", 1, 0, 0, nil, []string{"0"}}, // 构建段允许前导零/单零
		{"1.0.0+001", 1, 0, 0, nil, []string{"001"}},
	}
	for _, c := range cases {
		v, err := Parse(c.in)
		if err != nil {
			t.Errorf("Parse(%q) 出错: %v", c.in, err)
			continue
		}
		if v.Major != c.maj || v.Minor != c.min || v.Patch != c.pat {
			t.Errorf("Parse(%q) 核心段 = %d.%d.%d, 期望 %d.%d.%d", c.in, v.Major, v.Minor, v.Patch, c.maj, c.min, c.pat)
		}
		if !eqStrs(v.Prerelease, c.pre) {
			t.Errorf("Parse(%q) prerelease = %v, 期望 %v", c.in, v.Prerelease, c.pre)
		}
		if !eqStrs(v.Build, c.blt) {
			t.Errorf("Parse(%q) build = %v, 期望 %v", c.in, v.Build, c.blt)
		}
	}
}

func TestParseInvalid(t *testing.T) {
	cases := []string{
		"",          // 空版本
		"1",         // 段数不足
		"1.2",       // 段数不足
		"1.2.3.4",   // 段数超出
		"01.2.3",    // 主版本前导零
		"1.02.3",    // 次版本前导零
		"1.2.03",    // 修订号前导零
		"1.2.3-",    // 预发布分隔符但标识为空
		"1.2.3-.",   // 预发布空标识
		"1.2.3-alpha.", // 预发布末尾空标识
		"1.2.3-alpha.01", // 预发布数字标识前导零
		"1.2.3-01",  // 预发布数字标识前导零
		"1.2.3+",     // 构建分隔符但标识为空
		"1.2.3+build.", // 构建末尾空标识
		"v1.2.3",    // 不允许 v 前缀
		"1.2.3-beta_", // 非法字符
		"1.2.3 beta", // 含空格
		"1.2.-3",    // 修订号非法
		"1.2.3-α",   // 非 ASCII
		"1.2.3+",    // 重复，确保报错
	}
	for _, c := range cases {
		if _, err := Parse(c); err == nil {
			t.Errorf("Parse(%q) 期望错误，实际成功", c)
		}
	}
}

func eqStrs(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestCompare(t *testing.T) {
	// 覆盖 SemVer 2.0.0 规范给出的优先级链。
	chain := []string{
		"1.0.0-alpha",
		"1.0.0-beta",
		"1.0.0-rc.1",
		"1.0.0",
	}
	vers := make([]Version, len(chain))
	for i, s := range chain {
		v, err := Parse(s)
		if err != nil {
			t.Fatalf("Parse(%q) 出错: %v", s, err)
		}
		vers[i] = v
	}
	for i := 0; i < len(vers)-1; i++ {
		if c := Compare(vers[i], vers[i+1]); c >= 0 {
			t.Errorf("Compare(%q, %q) = %d, 期望 < 0", chain[i], chain[i+1], c)
		}
	}

	// 基础数值比较。
	pairs := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "2.0.0", -1},
		{"2.0.0", "2.1.0", -1},
		{"2.1.0", "2.1.1", -1},
		{"1.0.0", "1.0.0", 0},
		{"2.0.0", "1.0.0", 1},
		// 构建元数据完全被忽略。
		{"1.0.0+build1", "1.0.0+build2", 0},
		{"1.0.0+build", "1.0.0", 0},
		// 预发布低于正式版本。
		{"1.0.0-alpha", "1.0.0", -1},
		{"1.0.0", "1.0.0-alpha", 1},
		// 数字段按数值（非字典序）。
		{"1.0.0-beta.2", "1.0.0-beta.11", -1},
		// 数字段低于非数字段。
		{"1.0.0-alpha.1", "1.0.0-alpha.beta", -1},
	}
	for _, p := range pairs {
		a, _ := Parse(p.a)
		b, _ := Parse(p.b)
		if c := Compare(a, b); c != p.want {
			t.Errorf("Compare(%q, %q) = %d, 期望 %d", p.a, p.b, c, p.want)
		}
	}
}

func TestSatisfies(t *testing.T) {
	cases := []struct {
		name string
		ver  string
		rng  string
		want bool
	}{
		// 精确与比较操作符。
		{"exact-true", "1.2.3", "1.2.3", true},
		{"eq-true", "1.2.3", "=1.2.3", true},
		{"exact-false", "1.2.4", "1.2.3", false},
		{"ge-true", "1.2.3", ">=1.2.0", true},
		{"range-true", "1.2.9", ">=1.2.0 <2.0.0", true},
		{"range-false", "1.1.9", ">=1.2.0 <2.0.0", false},
		{"range-upper-false", "2.0.0", ">=1.2.0 <2.0.0", false},

		// 脱字符 ^：上界按最左非零位。
		{"caret-in", "1.5.0", "^1.2.3", true},
		{"caret-lower", "1.2.3", "^1.2.3", true},
		{"caret-upper", "2.0.0", "^1.2.3", false},
		{"caret-0minor-in", "0.2.5", "^0.2.3", true},
		{"caret-0minor-out", "0.3.0", "^0.2.3", false},
		{"caret-0patch-in", "0.0.3", "^0.0.3", true},

		// 波浪号 ~。
		{"tilde-full-in", "1.2.9", "~1.2.3", true},
		{"tilde-full-out", "1.3.0", "~1.2.3", false},
		{"tilde-minor-in", "1.0.5", "~1.0", true},
		{"tilde-minor-out", "1.1.0", "~1.0", false},
		{"tilde-major-in", "1.5.0", "~1", true},
		{"tilde-major-out", "2.0.0", "~1", false},

		// 通配符。
		{"wild-patch-in", "1.2.3", "1.2.x", true},
		{"wild-patch-out", "1.3.0", "1.2.x", false},
		{"wild-minor-in", "1.5.0", "1.x", true},
		{"wild-minor-out", "2.0.0", "1.x", false},
		{"star-any", "5.0.0", "*", true},
		{"star-low", "0.0.1", "*", true},

		// 边界约束 1：预发布版本默认被排除。
		{"pre-ge-excluded", "1.2.3-alpha", ">=1.0.0", false},
		{"pre-star-excluded", "1.2.3-alpha", "*", false},
		{"pre-same-allowed", "1.2.3-alpha", ">=1.2.3-alpha <2.0.0", true},
		{"pre-other-mmp-excluded", "1.2.4-alpha", ">=1.2.3-alpha <2.0.0", false},
		{"release-satisfies-pre-lower", "1.2.3", ">=1.2.3-alpha", true},
		{"pre-caret-excluded", "1.5.0-beta", "^1.2.3", false},

		// 边界约束 2：构建元数据在匹配中被忽略。
		{"build-ignored-1", "1.0.0+build1", "=1.0.0", true},
		{"build-ignored-2", "1.0.0+build2", "=1.0.0", true},
		{"build-ignored-star", "1.0.0+exp", "1.0.0", true},

		// 逻辑或。
		{"or-first", "1.2.3", "1.2.3 || 2.0.0", true},
		{"or-second", "2.0.0", "1.2.3 || 2.0.0", true},
		{"or-none", "1.5.0", "1.2.3 || 2.0.0", false},
		{"or-caret", "1.2.3", "^1.2.0 || ^2.0.0", true},
	}
	for _, c := range cases {
		v, err := Parse(c.ver)
		if err != nil {
			t.Errorf("%s: Parse(%q) 出错: %v", c.name, c.ver, err)
			continue
		}
		got, err := SatisfiesRange(v, c.rng)
		if err != nil {
			t.Errorf("%s: ParseRange(%q) 出错: %v", c.name, c.rng, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: Satisfies(%q, %q) = %v, 期望 %v", c.name, c.ver, c.rng, got, c.want)
		}
	}
}

func TestParseRangeInvalid(t *testing.T) {
	cases := []string{
		"",            // 空
		"  ",          // 仅空白
		"1.2.3 ||",    // 尾部空组
		"|| 1.2.3",    // 头部空组
		">=01.2.3",    // 范围项前导零
		">=1.2.3.4",   // 段数超出
		">=v1.2.3",    // v 前缀
		">*1.2.3",     // 非法
		">=1.2.3-",    // 预发布空
		"=1.2.3-alpha.01", // 预发布前导零
	}
	for _, c := range cases {
		if _, err := ParseRange(c); err == nil {
			t.Errorf("ParseRange(%q) 期望错误，实际成功", c)
		}
	}

	// ErrEmptyRange 应可被 errors.Is 识别。
	if _, err := ParseRange(""); !errors.Is(err, ErrEmptyRange) {
		t.Errorf("ParseRange(\"\") err = %v, 期望 ErrEmptyRange", err)
	}
}
