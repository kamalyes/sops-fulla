/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-03 23:07:31
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-05 00:03:52
 * @FilePath: \sops-fulla\bootstrap\init.go
 * @Description: init子命令：生成age密钥与.sops.yaml，让运维零依赖初始化配置仓库
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package bootstrap

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"filippo.io/age"

	"github.com/kamalyes/sops-fulla/crypto"
	"github.com/kamalyes/sops-fulla/errors"
	"github.com/kamalyes/sops-fulla/service"
)

// rulesTemplate 是 init 生成的 .sops.yaml：文件名带 .enc. 才加密，
// 与批量子命令、管理页面的约定保持一致
const rulesTemplate = `creation_rules:
  # 文件名带 .enc. 的 yaml/yml/json 才加密，其余按明文管理
  - path_regex: \.enc\.(yaml|yml|json)$
    age: %s
`

// initUsage 是 init 子命令自身的用法说明
const initUsage = `用法：sops-fulla --repo <目录> init [--example]

在目标目录初始化一个配置仓库：
  1. 复用或生成 age 密钥（存放在本机标准位置，与 sops CLI 互通）
  2. 写入 .sops.yaml 加密规则
  3. --example 时生成 demo-gateway 示例结构（含一份已加密的密钥文件）
`

// RunInit 执行 init 子命令，返回进程退出码
func RunInit(root string, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("init", flag.ContinueOnError)
	withExample := flags.Bool("example", false, "生成 demo-gateway 示例结构")
	flags.Usage = func() { fmt.Fprint(stderr, initUsage) }
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "init 不接受额外参数：%s\n%s", flags.Arg(0), initUsage)
		return 2
	}

	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		fmt.Fprintf(stderr, "目录不存在或不是目录：%s\n", root)
		return 1
	}
	rulesPath := filepath.Join(root, ".sops.yaml")
	if _, err := os.Stat(rulesPath); err == nil {
		fmt.Fprintf(stderr, "%s 已存在，这个仓库初始化过了；要重置请先删掉它\n", rulesPath)
		return 1
	}

	recipient, keyPath, created, err := loadOrCreateIdentity()
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		return 1
	}
	if err := os.WriteFile(rulesPath, []byte(fmt.Sprintf(rulesTemplate, recipient)), 0o644); err != nil {
		fmt.Fprintf(stderr, "写入 .sops.yaml 失败：%v\n", err)
		return 1
	}

	if *withExample {
		if err := scaffoldExample(root); err != nil {
			fmt.Fprintf(stderr, "生成示例结构失败：%v\n", err)
			return 1
		}
	}

	printInitSummary(stdout, root, recipient, keyPath, created, *withExample)
	return 0
}

// loadOrCreateIdentity 返回 age 公钥、私钥文件路径与「是否新建」
// 本机标准位置已有可用私钥时直接复用（多个仓库共用一把钥匙），
// 否则生成新密钥对并写入；从不覆盖已有文件
func loadOrCreateIdentity() (recipient, keyPath string, created bool, err error) {
	keyPath, err = service.StandardAgeKeyPath()
	if err != nil {
		return "", "", false, err
	}
	if data, readErr := os.ReadFile(keyPath); readErr == nil && strings.TrimSpace(string(data)) != "" {
		parsed, parseErr := age.ParseIdentities(bytes.NewReader(data))
		if parseErr != nil {
			return "", "", false, errors.Newf(422, errors.BizErrCodeInitFailed, "本机已有的 %s 解析不了，请先修复或移走它", keyPath)
		}
		for _, identity := range parsed {
			if x25519, ok := identity.(*age.X25519Identity); ok {
				return x25519.Recipient().String(), keyPath, false, nil
			}
		}
		return "", "", false, errors.Newf(422, errors.BizErrCodeInitFailed, "本机已有的 %s 里没有 age 私钥", keyPath)
	}
	identity, genErr := age.GenerateX25519Identity()
	if genErr != nil {
		return "", "", false, errors.Newf(500, errors.BizErrCodeInitFailed, "生成 age 密钥失败：%v", genErr)
	}
	if mkdirErr := os.MkdirAll(filepath.Dir(keyPath), 0o700); mkdirErr != nil {
		return "", "", false, errors.Newf(500, errors.BizErrCodeInitFailed, "创建密钥目录失败：%v", mkdirErr)
	}
	if writeErr := os.WriteFile(keyPath, []byte(identity.String()+"\n"), 0o600); writeErr != nil {
		return "", "", false, errors.Newf(500, errors.BizErrCodeInitFailed, "写入私钥文件失败：%v", writeErr)
	}
	return identity.Recipient().String(), keyPath, true, nil
}

// scaffoldExample 生成 demo-gateway 示例：一份明文配置加一份用
// 刚写入的公钥加密的密钥文件，init 完成后 status / 页面立即可用
func scaffoldExample(root string) error {
	configDir := filepath.Join(root, "demo-gateway", "dev")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return err
	}
	plainConfig := "# 明文运行配置：文件名不带 .enc.，不会被加密\nname: demo-gateway\nport: 8080\n"
	if err := os.WriteFile(filepath.Join(configDir, "config.dev.yaml"), []byte(plainConfig), 0o644); err != nil {
		return err
	}
	plainSecrets := "EXAMPLE_API_KEY: change-me\nEXAMPLE_DB_PASSWORD: change-me\n"
	relPath := "demo-gateway/dev/secrets.enc.yaml"
	ciphertext, err := crypto.Encrypt(root, []byte(plainSecrets), "yaml", relPath)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, filepath.FromSlash(relPath)), ciphertext, 0o600)
}

// printInitSummary 打印初始化摘要与下一步提示，让运维不看文档也知道怎么继续
func printInitSummary(stdout io.Writer, root, recipient, keyPath string, created, withExample bool) {
	keyStatus := "复用已有私钥"
	if created {
		keyStatus = "新生成私钥"
	}
	fmt.Fprintf(stdout, "配置仓库已就绪：%s\n", root)
	fmt.Fprintf(stdout, "  加密规则  .sops.yaml（age 公钥 %s…）\n", recipient[:16])
	fmt.Fprintf(stdout, "  私钥      %s（%s）\n", keyPath, keyStatus)
	if created {
		fmt.Fprintln(stdout, "  ⚠ 私钥请立即备份并加入备份策略：丢失后所有密文都无法解密")
	}
	if withExample {
		fmt.Fprintln(stdout, "  示例结构  demo-gateway/dev/config.dev.yaml（明文）")
		fmt.Fprintln(stdout, "            demo-gateway/dev/secrets.enc.yaml（已加密）")
	}
	fmt.Fprintln(stdout, "\n下一步：")
	fmt.Fprintf(stdout, "  sops-fulla --repo %s          打开管理页面\n", root)
	fmt.Fprintf(stdout, "  sops-fulla --repo %s status   查看密文状态\n", root)
}
