/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-03 23:31:08
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-05 00:05:02
 * @FilePath: \sops-fulla\bootstrap\init_test.go
 * @Description: init子命令测试：密钥生成/复用、规则写入与示例脚手架
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package bootstrap_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"
	agekey "github.com/getsops/sops/v3/age"

	"github.com/kamalyes/sops-fulla/bootstrap"
	"github.com/kamalyes/sops-fulla/crypto"
	"github.com/kamalyes/sops-fulla/service"
)

// isolateUserConfig 把用户配置目录指到临时目录，测试不碰宿主机的密钥
func isolateUserConfig(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("APPDATA", dir)
	t.Setenv("SOPS_AGE_KEY", "")
	t.Setenv("SOPS_AGE_KEY_FILE", "")
}

// runInit 执行 init 并要求退出码为 0
func runInit(t *testing.T, repo string, args []string) bytes.Buffer {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if code := bootstrap.RunInit(repo, args, &stdout, &stderr); code != 0 {
		t.Fatalf("init 应成功，退出码 %d，输出：%s", code, stderr.String())
	}
	return stdout
}

func TestInitCreatesRulesAndKey(t *testing.T) {
	isolateUserConfig(t)
	repo := t.TempDir()
	stdout := runInit(t, repo, nil)

	rules, err := os.ReadFile(filepath.Join(repo, ".sops.yaml"))
	if err != nil {
		t.Fatalf(".sops.yaml 应写入：%v", err)
	}
	if !strings.Contains(string(rules), "age: age1") || !strings.Contains(string(rules), "path_regex") {
		t.Fatalf(".sops.yaml 应包含 path_regex 与 age 公钥，实际：\n%s", rules)
	}
	keyPath, err := service.StandardAgeKeyPath()
	if err != nil {
		t.Fatal(err)
	}
	keyText, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("本机私钥文件应写入 %s：%v", keyPath, err)
	}
	if !strings.HasPrefix(strings.TrimSpace(string(keyText)), "AGE-SECRET-KEY-1") {
		t.Fatalf("私钥文件应以 AGE-SECRET-KEY-1 开头，实际 %q", keyText)
	}
	if !strings.Contains(stdout.String(), "已就绪") || !strings.Contains(stdout.String(), "备份") {
		t.Fatalf("摘要应提示就绪与备份，实际：\n%s", stdout.String())
	}

	// 二次 init 应拒绝覆盖
	var stdout2, stderr2 bytes.Buffer
	if code := bootstrap.RunInit(repo, nil, &stdout2, &stderr2); code != 1 {
		t.Fatalf("重复 init 应退出 1，实际 %d", code)
	}
}

func TestInitScaffoldsExample(t *testing.T) {
	isolateUserConfig(t)
	repo := t.TempDir()
	runInit(t, repo, []string{"--example"})

	plain, err := os.ReadFile(filepath.Join(repo, "demo-gateway", "dev", "config.dev.yaml"))
	if err != nil || !strings.Contains(string(plain), "demo-gateway") {
		t.Fatalf("示例明文配置应生成：%v", err)
	}
	ciphertext, err := os.ReadFile(filepath.Join(repo, "demo-gateway", "dev", "secrets.enc.yaml"))
	if err != nil || !strings.Contains(string(ciphertext), "ENC[") {
		t.Fatalf("示例密钥文件应为密文：%v", err)
	}
	// 用刚生成的私钥能解回来：init 完即可用，无需任何额外步骤
	keyPath, _ := service.StandardAgeKeyPath()
	keyText, _ := os.ReadFile(keyPath)
	identities, err := age.ParseIdentities(bytes.NewReader(keyText))
	if err != nil {
		t.Fatal(err)
	}
	plainSecrets, err := crypto.Decrypt(ciphertext, "yaml", agekey.ParsedIdentities(identities))
	if err != nil {
		t.Fatalf("init 生成的密文应能用 init 生成的私钥解开：%v", err)
	}
	if !strings.Contains(string(plainSecrets), "EXAMPLE_API_KEY") {
		t.Fatalf("解出的内容应包含示例键，实际：\n%s", plainSecrets)
	}
}

func TestInitReusesExistingKey(t *testing.T) {
	isolateUserConfig(t)
	repo := t.TempDir()
	keyPath, err := service.StandardAgeKeyPath()
	if err != nil {
		t.Fatal(err)
	}
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte(identity.String()+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(keyPath)

	stdout := runInit(t, repo, nil)
	rules, err := os.ReadFile(filepath.Join(repo, ".sops.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rules), identity.Recipient().String()) {
		t.Fatal(".sops.yaml 应使用已有私钥对应的公钥")
	}
	after, _ := os.ReadFile(keyPath)
	if string(before) != string(after) {
		t.Fatal("已存在的私钥文件不应被改动")
	}
	if !strings.Contains(stdout.String(), "复用已有私钥") {
		t.Fatalf("摘要应说明复用已有私钥，实际：\n%s", stdout.String())
	}
}
