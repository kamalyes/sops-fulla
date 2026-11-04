/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-03 22:05:33
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-03 22:37:52
 * @FilePath: \sops-fulla\server\server_test.go
 * @Description: 管理页面HTTP层测试：令牌校验、跨站防护与保存往返
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"

	"github.com/kamalyes/sops-fulla/crypto"
	"github.com/kamalyes/sops-fulla/service"
)

// newTestServer 建一个监听随机端口的测试服务，返回其基础地址
func newTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	root := t.TempDir()
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	if err := writeTestFile(root+"/.sops.yaml", "creation_rules:\n"+
		"  - path_regex: \\.enc\\.(yaml|yml|json)$\n"+
		"    age: "+identity.Recipient().String()+"\n"); err != nil {
		t.Fatal(err)
	}
	initial, err := crypto.Encrypt(root, []byte("HOST: example.com\nTOKEN: abc123\n"), "yaml", "demo/dev/secrets.enc.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeTestFile(root+"/demo/dev/secrets.enc.yaml", string(initial)); err != nil {
		t.Fatal(err)
	}
	svc := service.NewIsolated(root)
	if _, err := svc.SetPageKey(identity.String()); err != nil {
		t.Fatal(err)
	}
	srv := New(svc)
	if err := srv.Start("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	return srv, fmt.Sprintf("http://127.0.0.1:%d", srv.Port())
}

// writeTestFile 写文件并创建父目录
func writeTestFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o600)
}

// request 发一个带令牌的请求，返回状态码与响应体
func request(t *testing.T, method, url, token, contentType string, body []byte) (int, string) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("X-Config-Token", token)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(response.Body)
	return response.StatusCode, string(data)
}

func TestIndexInjectsSessionToken(t *testing.T) {
	srv, base := newTestServer(t)
	status, body := request(t, http.MethodGet, base+"/", srv.Token(), "", nil)
	if status != http.StatusOK {
		t.Fatalf("首页应返回 200，实际 %d", status)
	}
	if !strings.Contains(body, srv.Token()) {
		t.Fatal("首页应注入本次会话的访问令牌")
	}
}

func TestAPITokenEnforced(t *testing.T) {
	srv, base := newTestServer(t)
	if status, _ := request(t, http.MethodGet, base+"/api/catalog", "", "", nil); status != http.StatusForbidden {
		t.Fatalf("缺令牌应 403，实际 %d", status)
	}
	if status, _ := request(t, http.MethodGet, base+"/api/catalog", "wrong-token", "", nil); status != http.StatusForbidden {
		t.Fatalf("错令牌应 403，实际 %d", status)
	}
	if status, body := request(t, http.MethodGet, base+"/api/catalog", srv.Token(), "", nil); status != http.StatusOK {
		t.Fatalf("带令牌应 200，实际 %d：%s", status, body)
	}
}

func TestForeignOriginRejected(t *testing.T) {
	srv, base := newTestServer(t)
	req, _ := http.NewRequest(http.MethodGet, base+"/api/catalog", nil)
	req.Header.Set("X-Config-Token", srv.Token())
	req.Header.Set("Origin", "http://evil.example")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("外部 Origin 应 403，实际 %d", response.StatusCode)
	}
}

func TestSaveRoundTripOverHTTP(t *testing.T) {
	srv, base := newTestServer(t)
	status, body := request(t, http.MethodGet, base+"/api/file?service=demo&environment=dev&filename=secrets.enc.yaml", srv.Token(), "", nil)
	if status != http.StatusOK {
		t.Fatalf("读取文件应 200，实际 %d：%s", status, body)
	}
	var data struct {
		Revision *string           `json:"revision"`
		Kind     string            `json:"kind"`
		Values   map[string]string `json:"values"`
	}
	if err := json.Unmarshal([]byte(body), &data); err != nil {
		t.Fatal(err)
	}
	if data.Revision == nil || data.Values["HOST"] != "example.com" {
		t.Fatalf("文件数据不对：%s", body)
	}

	payload, _ := json.Marshal(map[string]any{
		"service":     "demo",
		"environment": "dev",
		"filename":    "secrets.enc.yaml",
		"revision":    *data.Revision,
		"values":      map[string]string{"HOST": "changed.example.com", "TOKEN": "abc123"},
	})
	status, body = request(t, http.MethodPost, base+"/api/save", srv.Token(), "application/json", payload)
	if status != http.StatusOK {
		t.Fatalf("保存应 200，实际 %d：%s", status, body)
	}
	status, body = request(t, http.MethodGet, base+"/api/file?service=demo&environment=dev&filename=secrets.enc.yaml", srv.Token(), "", nil)
	if !strings.Contains(body, "changed.example.com") {
		t.Fatalf("保存后应读到新值：%s", body)
	}
}

func TestSaveRequiresJSONContentType(t *testing.T) {
	srv, base := newTestServer(t)
	status, _ := request(t, http.MethodPost, base+"/api/save", srv.Token(), "text/plain", []byte("{}"))
	if status != http.StatusUnsupportedMediaType {
		t.Fatalf("非 JSON 请求应 415，实际 %d", status)
	}
}

func TestKeyPairGeneratesFreshAgeKey(t *testing.T) {
	srv, base := newTestServer(t)
	status, body := request(t, http.MethodPost, base+"/api/keypair", srv.Token(), "", nil)
	if status != http.StatusOK {
		t.Fatalf("生成密钥对应 200，实际 %d：%s", status, body)
	}
	var pair struct {
		Recipient string `json:"recipient"`
		Identity  string `json:"identity"`
	}
	if err := json.Unmarshal([]byte(body), &pair); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(pair.Recipient, "age1") || !strings.HasPrefix(pair.Identity, "AGE-SECRET-KEY-1") {
		t.Fatalf("应返回 age 公私钥对，实际 %+v", pair)
	}
}

func TestRotateOverHTTP(t *testing.T) {
	srv, base := newTestServer(t)
	// 先生成新密钥对
	status, body := request(t, http.MethodPost, base+"/api/keypair", srv.Token(), "", nil)
	if status != http.StatusOK {
		t.Fatalf("生成密钥对应 200，实际 %d", status)
	}
	var pair struct {
		Recipient string `json:"recipient"`
	}
	if err := json.Unmarshal([]byte(body), &pair); err != nil {
		t.Fatal(err)
	}
	// 空公钥应 400
	if status, _ := request(t, http.MethodPost, base+"/api/rotate", srv.Token(), "application/json", []byte(`{"recipient":""}`)); status != http.StatusBadRequest {
		t.Fatalf("空公钥应 400，实际 %d", status)
	}
	// 轮换到新公钥
	status, body = request(t, http.MethodPost, base+"/api/rotate", srv.Token(), "application/json", []byte(`{"recipient":"`+pair.Recipient+`"}`))
	if status != http.StatusOK {
		t.Fatalf("轮换应 200，实际 %d：%s", status, body)
	}
	var result struct {
		Files        []string `json:"files"`
		RulesUpdated bool     `json:"rules_updated"`
	}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 || !result.RulesUpdated {
		t.Fatalf("应轮换一份并更新规则，实际 %+v", result)
	}
}

func TestUnknownPathReturns404(t *testing.T) {
	srv, base := newTestServer(t)
	if status, _ := request(t, http.MethodGet, base+"/nope", srv.Token(), "", nil); status != http.StatusNotFound {
		t.Fatalf("未知页面应 404，实际 %d", status)
	}
	if status, _ := request(t, http.MethodPost, base+"/api/nope", srv.Token(), "application/json", []byte("{}")); status != http.StatusNotFound {
		t.Fatalf("未知接口应 404，实际 %d", status)
	}
}

// newEmptyWorkspaceServer 建一个空工作区的测试服务（免仓库启动的形态）
func newEmptyWorkspaceServer(t *testing.T) (*Server, string) {
	t.Helper()
	t.Setenv("FULLA_CONFIG_DIR", t.TempDir())
	srv := New(service.NewIsolated(""))
	if err := srv.Start("127.0.0.1:0"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { srv.Close() })
	return srv, fmt.Sprintf("http://127.0.0.1:%d", srv.Port())
}

// workspaceBody 构造 /api/workspace 的请求体（json.Marshal 处理路径转义）
func workspaceBody(path string) []byte {
	data, _ := json.Marshal(map[string]string{"path": path})
	return data
}

// TestWorkspaceSwitchOverHTTP 验证空工作区启动后通过接口选择并切换仓库
func TestWorkspaceSwitchOverHTTP(t *testing.T) {
	srv, base := newEmptyWorkspaceServer(t)

	// 空工作区：目录为空、能力标记无工作区
	status, body := request(t, http.MethodGet, base+"/api/catalog", srv.Token(), "", nil)
	if status != http.StatusOK {
		t.Fatalf("空工作区目录应 200，实际 %d", status)
	}
	var catalog struct {
		Services     []map[string]any `json:"services"`
		HasWorkspace bool             `json:"has_workspace"`
	}
	if err := json.Unmarshal([]byte(body), &catalog); err != nil {
		t.Fatal(err)
	}
	if catalog.HasWorkspace || len(catalog.Services) != 0 {
		t.Fatalf("应是无工作区的空目录，实际 %s", body)
	}

	// 最近列表初始为空
	status, body = request(t, http.MethodGet, base+"/api/workspaces", srv.Token(), "", nil)
	if status != http.StatusOK || !strings.Contains(body, `"recent":[]`) {
		t.Fatalf("最近列表应为空，实际 %d：%s", status, body)
	}

	// 切换到缺 .sops.yaml 的目录应 400
	bad := t.TempDir()
	if status, _ := request(t, http.MethodPost, base+"/api/workspace", srv.Token(), "application/json", workspaceBody(bad)); status != http.StatusBadRequest {
		t.Fatalf("无效目录应 400，实际 %d", status)
	}

	// 切换到有效仓库：返回新目录
	root := t.TempDir()
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	if err := writeTestFile(filepath.Join(root, ".sops.yaml"), "creation_rules:\n"+
		"  - path_regex: \\.enc\\.(yaml|yml|json)$\n"+
		"    age: "+identity.Recipient().String()+"\n"); err != nil {
		t.Fatal(err)
	}
	if err := writeTestFile(filepath.Join(root, "demo", "dev", "config.dev.yaml"), "Name: demo\n"); err != nil {
		t.Fatal(err)
	}
	status, body = request(t, http.MethodPost, base+"/api/workspace", srv.Token(), "application/json", workspaceBody(root))
	if status != http.StatusOK {
		t.Fatalf("切换工作区应 200，实际 %d：%s", status, body)
	}
	if !strings.Contains(body, `"has_workspace":true`) || !strings.Contains(body, `"repository":"`+filepath.Base(root)+`"`) {
		t.Fatalf("切换后应返回带新仓库的目录，实际 %s", body)
	}

	// 最近列表记录了新工作区
	status, body = request(t, http.MethodGet, base+"/api/workspaces", srv.Token(), "", nil)
	if status != http.StatusOK {
		t.Fatalf("最近列表应 200，实际 %d", status)
	}
	var info struct {
		Current string   `json:"current"`
		Recent  []string `json:"recent"`
	}
	if err := json.Unmarshal([]byte(body), &info); err != nil {
		t.Fatal(err)
	}
	if filepath.Base(info.Current) != filepath.Base(root) ||
		len(info.Recent) != 1 || filepath.Base(info.Recent[0]) != filepath.Base(root) {
		t.Fatalf("最近列表应记录新工作区，实际 %+v", info)
	}
}

// TestRememberKeyOverHTTP 验证 /api/key 的记住与清除私钥扩展
func TestRememberKeyOverHTTP(t *testing.T) {
	srv, base := newEmptyWorkspaceServer(t)
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}

	// 记住私钥：来源标记 remembered
	status, body := request(t, http.MethodPost, base+"/api/key", srv.Token(), "application/json",
		[]byte(`{"secret":"`+identity.String()+`","remember":true}`))
	if status != http.StatusOK || !strings.Contains(body, `"key_source":"page"`) {
		t.Fatalf("记住后页面私钥仍应优先（page），实际 %d：%s", status, body)
	}

	// 清除页面私钥后回落到 remembered
	status, body = request(t, http.MethodPost, base+"/api/key", srv.Token(), "application/json", []byte(`{"secret":""}`))
	if status != http.StatusOK || !strings.Contains(body, `"key_source":"remembered"`) {
		t.Fatalf("清除页面私钥后应回落到 remembered，实际 %d：%s", status, body)
	}

	// 无效私钥应 400
	if status, _ := request(t, http.MethodPost, base+"/api/key", srv.Token(), "application/json", []byte(`{"secret":"oops","remember":true}`)); status != http.StatusBadRequest {
		t.Fatalf("无效私钥应 400，实际 %d", status)
	}

	// 清除记住的私钥：来源不再来自 remembered
	status, body = request(t, http.MethodPost, base+"/api/key", srv.Token(), "application/json", []byte(`{"forget_remembered":true}`))
	if status != http.StatusOK || strings.Contains(body, `"key_source":"remembered"`) {
		t.Fatalf("清除后不应再是 remembered，实际 %d：%s", status, body)
	}
}
