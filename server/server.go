/*
 * @Author: kamalyes 501893067@qq.com
 * @Date: 2026-11-03 21:25:19
 * @LastEditors: kamalyes 501893067@qq.com
 * @LastEditTime: 2026-11-03 22:36:07
 * @FilePath: \sops-fulla\server\server.go
 * @Description: 本机管理页面的HTTP服务：仅监听127.0.0.1，会话令牌防跨站
 *
 * Copyright (c) 2026 by kamalyes, All Rights Reserved.
 */

// Package server 提供本机配置管理页面的 HTTP 服务
//
// 只监听 127.0.0.1；Host / Origin 校验挡住其他页面，API 需要随页面
// 一次性下发的会话令牌页面不加载外部资源，也不落任何持久化缓存
package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/kamalyes/sops-fulla/constants"
	"github.com/kamalyes/sops-fulla/errors"
	"github.com/kamalyes/sops-fulla/models"
	"github.com/kamalyes/sops-fulla/resources"
	"github.com/kamalyes/sops-fulla/service"
)

// Server 是配置管理页面的本机 HTTP 服务
type Server struct {
	svc        *service.Service
	token      string
	listener   net.Listener
	httpServer *http.Server
	port       int
	done       chan struct{}
}

// New 创建服务并生成一次性会话令牌
func New(svc *service.Service) *Server {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		panic("生成本机会话令牌失败：" + err.Error())
	}
	return &Server{
		svc:   svc,
		token: base64.RawURLEncoding.EncodeToString(buffer),
		done:  make(chan struct{}),
	}
}

// Start 监听给定地址并在后台开始服务
func (s *Server) Start(address string) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	s.listener = listener
	s.port = listener.Addr().(*net.TCPAddr).Port
	s.httpServer = &http.Server{Handler: s.Handler()}
	go func() {
		s.httpServer.Serve(listener)
		close(s.done)
	}()
	return nil
}

// Port 返回实际监听的端口（传 0 时为随机端口）
func (s *Server) Port() int { return s.port }

// Token 返回本次会话的访问令牌
func (s *Server) Token() string { return s.token }

// Shutdown 优雅停止服务
func (s *Server) Shutdown(ctx context.Context) error {
	if s.httpServer == nil {
		return nil
	}
	err := s.httpServer.Shutdown(ctx)
	<-s.done
	return err
}

// Close 立即停止服务
func (s *Server) Close() error {
	if s.httpServer == nil {
		return nil
	}
	err := s.httpServer.Close()
	<-s.done
	return err
}

// Handler 返回完整的 HTTP 处理器
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'")
		if err := s.route(w, r); err != nil {
			s.sendError(w, err)
		}
	})
}

// route 统一做本机与令牌校验，再分发到具体接口
func (s *Server) route(w http.ResponseWriter, r *http.Request) error {
	host := "127.0.0.1:" + strconv.Itoa(s.port)
	if r.Host != host {
		return errors.New(403, errors.BizErrCodeForbidden, "只允许通过本机管理地址访问")
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+host {
		return errors.New(403, errors.BizErrCodeForbidden, "不允许从其他页面访问本机配置")
	}
	// 会话令牌失效返回 401：页面据此自动刷新拿新令牌（服务重启过就会遇到）
	if strings.HasPrefix(r.URL.Path, "/api/") &&
		subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Config-Token")), []byte(s.token)) != 1 {
		return errors.New(401, errors.BizErrCodeUnauthorized, "页面会话已失效，请刷新后重试")
	}
	query := r.URL.Query()
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/catalog":
		return sendJSON(w, http.StatusOK, s.svc.Catalog())
	case r.Method == http.MethodGet && r.URL.Path == "/api/file":
		data, err := s.svc.Read(query.Get("service"), query.Get("environment"), query.Get("filename"))
		if err != nil {
			return err
		}
		return sendJSON(w, http.StatusOK, data)
	case r.Method == http.MethodGet && r.URL.Path == "/api/workspaces":
		return sendJSON(w, http.StatusOK, s.svc.WorkspacesInfo())
	case r.Method == http.MethodGet && r.URL.Path == "/api/appearance":
		return sendJSON(w, http.StatusOK, service.LoadAppearance())
	case r.Method == http.MethodPost && r.URL.Path == "/api/appearance":
		return s.handleAppearance(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/workspace":
		return s.handleWorkspace(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/key":
		return s.handleKey(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/keypair":
		return s.handleKeyPair(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/rotate":
		return s.handleRotate(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/api/save":
		return s.handleSave(w, r)
	case r.Method == http.MethodGet:
		return s.serveStatic(w, r.URL.Path)
	case r.Method == http.MethodPost:
		return errors.New(404, errors.BizErrCodeNotFound, "接口不存在")
	}
	return errors.New(404, errors.BizErrCodeNotFound, "页面不存在")
}

// handleWorkspace 切换到另一个配置仓库，成功后直接返回新目录，
// 页面一次往返就能完成刷新
func (s *Server) handleWorkspace(w http.ResponseWriter, r *http.Request) error {
	body, err := readJSONBody(r)
	if err != nil {
		return err
	}
	var request models.WorkspaceRequest
	if err := json.Unmarshal(body, &request); err != nil {
		return errors.New(400, errors.BizErrCodeInvalidRequest, "请求格式无效")
	}
	if err := s.svc.SwitchRoot(request.Path); err != nil {
		return err
	}
	return sendJSON(w, http.StatusOK, s.svc.Catalog())
}

// handleAppearance 保存页面个性化设置（主题色/字体/字号）到本机配置目录
func (s *Server) handleAppearance(w http.ResponseWriter, r *http.Request) error {
	body, err := readJSONBody(r)
	if err != nil {
		return err
	}
	var appearance service.Appearance
	if err := json.Unmarshal(body, &appearance); err != nil {
		return errors.New(400, errors.BizErrCodeInvalidRequest, "请求格式无效")
	}
	if err := service.SaveAppearance(appearance); err != nil {
		return err
	}
	return sendJSON(w, http.StatusOK, service.LoadAppearance())
}

func (s *Server) handleKey(w http.ResponseWriter, r *http.Request) error {
	body, err := readJSONBody(r)
	if err != nil {
		return err
	}
	var request struct {
		Secret           string `json:"secret"`
		Remember         bool   `json:"remember"`
		ForgetRemembered bool   `json:"forget_remembered"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		return errors.New(400, errors.BizErrCodeInvalidRequest, "请求格式无效")
	}
	if request.ForgetRemembered {
		if _, err := s.svc.ForgetRememberedKey(); err != nil {
			return err
		}
		return sendJSON(w, http.StatusOK, map[string]any{
			"secrets_ready": s.svc.Capabilities().SecretsReady,
			"key_source":    s.svc.Capabilities().KeySource,
			"message":       "已清除记住的私钥，下次启动不再自动加载",
		})
	}
	capabilities, err := s.svc.SetPageKey(request.Secret)
	if err != nil {
		return err
	}
	message := "已清除页面私钥，回落到环境变量或本机密钥文件"
	if request.Secret != "" {
		if request.Remember {
			if _, err := s.svc.RememberKey(request.Secret); err != nil {
				return err
			}
			message = "已应用页面提供的私钥，并记住到本机（下次启动自动可用）"
		} else {
			message = "已应用页面提供的私钥，仅本次会话有效，不会保存"
		}
	}
	return sendJSON(w, http.StatusOK, map[string]any{
		"secrets_ready": capabilities.SecretsReady,
		"key_source":    capabilities.KeySource,
		"message":       message,
	})
}

// handleKeyPair 生成一对全新的 age 密钥，辅助页面的「轮换到新钥匙」流程
// 无需请求体；私钥只在本次响应里出现，服务端不保存
func (s *Server) handleKeyPair(w http.ResponseWriter, r *http.Request) error {
	pair, err := service.GenerateKeyPair()
	if err != nil {
		return err
	}
	return sendJSON(w, http.StatusOK, pair)
}

// handleRotate 把仓库里全部密文轮换到新公钥，需要当前私钥已就绪
func (s *Server) handleRotate(w http.ResponseWriter, r *http.Request) error {
	body, err := readJSONBody(r)
	if err != nil {
		return err
	}
	var request models.RotateRequest
	if err := json.Unmarshal(body, &request); err != nil {
		return errors.New(400, errors.BizErrCodeInvalidRequest, "请求格式无效")
	}
	result, err := s.svc.RotateAll(request.Recipient)
	if err != nil {
		return err
	}
	return sendJSON(w, http.StatusOK, result)
}

func (s *Server) handleSave(w http.ResponseWriter, r *http.Request) error {
	body, err := readJSONBody(r)
	if err != nil {
		return err
	}
	request, err := service.ParseSaveRequest(body)
	if err != nil {
		return err
	}
	result, err := s.svc.Save(request)
	if err != nil {
		return err
	}
	return sendJSON(w, http.StatusOK, result)
}

// readJSONBody 统一校验 Content-Type 与请求体大小
func readJSONBody(r *http.Request) ([]byte, error) {
	if mediaType(r.Header.Get("Content-Type")) != "application/json" {
		return nil, errors.New(415, errors.BizErrCodeJSONOnly, "只接受 JSON 请求")
	}
	if r.ContentLength <= 0 || r.ContentLength > constants.MaxBodyBytes {
		return nil, errors.New(413, errors.BizErrCodePayloadTooLarge, "请求内容为空或过大")
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, constants.MaxBodyBytes+1))
	if err != nil {
		return nil, errors.New(400, errors.BizErrCodeInvalidRequest, "请求格式无效")
	}
	if len(body) > constants.MaxBodyBytes {
		return nil, errors.New(413, errors.BizErrCodePayloadTooLarge, "请求内容为空或过大")
	}
	return body, nil
}

func mediaType(contentType string) string {
	value, _, _ := strings.Cut(contentType, ";")
	return strings.TrimSpace(value)
}

// staticFiles 是页面路由到内嵌资源的映射
var staticFiles = map[string]struct {
	name        string
	contentType string
}{
	"/":          {"web/index.html", "text/html"},
	"/app.js":    {"web/app.js", "text/javascript"},
	"/style.css": {"web/style.css", "text/css"},
}

// serveStatic 从内嵌资源返回页面文件；首页注入本次会话的访问令牌
func (s *Server) serveStatic(w http.ResponseWriter, path string) error {
	file, ok := staticFiles[path]
	if !ok {
		return errors.New(404, errors.BizErrCodeNotFound, "页面不存在")
	}
	content, err := resources.Files.ReadFile(file.name)
	if err != nil {
		return errors.New(404, errors.BizErrCodeNotFound, "页面不存在")
	}
	if path == "/" {
		content = bytes.ReplaceAll(content, []byte("__SESSION_TOKEN__"), []byte(s.token))
	}
	w.Header().Set("Content-Type", file.contentType+"; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(content)
	return nil
}

func sendJSON(w http.ResponseWriter, status int, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("响应序列化失败：%w", err)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	w.Write(data)
	return nil
}

// sendError 把业务错误转成 JSON 响应；非业务错误按 500 处理，不泄露细节
func (s *Server) sendError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := "操作未完成，请刷新后重试；现有配置未被覆盖"
	if business, ok := errors.As(err); ok {
		status = business.Status
		message = business.Message
	}
	sendJSON(w, status, map[string]string{"error": message})
}
