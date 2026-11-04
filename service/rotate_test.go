/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-05 00:15:21
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-05 00:20:11
 * @FilePath: \sops-fulla\service\rotate_test.go
 * @Description: 密钥轮换测试：只换age段、规则同步更新、失败不留半成品
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package service_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"filippo.io/age"

	"github.com/kamalyes/sops-fulla/crypto"
	"github.com/kamalyes/sops-fulla/service"
)

// macLine 取密文里的 MAC 行，轮换前后应完全一致
func macLine(content string) string {
	for _, line := range strings.Split(content, "\n") {
		if strings.Contains(line, "mac: ENC[") {
			return line
		}
	}
	return ""
}

// recipientOf 从私钥文本反推公钥
func recipientOf(t *testing.T, identityText string) string {
	t.Helper()
	identity, err := age.ParseX25519Identity(identityText)
	if err != nil {
		t.Fatal(err)
	}
	return identity.Recipient().String()
}

func TestRotateAllRewrapsToNewRecipient(t *testing.T) {
	svc, identityText := newFixture(t)
	path := filepath.Join(svc.Root(), "demo", "dev", "secrets.enc.yaml")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rulesBefore, err := os.ReadFile(filepath.Join(svc.Root(), ".sops.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	fresh, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.RotateAll(fresh.Recipient().String())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Files) != 1 || result.Files[0] != "demo/dev/secrets.enc.yaml" {
		t.Fatalf("应轮换 demo/dev/secrets.enc.yaml 一份，实际 %+v", result.Files)
	}
	if !result.RulesUpdated {
		t.Fatal(".sops.yaml 应同步更新")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// 值密文与 MAC 逐字节不变：换的只是 age 段
	if got := encLine(string(after), "HOST"); got == "" || got != encLine(string(before), "HOST") {
		t.Fatalf("HOST 的值密文应保持不变，实际 %q", got)
	}
	if macLine(string(after)) != macLine(string(before)) {
		t.Fatal("MAC 行应保持不变")
	}
	if !strings.Contains(string(after), "recipient: "+fresh.Recipient().String()) {
		t.Fatal("密文应携带新公钥")
	}
	if strings.Contains(string(after), recipientOf(t, identityText)) {
		t.Fatal("密文不应再携带旧公钥")
	}

	rulesAfter, err := os.ReadFile(filepath.Join(svc.Root(), ".sops.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rulesAfter), fresh.Recipient().String()) {
		t.Fatal(".sops.yaml 应写进新公钥")
	}
	if !strings.Contains(string(rulesAfter), "path_regex") {
		t.Fatal(".sops.yaml 的 path_regex 应保留")
	}
	_ = rulesBefore

	// 旧私钥已解不开；页面切到新私钥后立即可读
	if _, err := svc.SetPageKey(fresh.String()); err != nil {
		t.Fatal(err)
	}
	data, err := svc.Read("demo", "dev", "secrets.enc.yaml")
	if err != nil {
		t.Fatalf("新私钥应能读取轮换后的密文：%v", err)
	}
	if data.Revision == nil || *data.Revision == "" {
		t.Fatal("读取结果应带 revision")
	}
}

func TestRotateAllRequiresIdentity(t *testing.T) {
	svc, _ := newFixture(t)
	bare := service.NewIsolated(svc.Root())
	fresh, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	_, err = bare.RotateAll(fresh.Recipient().String())
	mustFail(t, err, 503, "尚未就绪")
}

func TestRotateAllLeavesDiskUntouchedWhenAnyFileFails(t *testing.T) {
	svc, _ := newFixture(t)
	second := filepath.Join(svc.Root(), "other", "prod", "secrets.enc.yaml")
	initial, err := crypto.Encrypt(svc.Root(), []byte("A: one\n"), "yaml", "other/prod/secrets.enc.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(second), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, initial, 0o600); err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(svc.Root(), "demo", "dev", "secrets.enc.yaml")
	firstBefore, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	rulesBefore, err := os.ReadFile(filepath.Join(svc.Root(), ".sops.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	// 篡改第二份的密文，轮换应整体中止，第一份与 .sops.yaml 一个字节都不动
	tampered := strings.Replace(string(initial), "ENC[", "ENCX[", 1)
	if err := os.WriteFile(second, []byte(tampered), 0o600); err != nil {
		t.Fatal(err)
	}
	fresh, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.RotateAll(fresh.Recipient().String())
	if err == nil {
		t.Fatal("含被篡改的密文时轮换应失败")
	}
	firstAfter, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstAfter) != string(firstBefore) {
		t.Fatal("轮换失败时不应改动任何文件")
	}
	rulesAfter, err := os.ReadFile(filepath.Join(svc.Root(), ".sops.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(rulesAfter) != string(rulesBefore) {
		t.Fatal("轮换失败时不应改动 .sops.yaml")
	}
}

func TestRotateAllRejectsEmptyRecipient(t *testing.T) {
	svc, _ := newFixture(t)
	_, err := svc.RotateAll("   ")
	mustFail(t, err, 400, "请填写新的 age 公钥")
}
