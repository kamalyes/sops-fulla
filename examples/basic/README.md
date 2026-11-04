# basic 示例仓库

一个开箱即用的配置仓库样例：克隆 [sops-fulla](https://github.com/kamalyes/sops-fulla)
后不用做任何准备，直接对 `examples/basic` 跑工具即可体验完整流程

```bash
# 管理页面（会自动打开浏览器；左下角粘贴 demo-key.txt 的内容即可编辑密钥）
sops-fulla --repo examples/basic

# 命令行
sops-fulla --repo examples/basic status   # 每份是密文还是明文、各几个键
sops-fulla --repo examples/basic keys     # 只列键名
```

## 目录结构

```text
examples/basic/
├── .sops.yaml                      # 加密规则：文件名带 .enc. 的才加密
├── demo-key.txt                    # 演示私钥（见下方警告）
├── observability.yaml              # 根目录明文配置：放置位置不限
├── demo-gateway/
│   └── dev/
│       ├── config.dev.yaml         # 明文运行配置
│       └── secrets.enc.yaml        # 已加密的密钥文件（扁平 KEY: value）
└── platform/
    └── team-a/
        └── api.enc.yaml            # 多级嵌套的密钥文件：扫描与规则同样生效
```

## ⚠️ 演示密钥是公开材料

`demo-key.txt` 里的私钥随仓库一起公开，**仅用于本示例**
任何人都能解开这里的密文，所以里面的值全是 `change-me` 占位符
生产环境请用 `init` 生成自己的密钥对，私钥绝不能进仓库

## 与 sops CLI 互通

本工具用官方 sops 库生成标准 SOPS 密文，就算哪天不用这个工具了，
原版 sops 照样能解：

```bash
SOPS_AGE_KEY_FILE=examples/basic/demo-key.txt \
  sops -d examples/basic/demo-gateway/dev/secrets.enc.yaml
```

反过来，sops CLI 加密的文件（age 密钥）本工具也能直接打开和编辑

## 从这个示例开始自己的仓库

```bash
# 空目录里一条命令初始化：生成 age 密钥对 + 写 .sops.yaml + 可选示例结构
sops-fulla --repo /path/to/your-repo init --example

# 需要换钥匙时（左下角状态条或右侧「设置」→ 轮换到新密钥）：
# 全部密文只重写 age 段，值密文与 git diff 最小，.sops.yaml 一并更新
```
