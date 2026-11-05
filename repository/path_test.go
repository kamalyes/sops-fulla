/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-05 11:30:00
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-05 11:30:00
 * @FilePath: \sops-fulla\repository\path_test.go
 * @Description: 路径段校验与中文目录名解析测试
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package repository

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidName(t *testing.T) {
	cases := []struct {
		value string
		want  bool
	}{
		{"demo", true}, {"demo-gateway", true}, {"demo_2.prod", true}, {"secrets.enc.yaml", true},
		{"网关服务", true}, {"网关-服务_v2", true}, {"生产环境", true}, {"配置.enc.yaml", true},
		{"", false}, {".", false}, {"..", false}, {".hidden", false},
		{"-start", false}, {"_start", false}, {"has space", false}, {"a/b", false}, {`a\b`, false},
		{`a:b`, false}, {`a<b`, false}, {`a?b`, false}, {`a*b`, false},
	}
	for _, item := range cases {
		if got := validName(item.value); got != item.want {
			t.Errorf("validName(%q) = %v, 期望 %v", item.value, got, item.want)
		}
	}
}

func TestResolveChineseSegments(t *testing.T) {
	root := t.TempDir()
	service := filepath.Join(root, "网关服务", "生产环境")
	if err := os.MkdirAll(service, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(service, "app.yaml")
	if err := os.WriteFile(target, []byte("name: demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo := New(root)
	resolved, err := repo.Resolve("网关服务", "生产环境", "app.yaml")
	if err != nil {
		t.Fatalf("中文服务/环境名应能解析：%v", err)
	}
	if resolved != target {
		t.Fatalf("解析结果 %q，期望 %q", resolved, target)
	}
	if _, err := repo.Resolve("网关服务", "生产环境", "../app.yaml"); err == nil {
		t.Fatal("越界路径应被拒绝")
	}
}
