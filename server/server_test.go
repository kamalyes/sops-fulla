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
