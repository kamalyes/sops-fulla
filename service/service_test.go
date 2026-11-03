/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-03 19:20:52
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-03 19:56:30
 * @FilePath: \sops-fulla\service\service_test.go
 * @Description: 服务层端到端测试：加解密、最小diff保存与任意位置放置
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package service_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"

	"github.com/kamalyes/sops-fulla/batch"
	"github.com/kamalyes/sops-fulla/crypto"
	"github.com/kamalyes/sops-fulla/errors"
	"github.com/kamalyes/sops-fulla/models"
	"github.com/kamalyes/sops-fulla/service"
)

// newFixture 建一个带 age 密钥对的仓库：demo/dev 下一份密钥、一份明文配置
// 返回服务实例和私钥文本（已通过 SetPageKey 注入）
func newFixture(t *testing.T) (*service.Service, string) {
	t.Helper()
	root := t.TempDir()
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, ".sops.yaml"), "creation_rules:\n"+
		"  - path_regex: \\.enc\\.(yaml|yml|json)$\n"+
		"    age: "+identity.Recipient().String()+"\n")
	initial, err := crypto.Encrypt(root, []byte("HOST: example.com\nTOKEN: abc123\n"), "yaml", "demo/dev/secrets.enc.yaml")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "demo", "dev", "secrets.enc.yaml"), string(initial))
	writeFile(t, filepath.Join(root, "demo", "dev", "config.dev.yaml"), "Name: demo\n")

	svc := service.NewIsolated(root)
	if _, err := svc.SetPageKey(identity.String()); err != nil {
		t.Fatal(err)
	}
	return svc, identity.String()
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustFail(t *testing.T, err error, wantStatus int, wantPart string) {
	t.Helper()
	if err == nil {
		t.Fatalf("期望失败，实际成功")
	}
	business, ok := errors.As(err)
	if !ok {
		t.Fatalf("期望业务错误，实际 %T：%v", err, err)
	}
	if business.Status != wantStatus {
		t.Fatalf("期望状态码 %d，实际 %d（%s）", wantStatus, business.Status, business.Message)
	}
	if wantPart != "" && !strings.Contains(business.Message, wantPart) {
		t.Fatalf("报错 %q 不包含 %q", business.Message, wantPart)
	}
}

// mapping 构造一个扁平的密钥值树
func mapping(pairs map[string]string) *models.Value {
	value := &models.Value{Kind: models.KindMapping, Tag: "!!map"}
	for key, raw := range pairs {
		value.Map = append(value.Map, models.Entry{Key: key, Value: &models.Value{Kind: models.KindString, Tag: "!!str", Raw: raw}})
	}
	return value
}

// encLine 找出密文文件里某个键那一行（含 ENC[...]）
func encLine(content, key string) string {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, key+":") && strings.Contains(line, "ENC[") {
			return line
		}
	}
	return ""
}

func TestCatalogListsServices(t *testing.T) {
	svc, _ := newFixture(t)
	catalog := svc.Catalog()
	if len(catalog.Services) != 1 || catalog.Services[0].Name != "demo" {
		t.Fatalf("目录里应只有 demo 一个服务，实际 %+v", catalog.Services)
	}
	environments := catalog.Services[0].Environments
	if len(environments) != 1 || environments[0].Name != "dev" {
		t.Fatalf("demo 下应只有 dev 一个环境，实际 %+v", environments)
	}
	names := map[string]bool{}
	for _, file := range environments[0].Files {
		names[file.Name] = true
	}
	if !names["secrets.enc.yaml"] || !names["config.dev.yaml"] {
		t.Fatalf("dev 目录应包含两份文件，实际 %+v", environments[0].Files)
	}
	if !catalog.Capabilities.SecretsReady || catalog.Capabilities.KeySource != "page" {
		t.Fatalf("能力信息应显示私钥已就绪（page），实际 %+v", catalog.Capabilities)
	}
}

func TestCatalogAddsVirtualSecretsEntry(t *testing.T) {
	svc, _ := newFixture(t)
	writeFile(t, filepath.Join(svc.Root(), "demo", "prod", "app.prod.yaml"), "Name: demo\n")
	for _, environment := range svc.Catalog().Services[0].Environments {
		var hasSecrets bool
		for _, file := range environment.Files {
			if file.Kind == "secrets" {
				hasSecrets = true
				if environment.Name == "prod" && file.Exists {
					t.Fatalf("prod 的密钥条目应是虚拟的「待添加」")
				}
			}
		}
		if !hasSecrets {
			t.Fatalf("环境 %s 缺少密钥条目", environment.Name)
		}
	}
}

func TestReadDecryptsSecrets(t *testing.T) {
	svc, _ := newFixture(t)
	data, err := svc.Read("demo", "dev", "secrets.enc.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if data.Revision == nil || data.Kind != "secrets" || data.Values == nil {
		t.Fatalf("密钥文件形状不对：%+v", data)
	}
	pairs := map[string]string{}
	for _, entry := range data.Values.Map {
		pairs[entry.Key] = entry.Value.Raw
	}
	if pairs["HOST"] != "example.com" || pairs["TOKEN"] != "abc123" {
		t.Fatalf("解密结果不对：%+v", pairs)
	}
}

func TestSaveRejectsStaleRevision(t *testing.T) {
	svc, _ := newFixture(t)
	stale := "0000"
	_, err := svc.Save(&models.SaveRequest{
		Service: "demo", Environment: "dev", Filename: "secrets.enc.yaml",
		Revision: &stale, Values: mapping(map[string]string{"HOST": "h"}),
	})
	mustFail(t, err, 409, "文件已被其他人修改")
}

func TestSaveChangesOnlyTouchedCiphertext(t *testing.T) {
	svc, _ := newFixture(t)
	path := filepath.Join(svc.Root(), "demo", "dev", "secrets.enc.yaml")
	before, _ := os.ReadFile(path)

	data, err := svc.Read("demo", "dev", "secrets.enc.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Save(&models.SaveRequest{
		Service: "demo", Environment: "dev", Filename: "secrets.enc.yaml",
		Revision: data.Revision,
		Values:   mapping(map[string]string{"HOST": "changed.example.com", "TOKEN": "abc123"}),
	}); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)

	if line, same := encLine(string(before), "TOKEN"), encLine(string(after), "TOKEN"); line == "" || line != same {
		t.Fatalf("没改的 TOKEN 密文应逐字节不变：\n旧 %s\n新 %s", line, same)
	}
	if before, after := encLine(string(before), "HOST"), encLine(string(after), "HOST"); before == after {
		t.Fatal("改过的 HOST 密文不应与旧密文相同")
	}
	if bytes.Contains(after, []byte("changed.example.com")) {
		t.Fatal("明文不应出现在密文文件里")
	}
}

func TestCommentCiphertextStableAcrossSave(t *testing.T) {
	svc, _ := newFixture(t)
	plain := "# 服务的名字\nName: demo\n# 监听端口\nPort: 8080\n"
	path := filepath.Join(svc.Root(), "demo", "dev", "config.dev.enc.yaml")
	initial, err := crypto.Encrypt(svc.Root(), []byte(plain), "yaml", "demo/dev/config.dev.enc.yaml")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, string(initial))

	data, err := svc.Read("demo", "dev", "config.dev.enc.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if data.Content == nil || *data.Content != plain {
		t.Fatalf("解密回来的内容应与原文一致：%q", *data.Content)
	}
	edited := "# 服务的名字\nName: demo\n# 监听端口\nPort: 9090\n"
	if _, err := svc.Save(&models.SaveRequest{
		Service: "demo", Environment: "dev", Filename: "config.dev.enc.yaml",
		Revision: data.Revision, Content: &edited,
	}); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(path)

	// 注释没改：密文行应逐字节沿用；Name 没改：同样不变；只有 Port 的密文变
	oldText, newText := string(initial), string(after)
	for _, want := range []string{"type:comment"} {
		oldLines := filterLines(oldText, want)
		newLines := filterLines(newText, want)
		if len(oldLines) != 2 || len(newLines) != 2 {
			t.Fatalf("注释密文行数不对：旧 %d 新 %d", len(oldLines), len(newLines))
		}
		for i := range oldLines {
			if oldLines[i] != newLines[i] {
				t.Fatalf("未修改的注释密文应不变：\n旧 %s\n新 %s", oldLines[i], newLines[i])
			}
		}
	}
	if before, after := encLine(oldText, "Name"), encLine(newText, "Name"); before == "" || before != after {
		t.Fatalf("未修改的 Name 密文应不变：\n旧 %s\n新 %s", before, after)
	}
	if before, after := encLine(oldText, "Port"), encLine(newText, "Port"); before == after {
		t.Fatal("修改过的 Port 密文应变化")
	}
}

func filterLines(content, part string) []string {
	var matched []string
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, part) && strings.Contains(line, "ENC[") {
			matched = append(matched, line)
		}
	}
	return matched
}

func TestSavePlainConfigRoundTrip(t *testing.T) {
	svc, _ := newFixture(t)
	data, err := svc.Read("demo", "dev", "config.dev.yaml")
	if err != nil {
		t.Fatal(err)
	}
	edited := "Name: demo\nReplicas: 3\n"
	if _, err := svc.Save(&models.SaveRequest{
		Service: "demo", Environment: "dev", Filename: "config.dev.yaml",
		Revision: data.Revision, Content: &edited,
	}); err != nil {
		t.Fatal(err)
	}
	again, err := svc.Read("demo", "dev", "config.dev.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if again.Content == nil || *again.Content != edited {
		t.Fatalf("明文配置应原样写回：%q", *again.Content)
	}
}

func TestYAMLMergeKeysRoundTrip(t *testing.T) {
	svc, _ := newFixture(t)
	document := "base: &base\n  Name: demo\n  Port: 8080\nservice:\n  <<: *base\n  Port: 9090\n"
	initial, err := crypto.Encrypt(svc.Root(), []byte(document), "yaml", "demo/dev/merge.enc.yaml")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(svc.Root(), "demo", "dev", "merge.enc.yaml"), string(initial))
	data, err := svc.Read("demo", "dev", "merge.enc.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Save(&models.SaveRequest{
		Service: "demo", Environment: "dev", Filename: "merge.enc.yaml",
		Revision: data.Revision, Content: &document,
	}); err != nil {
		t.Fatal(err)
	}
	again, err := svc.Read("demo", "dev", "merge.enc.yaml")
	if err != nil {
		t.Fatal(err)
	}
	before, _ := models.ParseDocument(document, "yaml")
	after, _ := models.ParseDocument(*again.Content, "yaml")
	if !before.Equal(after) {
		t.Fatalf("合并键往返后语义应一致：\n%q", *again.Content)
	}
}

func TestTamperedCiphertextFailsMAC(t *testing.T) {
	svc, _ := newFixture(t)
	path := filepath.Join(svc.Root(), "demo", "dev", "secrets.enc.yaml")
	data, _ := os.ReadFile(path)
	index := bytes.Index(data, []byte("data:"))
	if index < 0 {
		t.Fatal("密文里找不到数据段")
	}
	tampered := append([]byte{}, data...)
	tampered[index+5] = tampered[index+5] ^ 0x01
	if _, err := crypto.Decrypt(tampered, "yaml", svc.Identities()); err == nil {
		t.Fatal("被改动的密文不应通过校验")
	} else {
		mustFail(t, err, 422, "")
	}
	// 篡改后的文件在页面上也应打不开
	writeFile(t, path, string(tampered))
	_, err := svc.Read("demo", "dev", "secrets.enc.yaml")
	mustFail(t, err, 422, "")
}

func TestSecretShapeIsFlatStringMapping(t *testing.T) {
	nested := &models.Value{Kind: models.KindMapping, Tag: "!!map", Map: []models.Entry{{
		Key: "NESTED", Value: &models.Value{Kind: models.KindMapping, Tag: "!!map"},
	}}}
	if _, err := service.SecretPairs(nested); err == nil {
		t.Fatal("嵌套值应被拒绝")
	}
	lowercase := &models.Value{Kind: models.KindMapping, Tag: "!!map", Map: []models.Entry{{
		Key: "lower", Value: &models.Value{Kind: models.KindString, Tag: "!!str", Raw: "x"},
	}}}
	if _, err := service.SecretPairs(lowercase); err == nil {
		t.Fatal("小写键名应被拒绝")
	}
}

func TestSetPageKeyRejectsGarbage(t *testing.T) {
	svc, _ := newFixture(t)
	_, err := svc.SetPageKey("not-an-age-key")
	mustFail(t, err, 400, "不是有效的 age 私钥")
}

func TestParseSaveRequestRejectsDuplicateKeys(t *testing.T) {
	_, err := service.ParseSaveRequest([]byte(`{"service":"a","service":"b"}`))
	mustFail(t, err, 400, "JSON 中存在重复的键")
}

func TestReadMissingConfigFails(t *testing.T) {
	svc, _ := newFixture(t)
	_, err := svc.Read("demo", "dev", "missing.yaml")
	mustFail(t, err, 404, "配置文件不存在")
}

func TestFilesAtArbitraryLocations(t *testing.T) {
	svc, _ := newFixture(t)
	writeFile(t, filepath.Join(svc.Root(), "root.yaml"), "Name: root\n")
	writeFile(t, filepath.Join(svc.Root(), "a", "b", "c", "deep.yaml"), "Name: deep\n")

	// 根目录、服务目录（无环境）、三级嵌套都能读
	for _, group := range []struct{ service, environment, filename, want string }{
		{"", "", "root.yaml", "Name: root\n"},
		{"a", "b/c", "deep.yaml", "Name: deep\n"},
	} {
		data, err := svc.Read(group.service, group.environment, group.filename)
		if err != nil {
			t.Fatalf("读取 %s 失败：%v", group.filename, err)
		}
		if data.Content == nil || *data.Content != group.want {
			t.Fatalf("%s 内容不对：%q", group.filename, *data.Content)
		}
	}

	// 密钥文件可以保存到任意层级：服务目录（无环境）与根目录
	// 服务目录需要先存在（工具不代建目录，与旧版一致）
	if err := os.MkdirAll(filepath.Join(svc.Root(), "team"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, group := range []struct{ service, environment, filename string }{
		{"team", "", "secrets.enc.yaml"},
		{"", "", "secrets.enc.yaml"},
	} {
		if _, err := svc.Save(&models.SaveRequest{
			Service: group.service, Environment: group.environment, Filename: group.filename,
			Values: mapping(map[string]string{"TOKEN": "t-" + group.service}),
		}); err != nil {
			t.Fatalf("保存 %s/%s 失败：%v", group.service, group.filename, err)
		}
		data, err := svc.Read(group.service, group.environment, group.filename)
		if err != nil {
			t.Fatal(err)
		}
		if len(data.Values.Map) != 1 || data.Values.Map[0].Value.Raw != "t-"+group.service {
			t.Fatalf("回读 %s 不对：%+v", group.service, data.Values.Map)
		}
	}

	// 目录树按「服务 / 环境」正确分组：根目录文件、无环境服务、多级嵌套各成一组
	files := map[string]map[string]bool{}
	for _, serviceEntry := range svc.Catalog().Services {
		for _, environment := range serviceEntry.Environments {
			key := serviceEntry.Name + "/" + environment.Name
			if files[key] == nil {
				files[key] = map[string]bool{}
			}
			for _, file := range environment.Files {
				files[key][file.Name] = true
			}
		}
	}
	for key, want := range map[string]map[string]bool{
		"/":        {"root.yaml": true},
		"team/":    {"secrets.enc.yaml": true},
		"a/b/c":    {"deep.yaml": true},
		"demo/dev": {"config.dev.yaml": true, "secrets.enc.yaml": true},
	} {
		for name := range want {
			if !files[key][name] {
				t.Fatalf("分组 %s 应包含 %s，实际 %v", key, name, files[key])
			}
		}
	}
}

func TestBatchCommandsRoundTrip(t *testing.T) {
	svc, _ := newFixture(t)
	var stdout, stderr bytes.Buffer

	if code := batch.RunBatch(svc, "check", nil, &stdout, &stderr); code != 0 {
		t.Fatalf("初始应全是密文，check 应退 0，实际 %d：%s", code, stderr.String())
	}
	if code := batch.RunBatch(svc, "status", nil, &stdout, &stderr); code != 0 {
		t.Fatalf("status 应退 0，实际 %d", code)
	}
	if !strings.Contains(stdout.String(), "encrypted") {
		t.Fatalf("status 应报告密文状态：%s", stdout.String())
	}
	// 缺 --yes 时拒绝解密
	stdout.Reset()
	if code := batch.RunBatch(svc, "decrypt", nil, &stdout, &stderr); code != 1 {
		t.Fatalf("不带 --yes 的 decrypt 应退 1，实际 %d", code)
	}
	// 就地解密 → 有明文 → check 退 1 → encrypt 加密回去 → check 恢复 0
	if code := batch.RunBatch(svc, "decrypt", []string{"--yes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("decrypt --yes 应退 0，实际 %d：%s", code, stderr.String())
	}
	if code := batch.RunBatch(svc, "check", nil, &stdout, &stderr); code != 1 {
		t.Fatalf("解密后 check 应退 1，实际 %d", code)
	}
	if code := batch.RunBatch(svc, "encrypt", nil, &stdout, &stderr); code != 0 {
		t.Fatalf("encrypt 应退 0，实际 %d：%s", code, stderr.String())
	}
	if code := batch.RunBatch(svc, "check", nil, &stdout, &stderr); code != 0 {
		t.Fatalf("加密回去后 check 应退 0，实际 %d", code)
	}
	// show 只输出到标准输出，不落盘修改
	stdout.Reset()
	if code := batch.RunBatch(svc, "show", []string{"demo/dev/secrets.enc.yaml"}, &stdout, &stderr); code != 0 {
		t.Fatalf("show 应退 0，实际 %d：%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "example.com") {
		t.Fatalf("show 应输出解密内容：%s", stdout.String())
	}
}

func TestAtomicWriteLeavesNoTempFiles(t *testing.T) {
	svc, _ := newFixture(t)
	data, err := svc.Read("demo", "dev", "secrets.enc.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Save(&models.SaveRequest{
		Service: "demo", Environment: "dev", Filename: "secrets.enc.yaml",
		Revision: data.Revision, Values: mapping(map[string]string{"HOST": "h", "TOKEN": "t"}),
	}); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(svc.Root(), "demo", "dev"))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") && strings.Contains(entry.Name(), ".tmp") {
			t.Fatalf("原子写不应留下临时文件：%s", entry.Name())
		}
	}
}
