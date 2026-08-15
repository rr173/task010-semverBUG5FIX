// Package selfcheck 提供无需外部依赖、执行后自行退出的 --smoke-test。
package selfcheck

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"

	"task010-semver/internal/httpapi"
)

// Run 执行内置自测，返回退出码：0 表示全部通过，1 表示存在失败。
func Run() int {
	passed, failed := 0, 0
	check := func(name string, fn func() error) {
		if err := fn(); err != nil {
			failed++
			fmt.Printf("FAIL %-36s %v\n", name, err)
		} else {
			passed++
			fmt.Printf("PASS %s\n", name)
		}
	}

	api := httpapi.New()
	srv := httptest.NewServer(api.Handler())
	defer srv.Close()

	do := func(method, path, body string) (*http.Response, []byte, error) {
		var r io.Reader
		if body != "" {
			r = bytes.NewReader([]byte(body))
		}
		req, err := http.NewRequest(method, srv.URL+path, r)
		if err != nil {
			return nil, nil, err
		}
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, nil, err
		}
		data, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		return resp, data, readErr
	}
	post := func(path, body string) (*http.Response, []byte, error) {
		return do(http.MethodPost, path, body)
	}

	check("健康检查", func() error {
		resp, _, err := do(http.MethodGet, "/healthz", "")
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status=%d", resp.StatusCode)
		}
		return nil
	})

	check("解析合法版本返回分段", func() error {
		resp, body, err := post("/api/parse", `{"version":"1.2.3-alpha.1+build.7"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status=%d body=%s", resp.StatusCode, body)
		}
		var out struct {
			Version struct {
				Major      int      `json:"major"`
				Minor      int      `json:"minor"`
				Patch      int      `json:"patch"`
				Prerelease []string `json:"prerelease"`
				Build      []string `json:"build"`
			} `json:"version"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			return err
		}
		v := out.Version
		if v.Major != 1 || v.Minor != 2 || v.Patch != 3 {
			return fmt.Errorf("核心段=%d.%d.%d", v.Major, v.Minor, v.Patch)
		}
		if len(v.Prerelease) != 2 || v.Prerelease[0] != "alpha" || v.Prerelease[1] != "1" {
			return fmt.Errorf("prerelease=%v", v.Prerelease)
		}
		if len(v.Build) != 2 || v.Build[0] != "build" || v.Build[1] != "7" {
			return fmt.Errorf("build=%v", v.Build)
		}
		return nil
	})

	check("解析普通版本无预发布/构建返回空数组", func() error {
		resp, body, err := post("/api/parse", `{"version":"1.0.0"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status=%d body=%s", resp.StatusCode, body)
		}
		var out struct {
			Version struct {
				Major int `json:"major"`
				Minor int `json:"minor"`
				Patch int `json:"patch"`
			} `json:"version"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			return err
		}
		if out.Version.Major != 1 || out.Version.Minor != 0 || out.Version.Patch != 0 {
			return fmt.Errorf("核心段=%d.%d.%d", out.Version.Major, out.Version.Minor, out.Version.Patch)
		}
		return nil
	})

	check("前导零版本被拒绝", func() error {
		resp, _, err := post("/api/parse", `{"version":"1.02.3"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusBadRequest {
			return fmt.Errorf("status=%d", resp.StatusCode)
		}
		return nil
	})

	check("空版本号被拒绝", func() error {
		resp, _, err := post("/api/parse", `{"version":""}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusBadRequest {
			return fmt.Errorf("status=%d", resp.StatusCode)
		}
		return nil
	})

	check("比较返回 less", func() error {
		resp, body, err := post("/api/compare", `{"a":"1.2.3","b":"2.0.0"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status=%d body=%s", resp.StatusCode, body)
		}
		var out struct {
			Result string `json:"result"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			return err
		}
		if out.Result != "less" {
			return fmt.Errorf("result=%s", out.Result)
		}
		return nil
	})

	check("比较预发布链返回 greater", func() error {
		resp, body, err := post("/api/compare", `{"a":"1.0.0","b":"1.0.0-rc.1"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status=%d body=%s", resp.StatusCode, body)
		}
		var out struct {
			Result string `json:"result"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			return err
		}
		if out.Result != "greater" {
			return fmt.Errorf("result=%s", out.Result)
		}
		return nil
	})

	check("构建元数据不影响比较", func() error {
		resp, body, err := post("/api/compare", `{"a":"1.0.0+build1","b":"1.0.0+build2"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status=%d body=%s", resp.StatusCode, body)
		}
		var out struct {
			Result string `json:"result"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			return err
		}
		if out.Result != "equal" {
			return fmt.Errorf("result=%s", out.Result)
		}
		return nil
	})

	check("比较非法版本被拒绝", func() error {
		resp, _, err := post("/api/compare", `{"a":"1.2.3","b":"not-a-version"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusBadRequest {
			return fmt.Errorf("status=%d", resp.StatusCode)
		}
		return nil
	})

	check("范围满足返回 true", func() error {
		resp, body, err := post("/api/satisfies", `{"version":"1.5.0","range":"^1.2.3"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status=%d body=%s", resp.StatusCode, body)
		}
		var out struct {
			Satisfied bool `json:"satisfied"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			return err
		}
		if !out.Satisfied {
			return fmt.Errorf("satisfied=false")
		}
		return nil
	})

	check("脱字符上界排除返回 false", func() error {
		resp, body, err := post("/api/satisfies", `{"version":"2.0.0","range":"^1.2.3"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status=%d body=%s", resp.StatusCode, body)
		}
		var out struct {
			Satisfied bool `json:"satisfied"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			return err
		}
		if out.Satisfied {
			return fmt.Errorf("satisfied=true")
		}
		return nil
	})

	check("预发布版本默认被排除", func() error {
		resp, body, err := post("/api/satisfies", `{"version":"1.2.3-alpha","range":">=1.0.0"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status=%d body=%s", resp.StatusCode, body)
		}
		var out struct {
			Satisfied bool `json:"satisfied"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			return err
		}
		if out.Satisfied {
			return fmt.Errorf("satisfied=true 预发布不应满足")
		}
		return nil
	})

	check("同主次修订预发布被允许", func() error {
		resp, body, err := post("/api/satisfies", `{"version":"1.2.3-alpha","range":">=1.2.3-alpha <2.0.0"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status=%d body=%s", resp.StatusCode, body)
		}
		var out struct {
			Satisfied bool `json:"satisfied"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			return err
		}
		if !out.Satisfied {
			return fmt.Errorf("satisfied=false 同主次修订预发布应被允许")
		}
		return nil
	})

	check("异主次修订预发布被排除", func() error {
		resp, body, err := post("/api/satisfies", `{"version":"1.2.4-alpha","range":">=1.2.3-alpha <2.0.0"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status=%d body=%s", resp.StatusCode, body)
		}
		var out struct {
			Satisfied bool `json:"satisfied"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			return err
		}
		if out.Satisfied {
			return fmt.Errorf("satisfied=true 异主次修订预发布应被排除")
		}
		return nil
	})

	check("构建元数据在匹配中被忽略", func() error {
		resp, body, err := post("/api/satisfies", `{"version":"1.0.0+build1","range":"=1.0.0"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status=%d body=%s", resp.StatusCode, body)
		}
		var out struct {
			Satisfied bool `json:"satisfied"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			return err
		}
		if !out.Satisfied {
			return fmt.Errorf("satisfied=false 构建元数据应被忽略")
		}
		return nil
	})

	check("通配符范围", func() error {
		resp, body, err := post("/api/satisfies", `{"version":"1.2.9","range":"1.2.x"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status=%d body=%s", resp.StatusCode, body)
		}
		var out struct {
			Satisfied bool `json:"satisfied"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			return err
		}
		if !out.Satisfied {
			return fmt.Errorf("satisfied=false")
		}
		return nil
	})

	check("或范围命中第二组", func() error {
		resp, body, err := post("/api/satisfies", `{"version":"2.0.0","range":"1.2.3 || 2.0.0"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status=%d body=%s", resp.StatusCode, body)
		}
		var out struct {
			Satisfied bool `json:"satisfied"`
		}
		if err := json.Unmarshal(body, &out); err != nil {
			return err
		}
		if !out.Satisfied {
			return fmt.Errorf("satisfied=false")
		}
		return nil
	})

	check("非法范围被拒绝", func() error {
		resp, _, err := post("/api/satisfies", `{"version":"1.2.3","range":">=01.2.3"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusBadRequest {
			return fmt.Errorf("status=%d", resp.StatusCode)
		}
		return nil
	})

	check("非法 JSON 被拒绝", func() error {
		resp, _, err := post("/api/parse", `{not json}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusBadRequest {
			return fmt.Errorf("status=%d", resp.StatusCode)
		}
		return nil
	})

	check("多段 JSON 被拒绝", func() error {
		resp, _, err := post("/api/parse", `{"version":"1.2.3"}{"version":"1.0.0"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusBadRequest {
			return fmt.Errorf("status=%d", resp.StatusCode)
		}
		return nil
	})

	check("未知字段被拒绝", func() error {
		resp, _, err := post("/api/parse", `{"version":"1.2.3","extra":1}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusBadRequest {
			return fmt.Errorf("status=%d", resp.StatusCode)
		}
		return nil
	})

	check("缺失字段被拒绝", func() error {
		resp, _, err := post("/api/compare", `{"a":"1.2.3"}`)
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusBadRequest {
			return fmt.Errorf("status=%d", resp.StatusCode)
		}
		return nil
	})

	fmt.Printf("\n%d passed, %d failed\n", passed, failed)
	if failed > 0 {
		return 1
	}
	return 0
}
