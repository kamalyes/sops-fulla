/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-05 00:13:31
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-05 00:19:08
 * @FilePath: \sops-fulla\crypto\rotate_test.go
 * @Description: .sops.yaml 公钥替换测试：保留注释排版，拒绝多公钥行
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package crypto

import (
	"strings"
	"testing"
)

func TestUpdateRulesRecipientReplacesAgeLine(t *testing.T) {
	rules := "# 仓库的加密规则\n" +
		"creation_rules:\n" +
		"  - path_regex: \\.enc\\.(yaml|yml|json)$\n" +
		"    age: age1aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n"
	updated, changed, err := UpdateRulesRecipient([]byte(rules), "age1bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("公钥有变化时应报告 changed")
	}
	text := string(updated)
	if !strings.Contains(text, "age1bbbb") || strings.Contains(text, "age1aaaa") {
		t.Fatalf("公钥应替换为新值，实际：\n%s", text)
	}
	if !strings.Contains(text, "# 仓库的加密规则") || !strings.Contains(text, "path_regex") {
		t.Fatalf("注释与 path_regex 应保持原样，实际：\n%s", text)
	}
}

func TestUpdateRulesRecipientKeepsFileWhenNoAgeLine(t *testing.T) {
	rules := "creation_rules:\n  - path_regex: .*$\n    pgp: 'ABCDEF'\n"
	updated, changed, err := UpdateRulesRecipient([]byte(rules), "age1cccc")
	if err != nil {
		t.Fatal(err)
	}
	if changed || string(updated) != rules {
		t.Fatal("没有 age 行时应原样返回且 changed 为 false")
	}
}

func TestUpdateRulesRecipientRejectsMultiRecipientRule(t *testing.T) {
	rules := "creation_rules:\n" +
		"  - path_regex: \\.enc\\.(yaml|yml|json)$\n" +
		"    age: age1aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa,age1dddd\n"
	_, _, err := UpdateRulesRecipient([]byte(rules), "age1eeee")
	if err == nil {
		t.Fatal("一条规则配多把公钥时应拒绝自动替换")
	}
}
