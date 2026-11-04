"use strict";

const $ = (id) => document.getElementById(id);
const token = document.querySelector('meta[name="config-token"]').content;
const state = { catalog: null, service: "", environment: "", filename: "", file: null, rows: [], dirty: false, busy: false, loading: false };
let rowId = 0;

/* ---------- 基础工具 ---------- */

function element(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
}

async function api(path, options = {}) {
  const response = await fetch(path, { ...options, cache: "no-store", headers: { "X-Config-Token": token, ...(options.headers || {}) } });
  const data = await response.json();
  if (!response.ok) throw new Error(data.error || "操作未完成，请重试");
  return data;
}

/* ---------- Toast 通知 ---------- */

function toast(message, kind = "success") {
  if (!message) return;
  const host = $("toasts");
  host.querySelectorAll(".toast.leaving").forEach((node) => node.remove());
  const node = element("div", "toast " + kind);
  node.append(element("span", "toast-icon", kind === "error" ? "✕" : kind === "info" ? "i" : "✓"), element("span", "", message));
  const dismiss = () => { node.classList.add("leaving"); setTimeout(() => node.remove(), 220); };
  node.onclick = dismiss;
  host.append(node);
  setTimeout(dismiss, kind === "error" ? 7000 : 4000);
}

/* ---------- 确认弹窗（Promise 化，替代 window.confirm） ---------- */

let confirmResolve = null;

function confirmDialog(message, { title = "请确认", acceptText = "确定", danger = true } = {}) {
  $("confirm-title").textContent = title;
  $("confirm-message").textContent = message;
  $("confirm-accept").textContent = acceptText;
  $("confirm-accept").className = danger ? "button danger" : "button primary";
  $("confirm-modal").hidden = false;
  $("confirm-accept").focus();
  return new Promise((resolve) => { confirmResolve = resolve; });
}

function settleConfirm(result) {
  $("confirm-modal").hidden = true;
  if (confirmResolve) { confirmResolve(result); confirmResolve = null; }
}

$("confirm-accept").onclick = () => settleConfirm(true);
$("confirm-cancel").onclick = () => settleConfirm(false);
$("confirm-modal").onclick = (event) => { if (event.target === $("confirm-modal")) settleConfirm(false); };

/* ---------- age 私钥弹窗 ---------- */

function keySourceLabel(source) {
  return { page: "页面提供的私钥", env: "环境变量中的私钥", file: "本机密钥文件" }[source] || "";
}

function renderKeyStatus() {
  const ready = state.catalog?.secrets_ready;
  const source = state.catalog?.key_source;
  $("key-status-dot").classList.toggle("ok", Boolean(ready));
  $("key-status-text").textContent = ready
    ? "私钥就绪 · " + keySourceLabel(source)
    : "私钥未配置 · 点击设置";
  // 设置抽屉里的「当前状态」信息一并刷新
  $("info-repository").textContent = state.catalog?.repository || "—";
  $("info-key").textContent = ready ? "已就绪，可查看与保存密文" : "未配置，密文仅显示占位";
  $("info-key-source").textContent = keySourceLabel(source) || "—";
}

function openSettings() {
  $("key-input").value = "";
  const ready = state.catalog?.secrets_ready;
  $("key-status").textContent = ready
    ? "当前已使用" + keySourceLabel(state.catalog.key_source) + "。粘贴新的私钥并应用可替换；留空应用或点「清除私钥」则回落到环境变量"
    : "尚未配置可用的私钥：查看与保存密钥前需要先填写。也可以改用环境变量 SOPS_AGE_KEY / SOPS_AGE_KEY_FILE";
  $("drawer-backdrop").hidden = false;
  $("settings-drawer").hidden = false;
  $("key-input").focus();
}

function closeSettings() {
  $("settings-drawer").hidden = true;
  $("drawer-backdrop").hidden = true;
}

async function applyPageKey(secret, { clear = false } = {}) {
  const result = await api("/api/key", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ secret }) });
  state.catalog.secrets_ready = result.secrets_ready;
  state.catalog.key_source = result.key_source;
  renderKeyStatus();
  toast(result.message, result.secrets_ready || !clear ? "success" : "info");
  // 私钥就绪与否直接影响当前文件能否解密，刷新一次
  if (state.filename) await loadFile();
}

$("open-key-modal").onclick = openSettings;
$("rail-settings").onclick = openSettings;
$("drawer-close").onclick = closeSettings;
$("drawer-backdrop").onclick = closeSettings;
$("key-modal-clear").onclick = async () => {
  try { await applyPageKey("", { clear: true }); closeSettings(); }
  catch (error) { toast(error.message, "error"); }
};
$("key-modal-apply").onclick = async () => {
  const secret = $("key-input").value.trim();
  try { await applyPageKey(secret); if (secret) closeSettings(); }
  catch (error) { $("key-status").textContent = error.message; }
};
$("key-input").onkeydown = (event) => {
  if (event.key === "Enter" && (event.metaKey || event.ctrlKey)) { event.preventDefault(); $("key-modal-apply").click(); }
};

/* ---------- 密钥轮换 ---------- */

$("rotate-generate").onclick = async () => {
  try {
    const pair = await api("/api/keypair", { method: "POST", headers: { "Content-Type": "application/json" }, body: "{}" });
    $("rotate-recipient").value = pair.recipient;
    $("rotate-identity").value = pair.identity;
    $("rotate-keypair").hidden = false;
    $("rotate-status").textContent = "已生成新密钥对请先把新私钥保存到安全的地方（离开页面后不再显示），再执行轮换";
  } catch (error) { $("rotate-status").textContent = error.message; }
};

$("rotate-run").onclick = async () => {
  const recipient = $("rotate-recipient").value.trim();
  const identity = $("rotate-identity").value.trim();
  if (!recipient) { $("rotate-status").textContent = "请先生成新密钥对，或粘贴新的 age 公钥（age1 开头）"; return; }
  const confirmed = await confirmDialog("轮换会把仓库里全部密文换到新公钥，并把 .sops.yaml 的公钥一并更新确定继续吗？", { title: "轮换密钥", acceptText: "执行轮换" });
  if (!confirmed) return;
  try {
    $("rotate-run").disabled = true;
    const result = await api("/api/rotate", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ recipient }) });
    let note = "轮换完成：" + result.files.length + " 份密文已换到新公钥" + (result.rules_updated ? "，.sops.yaml 已同步更新" : "") + "";
    if (identity) {
      await applyPageKey(identity);
      note += "页面私钥已切换为新密钥";
      $("rotate-identity").value = "";
    } else {
      note += "请把当前私钥切换为新密钥后再查看密文";
    }
    $("rotate-status").textContent = note;
    $("rotate-recipient").value = "";
    state.catalog = await api("/api/catalog");
    renderKeyStatus();
    if (state.filename) await loadFile();
  } catch (error) { $("rotate-status").textContent = error.message; }
  finally { $("rotate-run").disabled = false; }
};

/* ---------- 控件状态 ---------- */

function controls() {
  const unavailable = state.file?.kind === "secrets" && !state.catalog.secrets_ready;
  $("rail-dirty").hidden = !state.dirty;
  $("save").disabled = state.busy || unavailable || !state.file || !state.dirty;
  $("reload").disabled = state.busy || !state.filename;
  $("save-status").textContent = state.busy ? "正在处理…" : state.dirty ? "有未保存的更改" : "所有更改已保存";
  $("busy-bar").hidden = !state.busy && !state.loading;
  document.querySelectorAll("#services button, #environments button, #files button, #secret-editor button, #secret-editor input, #secret-editor textarea, #config-content, #open-key-modal").forEach((node) => { node.disabled = state.busy; });
  if (unavailable) document.querySelectorAll("#secret-editor button, #secret-editor input, #secret-editor textarea").forEach((node) => { node.disabled = true; });
  $("editor-card").setAttribute("aria-busy", String(state.busy));
  updateFileTabDirty();
  renderKeyStatus();
}

function updateFileTabDirty() {
  document.querySelectorAll("#files .file-tab").forEach((tab) => {
    tab.classList.toggle("dirty", tab.dataset.filename === state.filename && state.dirty);
  });
}

function dirty() { state.dirty = true; controls(); }

async function canLeave() {
  if (!state.busy && !state.dirty) return true;
  if (state.busy) return false;
  return await confirmDialog("当前更改尚未保存，确定放弃并切换吗？", { title: "未保存的更改", acceptText: "放弃更改" });
}

function selectedService() { return state.catalog.services.find((item) => item.name === state.service); }
function selectedEnvironment() { return selectedService()?.environments.find((item) => item.name === state.environment); }

/* ---------- 服务目录 / 环境 / 文件 ---------- */

function renderServices() {
  const query = $("service-search").value.trim().toLowerCase();
  $("clear-search").hidden = !query;
  $("services").replaceChildren();
  const services = state.catalog.services.filter((item) => (item.name || "（根目录）").toLowerCase().includes(query));
  for (const service of services) {
    const button = element("button", "service-button" + (service.name === state.service ? " active" : ""));
    button.setAttribute("aria-pressed", String(service.name === state.service));
    button.append(element("span", "service-icon", "◇"), element("span", "", service.name || "（根目录）"), element("span", "arrow", "›"));
    button.onclick = async () => {
      if (!(await canLeave())) return;
      state.service = service.name;
      state.environment = service.environments.some((env) => env.name === state.environment) ? state.environment : service.environments[0].name;
      chooseFile(); renderNavigation(); await loadFile();
    };
    $("services").append(button);
  }
  const empty = element("div", "services-empty");
  empty.textContent = services.length ? "" : "没有匹配的服务";
  if (!services.length) $("services").append(empty);
}

function chooseFile() {
  const files = selectedEnvironment()?.files || [];
  state.filename = (files.find((file) => file.kind === "secrets") || files[0])?.name || "";
}

function renderNavigation() {
  renderServices();
  $("service-title").textContent = state.service || "服务配置";
  $("topbar-path").textContent = [state.service, state.environment].filter(Boolean).join("/") || "—";

  $("environments").replaceChildren();
  for (const environment of selectedService()?.environments || []) {
    const button = element("button", environment.name === state.environment ? "active" : "", environment.name || "（根目录）");
    button.setAttribute("aria-pressed", String(environment.name === state.environment));
    button.onclick = async () => {
      if (!(await canLeave())) return;
      state.environment = environment.name;
      chooseFile(); renderNavigation(); await loadFile();
    };
    $("environments").append(button);
  }

  $("files").replaceChildren();
  for (const file of selectedEnvironment()?.files || []) {
    const button = element("button", "file-tab" + (file.name === state.filename ? " active" : ""));
    button.dataset.filename = file.name;
    button.append(element("span", "file-kind", file.kind === "secrets" ? "环境密钥" : "运行配置"));
    button.append(element("span", "file-hint", file.kind === "secrets" ? (file.exists ? "私密" : "待添加") : file.name));
    button.append(element("span", "file-dot"));
    button.setAttribute("aria-pressed", String(file.name === state.filename));
    button.onclick = async () => {
      if (!(await canLeave())) return;
      state.filename = file.name; renderNavigation(); await loadFile();
    };
    $("files").append(button);
  }
  updateFileTabDirty();
}

/* ---------- 密钥编辑 ---------- */

async function copyText(text, label) {
  if (!text) { toast("没有可复制的内容", "info"); return; }
  try {
    await navigator.clipboard.writeText(text);
    toast("已复制" + (label ? "：" + label : ""));
  } catch { toast("复制失败，请手动选中", "error"); }
}

function allRevealed() { return state.rows.length > 0 && state.rows.every((row) => row.revealed); }

function renderRows() {
  const query = $("key-search").value.trim().toUpperCase();
  const visible = state.rows.filter((row) => row.key.toUpperCase().includes(query));
  $("toggle-reveal").textContent = allRevealed() && state.rows.length ? "全部隐藏" : "全部显示";
  $("secret-rows").replaceChildren();
  for (const row of visible) {
    const line = element("div", "secret-row");

    const keyWrap = element("div", "key-wrap");
    const key = element("textarea", "key-input");
    key.rows = 1;
    key.value = row.key; key.spellcheck = false; key.autocomplete = "off";
    key.title = row.key;
    key.placeholder = "例如 LINMI_ADMIN_DB_PASSWORD";
    key.setAttribute("aria-label", "密钥名称");
    key.onkeydown = (event) => { if (event.key === "Enter") event.preventDefault(); };
    key.oninput = () => { row.key = key.value; dirty(); };
    keyWrap.append(key, element("div", "key-caption", row.value ? "已配置" : "空值"));
    keyWrap.querySelector(".key-caption").append(renderCopy(key.value ? () => copyText(row.key, row.key) : null));

    const valueWrap = element("div", "value-wrap");
    if (row.revealed) {
      const value = element("textarea");
      value.value = row.value; value.rows = Math.min(5, Math.max(1, row.value.split("\n").length));
      value.spellcheck = false; value.autocomplete = "off";
      value.placeholder = "输入配置值，支持多行";
      value.setAttribute("aria-label", (row.key || "新密钥") + "的值");
      value.oninput = () => { row.value = value.value; dirty(); };
      valueWrap.append(value);
    } else {
      const masked = element("span", "masked-value" + (row.value ? "" : " empty"), row.value ? "••••••••••••" : "尚未填写");
      masked.title = "点击查看";
      masked.onclick = () => { row.revealed = true; renderRows(); };
      valueWrap.append(masked);
      valueWrap.append(renderCopy(() => copyText(row.value, row.key || "该值")));
    }

    const actions = element("div", "row-actions");
    const reveal = element("button", "text-button", row.revealed ? "隐藏" : "查看 / 修改");
    reveal.setAttribute("aria-expanded", String(row.revealed));
    reveal.onclick = () => { row.revealed = !row.revealed; renderRows(); };
    const remove = element("button", "text-button danger delete-button", "×");
    remove.setAttribute("aria-label", "删除 " + (row.key || "新密钥"));
    remove.onclick = async () => {
      const proceed = await confirmDialog("确定删除「" + (row.key || "这个新密钥") + "」吗？保存后生效", { title: "删除密钥", acceptText: "删除" });
      if (!proceed) return;
      state.rows = state.rows.filter((item) => item.id !== row.id); dirty(); renderRows();
    };
    actions.append(reveal, remove);
    line.append(keyWrap, valueWrap, actions);
    $("secret-rows").append(line);
  }
  $("item-count").textContent = state.rows.length + " 项";
  $("no-keys").hidden = visible.length > 0;
  $("no-keys").querySelector("h3").textContent = query ? "没有匹配的密钥" : "还没有密钥";
  $("no-keys").querySelector("p").textContent = query ? "试试其他名称" : "点击「添加密钥」，填写名称和对应值即可";
  controls();
}

function renderCopy(handler) {
  const button = element("button", "text-button copy-button", "⧉");
  button.title = "复制";
  if (handler) { button.setAttribute("aria-label", "复制"); button.onclick = handler; }
  else button.disabled = true;
  return button;
}

/* ---------- 文件加载与保存 ---------- */

async function loadFile() {
  state.busy = true; state.file = null; state.rows = []; state.dirty = false;
  $("secret-editor").hidden = true; $("config-editor").hidden = true;
  $("editor-footer").hidden = true; $("item-count").hidden = true;
  $("empty").hidden = false; $("empty").querySelector("h3").textContent = "正在加载配置";
  $("empty").querySelector("p").textContent = "请稍候";
  $("config-content").value = ""; $("secret-rows").replaceChildren();
  controls();
  try {
    state.file = await api("/api/file?" + new URLSearchParams({ service: state.service, environment: state.environment, filename: state.filename }));
    $("empty").hidden = true;
    $("editor-footer").hidden = false;
    $("file-path").textContent = [state.service, state.environment, state.filename].filter(Boolean).join("/");
    const isSecret = state.file.kind === "secrets";
    $("editor-title").textContent = isSecret ? "环境密钥" : "运行配置";
    $("editor-description").textContent = isSecret ? "直接编辑对应值，保存时自动加密，无需手动转换" : "编辑运行参数，保存前会自动检查配置格式";
    $("secret-editor").hidden = !isSecret; $("config-editor").hidden = isSecret;
    $("item-count").hidden = !isSecret;
    if (isSecret) {
      state.rows = Object.entries(state.file.values).map(([key, value]) => ({ id: ++rowId, key, value, revealed: false }));
      $("key-search").value = ""; renderRows();
      if (!state.catalog.secrets_ready) toast("查看与保存密钥需要 age 私钥 —— 点左下角状态条或右侧「设置」", "info");
    } else $("config-content").value = state.file.content;
  } catch (error) {
    state.file = null;
    $("empty").querySelector("h3").textContent = "暂时无法读取配置";
    $("empty").querySelector("p").textContent = "可切换其他配置，或重新加载";
    toast(error.message, "error");
  } finally { state.busy = false; controls(); }
}

async function save() {
  if (!state.file || state.busy || !state.dirty) return;
  const payload = { service: state.service, environment: state.environment, filename: state.filename, revision: state.file.revision };
  if (state.file.kind === "secrets") {
    payload.values = Object.create(null);
    for (const row of state.rows) {
      if (!/^[A-Z_][A-Z0-9_]*$/.test(row.key)) { toast("请检查密钥名称，只能使用大写字母、数字和下划线，不能以数字开头", "error"); return; }
      if (Object.hasOwn(payload.values, row.key)) { toast("存在重复的密钥名称：" + row.key, "error"); return; }
      payload.values[row.key] = row.value;
    }
  } else payload.content = $("config-content").value;
  state.busy = true; controls();
  try {
    const result = await api("/api/save", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(payload) });
    state.file.revision = result.revision; state.dirty = false;
    const file = selectedEnvironment().files.find((item) => item.name === state.filename);
    if (file) file.exists = true;
    renderNavigation();
    state.rows.forEach((row) => { row.revealed = false; });
    if (state.file.kind === "secrets") renderRows();
    toast(result.message);
  } catch (error) { toast(error.message, "error"); }
  finally { state.busy = false; controls(); }
}

/* ---------- 事件绑定 ---------- */

$("service-search").oninput = () => { if (state.catalog) { renderServices(); controls(); } };
$("clear-search").onclick = () => { $("service-search").value = ""; renderServices(); controls(); $("service-search").focus(); };
$("service-search").onkeydown = (event) => { if (event.key === "Escape") $("clear-search").click(); };
$("key-search").oninput = renderRows;
$("key-search").onkeydown = (event) => { if (event.key === "Escape") { $("key-search").value = ""; renderRows(); } };

$("toggle-reveal").onclick = () => {
  const reveal = !allRevealed() || !state.rows.length;
  state.rows.forEach((row) => { row.revealed = reveal; });
  renderRows();
};

$("config-content").oninput = dirty;
$("config-content").onkeydown = (event) => {
  if (event.key !== "Tab") return;
  event.preventDefault();
  const input = event.target;
  input.setRangeText("  ", input.selectionStart, input.selectionEnd, "end"); dirty();
};

$("add-key").onclick = () => {
  state.rows.push({ id: ++rowId, key: "", value: "", revealed: true });
  $("key-search").value = ""; dirty(); renderRows();
  const last = $("secret-rows").lastElementChild;
  if (last) last.querySelector(".key-input").focus();
};

$("reload").onclick = async () => { if (await canLeave()) loadFile(); };
$("save").onclick = save;

window.addEventListener("beforeunload", (event) => { if (state.dirty) { event.preventDefault(); event.returnValue = ""; } });
window.addEventListener("keydown", (event) => {
  if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === "s") { event.preventDefault(); save(); }
  if (event.key === "Escape") {
    if (!$("confirm-modal").hidden) settleConfirm(false);
    if (!$("settings-drawer").hidden) closeSettings();
  }
});

/* ---------- 启动 ---------- */

(async () => {
  try {
    state.catalog = await api("/api/catalog");
    $("repository-name").textContent = state.catalog.repository;
    $("service-count").textContent = state.catalog.services.length;
    renderKeyStatus();
    if (!state.catalog.services.length) {
      $("empty").querySelector("h3").textContent = "当前仓库还没有配置文件";
      $("empty").querySelector("p").textContent = "请先在仓库里放置 YAML / JSON 配置（目录层级不限）";
      return;
    }
    state.service = state.catalog.services[0].name;
    state.environment = selectedService().environments[0].name;
    chooseFile(); renderNavigation(); await loadFile();
  } catch (error) { toast(error.message, "error"); }
})();
