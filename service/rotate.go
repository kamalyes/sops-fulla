/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-05 00:08:37
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-05 00:18:05
 * @FilePath: \sops-fulla\service\rotate.go
 * @Description: 密钥轮换编排：全量密文换新公钥，值密文与MAC保持不变
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

package service

import (
	"path/filepath"
	"strings"

	"filippo.io/age"

	"github.com/kamalyes/sops-fulla/crypto"
	"github.com/kamalyes/sops-fulla/errors"
	"github.com/kamalyes/sops-fulla/models"
	"github.com/kamalyes/sops-fulla/repository"
)

// RotateAll 把仓库里全部密文轮换到新公钥，并把 .sops.yaml 的 age 公钥
// 一并更新，保证轮换后继续保存的文件也用新钥匙
// 数据密钥不变：值密文与 MAC 逐字节不动，git diff 里只有 sops.age 段
// 两阶段执行：全部文件先在内存里轮换成功，再统一落盘，中途失败不写任何文件
func (s *Service) RotateAll(recipient string) (*models.RotateResult, error) {
	recipient = strings.TrimSpace(recipient)
	if recipient == "" {
		return nil, errors.New(400, errors.BizErrCodeInvalidRequest, "请填写新的 age 公钥")
	}
	identities := s.Identities()
	if len(identities) == 0 {
		return nil, errors.New(503, errors.BizErrCodeSecretsNotReady, "密钥轮换尚未就绪，请先在页面填写当前 age 私钥")
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	// 阶段一：逐份在内存里轮换，任何一份失败都直接返回，磁盘不动
	targets := s.repo.EncryptedFiles()
	type rotated struct {
		target string
		rel    string
		output []byte
	}
	pending := make([]rotated, 0, len(targets))
	for _, target := range targets {
		rel, err := relativePath(s.Root(), target)
		if err != nil {
			return nil, err
		}
		raw, ok := s.repo.ReadFile(target)
		if !ok {
			continue
		}
		output, err := crypto.RotateRecipient(raw, models.FileKind(filepath.Base(target)), recipient, identities)
		if err != nil {
			return nil, annotate(err, rel)
		}
		pending = append(pending, rotated{target: target, rel: rel, output: output})
	}

	rulesPath := filepath.Join(s.Root(), ".sops.yaml")
	rules, hasRules := s.repo.ReadFile(rulesPath)
	var updatedRules []byte
	rulesChanged := false
	if hasRules {
		var err error
		updatedRules, rulesChanged, err = crypto.UpdateRulesRecipient(rules, recipient)
		if err != nil {
			return nil, annotate(err, ".sops.yaml")
		}
	}

	// 阶段二：全部就绪，统一落盘
	result := &models.RotateResult{Files: make([]string, 0, len(pending))}
	for _, item := range pending {
		if err := repository.AtomicWrite(item.target, item.output); err != nil {
			return nil, annotate(err, item.rel)
		}
		result.Files = append(result.Files, item.rel)
	}
	if rulesChanged {
		if err := repository.AtomicWrite(rulesPath, updatedRules); err != nil {
			return nil, annotate(err, ".sops.yaml")
		}
		result.RulesUpdated = true
	}
	return result, nil
}

// GenerateKeyPair 生成一对全新的 age 密钥：页面「轮换到新钥匙」时用
// 私钥只在返回值里出现一次，由使用者自行保存，服务端不落盘不留副本
func GenerateKeyPair() (*models.KeyPair, error) {
	identity, err := age.GenerateX25519Identity()
	if err != nil {
		return nil, errors.New(500, errors.BizErrCodeInitFailed, "生成 age 密钥失败")
	}
	return &models.KeyPair{
		Recipient: identity.Recipient().String(),
		Identity:  identity.String(),
	}, nil
}

// annotate 给业务错误补上出问题的文件路径，方便定位是哪一份密文
func annotate(err error, relPath string) error {
	if business, ok := errors.As(err); ok {
		return errors.Newf(business.Status, business.Code, "%s：%s", relPath, business.Message)
	}
	return err
}
