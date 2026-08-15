// Package httpapi 提供语义化版本服务的 HTTP 接口。
package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"task010-semver/internal/semver"
)

// ErrBadJSON 表示请求体不是合法的单个 JSON 对象。
var ErrBadJSON = errors.New("请求体不是合法的单个 JSON 对象")

// API 是无状态的语义化版本服务。
type API struct{}

// New 创建 API 实例。
func New() *API { return &API{} }

// Handler 返回 HTTP 路由。
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.health)
	mux.HandleFunc("POST /api/parse", a.parse)
	mux.HandleFunc("POST /api/compare", a.compare)
	mux.HandleFunc("POST /api/satisfies", a.satisfies)
	return mux
}

func decodeJSON(r *http.Request, v any) error {
	if r.Body == nil {
		return ErrBadJSON
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%w: %v", ErrBadJSON, err)
	}
	var extra any
	if dec.Decode(&extra) == nil {
		return ErrBadJSON
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError 将输入类错误统一以 400 返回，避免 panic 或 5xx。
func writeError(w http.ResponseWriter, err error) {
	writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error(), "status": http.StatusBadRequest})
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) parse(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Version string `json:"version"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if req.Version == "" {
		writeError(w, errors.New("version 字段不能为空"))
		return
	}
	v, err := semver.Parse(req.Version)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"version": versionView(v)})
}

func (a *API) compare(w http.ResponseWriter, r *http.Request) {
	var req struct {
		A string `json:"a"`
		B string `json:"b"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if req.A == "" || req.B == "" {
		writeError(w, errors.New("a 和 b 字段不能为空"))
		return
	}
	va, err := semver.Parse(req.A)
	if err != nil {
		writeError(w, err)
		return
	}
	vb, err := semver.Parse(req.B)
	if err != nil {
		writeError(w, err)
		return
	}
	res := semver.Compare(va, vb)
	word := "equal"
	switch res {
	case -1:
		word = "less"
	case 1:
		word = "greater"
	}
	writeJSON(w, http.StatusOK, map[string]any{"result": word})
}

func (a *API) satisfies(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Version string `json:"version"`
		Range   string `json:"range"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if req.Version == "" || req.Range == "" {
		writeError(w, errors.New("version 和 range 字段不能为空"))
		return
	}
	v, err := semver.Parse(req.Version)
	if err != nil {
		writeError(w, err)
		return
	}
	ok, err := semver.SatisfiesRange(v, req.Range)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"satisfied": ok})
}

// versionView 将 Version 转为 JSON 视图，prerelease/build 永远为数组。
func versionView(v semver.Version) map[string]any {
	return map[string]any{
		"major":      v.Major,
		"minor":      v.Minor,
		"patch":      v.Patch,
		"prerelease": v.Prerelease,
		"build":      v.Build,
	}
}
