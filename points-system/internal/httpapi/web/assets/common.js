"use strict";

(() => {
  const session = { csrf: "" };
  const requestTimeoutMs = 20_000;
  let noticeTimer = 0;

  const embedded = new URLSearchParams(window.location.search).get("ui_mode") === "embedded";
  document.body.dataset.uiMode = embedded ? "embedded" : "standalone";
  document.documentElement.dataset.uiMode = document.body.dataset.uiMode;
  const parentOrigin = document.querySelector('meta[name="sub2api-parent-origin"]')?.content || "";
  let readySent = false;
  let embeddedTheme = "";

  function applyTheme(theme) {
    if (theme !== "light" && theme !== "dark") return;
    const changed = document.documentElement.dataset.theme !== theme;
    document.documentElement.dataset.theme = theme;
    if (changed) {
      window.dispatchEvent(new CustomEvent("points:themechange", { detail: { theme } }));
    }
  }

  function applyEmbeddedTheme(event) {
    if (!embedded || window.parent === window || !parentOrigin) return;
    if (event.source !== window.parent || event.origin !== parentOrigin) return;
    if (event.data?.type !== "sub2api:points-theme") return;
    if (event.data.theme !== "light" && event.data.theme !== "dark") return;
    embeddedTheme = event.data.theme;
    applyTheme(embeddedTheme);
  }

  window.addEventListener("message", applyEmbeddedTheme);

  document.querySelectorAll("[data-brand-logo]").forEach((image) => {
    image.addEventListener("error", () => {
      if (image.dataset.fallbackApplied === "true") return;
      image.dataset.fallbackApplied = "true";
      image.src = "/assets/logo.svg";
    }, { once: true });
  });

  function notifyReady() {
    if (readySent || !embedded || window.parent === window) return;
    readySent = true;
    window.parent.postMessage({
      type: "sub2api:points-ready",
      role: document.body.classList.contains("admin-shell") ? "admin" : "user"
    }, parentOrigin || "*");
  }

  const errorMessages = {
    unauthorized: "登录状态已失效",
    forbidden: "当前账户无权访问",
    points_disabled: "积分功能暂未开放",
    origin_mismatch: "请求来源校验失败",
    csrf_invalid: "页面凭证已失效，请重新进入",
    rate_limited: "操作过于频繁，请稍后再试",
    request_timeout: "请求超时，请检查网络后重试",
    invalid_request: "提交内容不完整或格式不正确",
    invalid_cursor: "分页状态已失效，请刷新后重试",
    invalid_effective_date: "策略生效日期不正确",
    effective_date_must_be_tomorrow: "策略只能于下一自然日生效",
    invalid_business_date: "业务日期必须早于今天",
    idempotency_required: "请求标识生成失败，请重试",
    idempotency_conflict: "该操作已提交，请刷新查看结果",
    policy_incomplete: "策略配置不完整",
    snapshot_not_ready: "昨日积分仍在结算中",
    business_rule: "当前条件不满足业务规则",
    not_found: "请求的记录不存在",
    internal_error: "服务暂时异常，请稍后再试"
  };

  const statusMessages = {
    pending: "待处理",
    processing: "处理中",
    failed: "可重试失败",
    permanently_failed: "处理失败",
    settled: "已到账",
    reversal_pending: "待冲正",
    reversal_processing: "冲正中",
    reversal_permanently_failed: "冲正失败",
    reversed: "已冲正",
    ready: "已就绪",
    complete: "已完成",
    completed: "已完成",
    success: "成功",
    empty: "无消费",
    needs_review: "待复核",
    disabled: "未启用",
    missing: "未生成",
    rejected: "未通过"
  };

  const kindMessages = {
    usage_points: "消费积分",
    checkin: "签到赠送",
    reversal: "冲正"
  };

  function byId(id) {
    return document.getElementById(id);
  }

  function icon(name, extraClass = "") {
    const node = document.createElementNS("http://www.w3.org/2000/svg", "svg");
    node.setAttribute("class", `icon ${extraClass}`.trim());
    node.setAttribute("aria-hidden", "true");
    const use = document.createElementNS("http://www.w3.org/2000/svg", "use");
    use.setAttribute("href", `/assets/lucide-sprite.svg#${name}`);
    node.append(use);
    return node;
  }

  function number(value) {
    const result = Number(value || 0);
    return Number.isFinite(result) ? result : 0;
  }

  function points(hundredths) {
    return (number(hundredths) / 100).toLocaleString("zh-CN", {
      minimumFractionDigits: 2,
      maximumFractionDigits: 2
    });
  }

  function money(microUSD) {
    return `${(number(microUSD) / 1_000_000).toLocaleString("zh-CN", {
      minimumFractionDigits: 2,
      maximumFractionDigits: 2
    })} U`;
  }

  function date(value) {
    if (!value) return "-";
    const parsed = new Date(value);
    return Number.isNaN(parsed.getTime()) ? "-" : parsed.toLocaleDateString("zh-CN");
  }

  function shortDate(value) {
    if (!value) return "-";
    const parsed = new Date(value);
    if (Number.isNaN(parsed.getTime())) return "-";
    return `${parsed.getMonth() + 1}月${parsed.getDate()}日`;
  }

  function dateTime(value) {
    if (!value) return "-";
    const parsed = new Date(value);
    return Number.isNaN(parsed.getTime()) ? "-" : parsed.toLocaleString("zh-CN", {
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit"
    });
  }

  function plain(value) {
    return value == null || value === "" ? "-" : String(value);
  }

  function statusText(value) {
    return statusMessages[value] || plain(value);
  }

  function kindText(value) {
    return kindMessages[value] || plain(value);
  }

  function statusClass(value) {
    if (["settled", "ready", "complete", "completed", "success"].includes(value)) return "success";
    if (["pending", "processing", "reversal_pending", "reversal_processing", "needs_review"].includes(value)) return "warning";
    if (["failed", "permanently_failed", "reversal_permanently_failed", "rejected"].includes(value)) return "danger";
    if (value === "reversed") return "neutral";
    return "neutral";
  }

  function statusChip(value) {
    const node = document.createElement("span");
    node.className = `status-chip ${statusClass(value)}`;
    node.textContent = statusText(value);
    return node;
  }

  function idempotencyKey() {
    if (typeof crypto.randomUUID === "function") return crypto.randomUUID();
    const bytes = crypto.getRandomValues(new Uint8Array(16));
    return Array.from(bytes, (value) => value.toString(16).padStart(2, "0")).join("");
  }

  function setSession(data) {
    session.csrf = data?.csrf_token || "";
    const sessionTheme = data?.theme === "dark" ? "dark" : "light";
    applyTheme(embedded && embeddedTheme ? embeddedTheme : sessionTheme);
    document.documentElement.lang = "zh-CN";
  }

  async function api(path, options = {}) {
    const headers = new Headers(options.headers || {});
    const method = String(options.method || "GET").toUpperCase();
    const controller = new AbortController();
    const externalSignal = options.signal;
    let timedOut = false;
    const abortFromExternal = () => controller.abort(externalSignal?.reason);
    if (externalSignal?.aborted) abortFromExternal();
    else externalSignal?.addEventListener("abort", abortFromExternal, { once: true });
    const timeout = window.setTimeout(() => {
      timedOut = true;
      controller.abort();
    }, requestTimeoutMs);
    if (options.body) headers.set("Content-Type", "application/json");
    if (method !== "GET" && method !== "HEAD") headers.set("X-CSRF-Token", session.csrf);
    try {
      const response = await fetch(path, {
        ...options,
        method,
        headers,
        credentials: "same-origin",
        signal: controller.signal
      });
      const body = await response.json().catch(() => ({}));
      if (!response.ok || body.error) {
        const code = body.error?.code || "";
        const error = new Error(errorMessages[code] || `请求未完成（${response.status}）`);
        error.code = code;
        error.status = response.status;
        throw error;
      }
      return body.data;
    } catch (error) {
      if (!timedOut || error?.name !== "AbortError") throw error;
      const timeoutError = new Error(errorMessages.request_timeout);
      timeoutError.code = "request_timeout";
      timeoutError.status = 0;
      throw timeoutError;
    } finally {
      window.clearTimeout(timeout);
      externalSignal?.removeEventListener("abort", abortFromExternal);
    }
  }

  function notice(message, isError = false) {
    const node = byId("notice");
    if (!node) return;
    window.clearTimeout(noticeTimer);
    node.textContent = message;
    node.classList.toggle("error", isError);
    node.setAttribute("role", isError ? "alert" : "status");
    node.setAttribute("aria-live", isError ? "assertive" : "polite");
    node.classList.remove("hidden");
    noticeTimer = window.setTimeout(() => node.classList.add("hidden"), isError ? 8000 : 5000);
  }

  function renderRows(target, rows, columns, emptyMessage = "暂无记录") {
    const body = byId(target);
    if (!body) return;
    body.replaceChildren();
    const items = Array.isArray(rows) ? rows : [];
    if (items.length === 0) {
      const row = document.createElement("tr");
      const cell = document.createElement("td");
      cell.colSpan = columns.length;
      cell.className = "empty-cell";
      cell.textContent = emptyMessage;
      row.append(cell);
      body.append(row);
      return;
    }
    items.forEach((item) => {
      const row = document.createElement("tr");
      columns.forEach((column) => {
        const cell = document.createElement("td");
        const value = typeof column === "function" ? column(item, cell) : item[column];
        if (value instanceof Node) cell.append(value);
        else if (value !== undefined) cell.textContent = plain(value);
        row.append(cell);
      });
      body.append(row);
    });
  }

  function setButtonLabel(button, text) {
    if (!button) return;
    const label = button.querySelector("[data-button-label]");
    if (label) label.textContent = text;
    else button.textContent = text;
  }

  function buttonLabel(button) {
    const label = button?.querySelector("[data-button-label]");
    return label ? label.textContent : button?.textContent || "";
  }

  function setButtonBusy(button, busy, busyText = "处理中") {
    if (!button) return;
    if (busy) {
      button.dataset.label = buttonLabel(button);
      setButtonLabel(button, busyText);
      button.disabled = true;
      button.setAttribute("aria-busy", "true");
      button.classList.add("is-busy");
      return;
    }
    if (button.dataset.label) setButtonLabel(button, button.dataset.label);
    delete button.dataset.label;
    button.disabled = false;
    button.removeAttribute("aria-busy");
    button.classList.remove("is-busy");
  }

  async function logout() {
    await api("/api/v1/logout", { method: "POST" });
    window.location.replace("/");
  }

  window.PointsUI = Object.freeze({
    api,
    byId,
    date,
    dateTime,
    icon,
    idempotencyKey,
    kindText,
    logout,
    money,
    notifyReady,
    notice,
    number,
    plain,
    points,
    renderRows,
    setButtonBusy,
    setButtonLabel,
    setSession,
    shortDate,
    statusChip,
    statusClass,
    statusText
  });
})();
