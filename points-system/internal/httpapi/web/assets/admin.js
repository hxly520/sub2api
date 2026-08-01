"use strict";

(() => {
  const ui = window.PointsUI;
  const state = {
    policies: [],
    grants: [],
    grantSummary: {},
    users: [],
    usersPage: { limit: 50, offset: 0, total: 0, loading: false },
    grantsPage: { cursor: "", nextCursor: "", backPages: [], forwardPages: [], loading: false },
    requestSequence: { policies: 0, users: 0, grants: 0 },
    reverseID: "",
    reverseReturnFocus: null,
    businessDate: "",
    nextPolicyDate: "",
    view: "overview"
  };
  const viewTitles = {
    overview: "运行总览",
    users: "用户明细",
    policies: "策略管理",
    operations: "签到发放记录"
  };
  let adminInitialized = false;
  let bootstrapInFlight = false;

  function setAdminSyncState(state, label) {
    const syncMark = ui.byId("admin-sync-mark");
    syncMark.dataset.state = state;
    syncMark.title = label;
    syncMark.setAttribute("aria-label", label);
  }

  function setAccessState(message, { error = false, retry = false } = {}) {
    const access = ui.byId("admin-access-state");
    access.classList.toggle("error", error);
    access.setAttribute("role", error ? "alert" : "status");
    ui.byId("admin-access-message").textContent = message;
    ui.byId("retry-admin-bootstrap").hidden = !retry;
  }

  function initializeAdminWorkspace() {
    if (adminInitialized) return;
    bindEvents();
    setPolicyEffectiveDate();
    addTier();
    syncConsumerOnly();
    syncCheckinControls();
    adminInitialized = true;
  }

  function localDate(daysAhead) {
    const value = new Date();
    value.setHours(12, 0, 0, 0);
    value.setDate(value.getDate() + daysAhead);
    const year = value.getFullYear();
    const month = String(value.getMonth() + 1).padStart(2, "0");
    const day = String(value.getDate()).padStart(2, "0");
    return `${year}-${month}-${day}`;
  }

  function refreshTime(minute) {
    const value = Math.max(0, ui.number(minute));
    return `${String(Math.floor(value / 60)).padStart(2, "0")}:${String(value % 60).padStart(2, "0")}`;
  }

  function setPolicyEffectiveDate() {
    const tomorrow = serviceDate(1);
    ui.byId("policy-date").value = tomorrow;
    ui.byId("policy-editor-effective").textContent = tomorrow;
    return tomorrow;
  }

  function serviceDate(daysAhead) {
    if (daysAhead === 1 && /^\d{4}-\d{2}-\d{2}$/.test(state.nextPolicyDate)) {
      return state.nextPolicyDate;
    }
    if (!/^\d{4}-\d{2}-\d{2}$/.test(state.businessDate)) return localDate(daysAhead);
    const value = new Date(`${state.businessDate}T12:00:00Z`);
    value.setUTCDate(value.getUTCDate() + daysAhead);
    return value.toISOString().slice(0, 10);
  }

  function policyDate(policy) {
    return typeof policy?.effective_date === "string" ? policy.effective_date.slice(0, 10) : "";
  }

  function policyForDate(date) {
    return state.policies.reduce((selected, policy) => {
      const candidateDate = policyDate(policy);
      if (!candidateDate || candidateDate > date) return selected;
      if (!selected) return policy;
      const selectedDate = policyDate(selected);
      if (candidateDate > selectedDate) return policy;
      if (candidateDate === selectedDate && ui.number(policy.version_no) > ui.number(selected.version_no)) return policy;
      return selected;
    }, null);
  }

  function setView(name) {
    if (!viewTitles[name]) return;
    state.view = name;
    document.querySelectorAll(".admin-view").forEach((view) => view.classList.add("hidden"));
    document.querySelectorAll(".admin-nav-button").forEach((button) => {
      const selected = button.dataset.view === name;
      button.classList.toggle("active", selected);
      button.setAttribute("aria-selected", String(selected));
      button.tabIndex = selected ? 0 : -1;
    });
    ui.byId(`view-${name}`).classList.remove("hidden");
    ui.byId("admin-page-title").textContent = viewTitles[name];
  }

  function currentPolicy() {
    return policyForDate(serviceDate(0)) || state.policies[0] || null;
  }

  function editorPolicy() {
    return policyForDate(serviceDate(1)) || currentPolicy();
  }

  function renderOverview() {
    const policy = currentPolicy();
    ui.byId("overview-version").textContent = policy ? `v${policy.version_no}` : "-";
    ui.byId("overview-points").textContent = policy?.enabled ? "已启用" : "未启用";
    ui.byId("overview-checkin").textContent = policy?.checkin_enabled ? "已启用" : "未启用";
    ui.byId("overview-effective").textContent = ui.date(policy?.effective_date);
    ui.byId("overview-ratio").textContent = policy ? `${ui.points(policy.points_per_usd_hundredths)} 积分 / U` : "-";
    ui.byId("overview-refresh").textContent = policy ? refreshTime(policy.refresh_minute) : "-";
    ui.byId("overview-audience").textContent = policy
      ? policy.mode === "consumer_only" ? "昨日消费用户" : "全部用户"
      : "-";
    ui.byId("overview-basis").textContent = policy
      ? policy.basis === "total" ? "总积分" : "昨日积分"
      : "-";
    ui.byId("overview-tiers").textContent = String(policy?.tiers?.length || 0);
    ui.byId("overview-points-card").dataset.state = policy?.enabled ? "enabled" : "disabled";
    ui.byId("overview-checkin-card").dataset.state = policy?.checkin_enabled ? "enabled" : "disabled";

    const pendingStatuses = new Set(["pending", "processing", "reversal_pending", "reversal_processing"]);
    const statusCount = (status) => ui.number(state.grantSummary[status]);
    const pendingCount = [...pendingStatuses].reduce((total, status) => total + statusCount(status), 0);
    ui.byId("overview-pending").textContent = String(pendingCount);
    ui.byId("overview-pending-card").dataset.state = pendingCount > 0 ? "warning" : "neutral";
    const summary = ui.byId("grant-status-summary");
    summary.replaceChildren();
    const groups = [
      ["待处理", ["pending", "processing"]],
      ["已到账", ["settled"]],
      ["失败", ["failed", "permanently_failed", "reversal_permanently_failed"]],
      ["冲正中", ["reversal_pending", "reversal_processing"]],
      ["已冲正", ["reversed"]]
    ];
    groups.forEach(([label, statuses]) => {
      const item = document.createElement("div");
      const name = document.createElement("span");
      const count = document.createElement("strong");
      name.textContent = label;
      count.textContent = String(statuses.reduce((total, status) => total + statusCount(status), 0));
      item.dataset.state = statuses.includes("settled") ? "enabled"
        : statuses.some((status) => status.includes("failed")) ? "danger"
          : statuses.some((status) => status.includes("pending") || status.includes("processing")) ? "warning"
            : "neutral";
      item.append(name, count);
      summary.append(item);
    });
  }

  async function loadPolicies() {
    const requestSequence = ++state.requestSequence.policies;
    const rows = await ui.api("/api/v1/admin/policies?limit=50");
    if (requestSequence !== state.requestSequence.policies) return;
    state.policies = Array.isArray(rows) ? rows : [];
    renderPolicyEditor(editorPolicy());
    ui.renderRows("policies-body", state.policies, [
      (policy) => `v${policy.version_no}`,
      (policy) => ui.date(policy.effective_date),
      (policy) => policy.enabled ? "启用" : "关闭",
      (policy) => policy.checkin_enabled ? `每日 ${policy.checkin_daily_limit || 0} 次` : "关闭",
      (policy) => policy.mode === "consumer_only" ? "消费用户" : "全部用户",
      (policy) => policy.basis === "total" ? "总积分" : "昨日积分",
      (policy) => ui.points(policy.points_per_usd_hundredths),
      (policy) => refreshTime(policy.refresh_minute),
      (policy) => ui.money(policy.minimum_checkin_spend_microusd),
      (policy) => `${policy.tiers?.length || 0} 档`
    ], "暂无历史策略记录");
    renderOverview();
  }

  function decimalValue(value, divisor, fallback) {
    const numeric = Number(value);
    if (!Number.isFinite(numeric)) return fallback;
    return (numeric / divisor).toFixed(2);
  }

  function renderPolicyEditor(policy) {
    setPolicyEffectiveDate();
    ui.byId("policy-editor-source").textContent = policy
      ? `v${policy.version_no}${policyDate(policy) === serviceDate(1) ? "（已排期）" : "（当前生效）"}`
      : "默认配置";
    if (!policy) return;

    ui.byId("policy-points-rate").value = decimalValue(policy.points_per_usd_hundredths, 100, "10.00");
    ui.byId("policy-refresh-time").value = refreshTime(policy.refresh_minute);
    ui.byId("policy-enabled").checked = Boolean(policy.enabled);
    ui.byId("policy-checkin-enabled").checked = Boolean(policy.checkin_enabled);
    ui.byId("policy-consumer-only").checked = policy.mode === "consumer_only";
    ui.byId("policy-basis").value = policy.basis === "total" ? "total" : "yesterday";
    ui.byId("policy-checkin-limit").value = String(policy.checkin_daily_limit || 1);
    ui.byId("policy-minimum-spend").value = decimalValue(policy.minimum_checkin_spend_microusd, 1_000_000, "0.00");
    ui.byId("policy-single-cap").value = decimalValue(policy.checkin_single_award_cap_microusd, 1_000_000, "1.00");
    ui.byId("policy-user-cap").value = decimalValue(policy.checkin_user_daily_cap_microusd, 1_000_000, "1.00");
    ui.byId("policy-platform-cap").value = decimalValue(policy.checkin_platform_daily_cap_microusd, 1_000_000, "100.00");

    const tiers = ui.byId("tiers");
    tiers.replaceChildren();
    if (Array.isArray(policy.tiers) && policy.tiers.length > 0) policy.tiers.forEach(addTier);
    else addTier();
    syncCheckinControls();
  }

  function syncAdminUsersPager() {
    const page = state.usersPage;
    ui.byId("admin-users-prev").disabled = page.loading || page.offset === 0;
    ui.byId("admin-users-next").disabled = page.loading || page.offset + state.users.length >= page.total;
  }

  async function loadAdminUsers({ offset = state.usersPage.offset } = {}) {
    const page = state.usersPage;
    const requestSequence = ++state.requestSequence.users;
    const requestedLimit = page.limit;
    const requestedOffset = Math.max(0, ui.number(offset));
    page.loading = true;
    syncAdminUsersPager();
    try {
      const data = await ui.api(`/api/v1/admin/users/points?limit=${requestedLimit}&offset=${requestedOffset}`);
      if (requestSequence !== state.requestSequence.users) return;
      state.users = Array.isArray(data?.items) ? data.items : [];
      page.total = Math.max(0, ui.number(data?.total));
      page.limit = Math.max(1, ui.number(data?.limit) || page.limit);
      page.offset = Math.max(0, ui.number(data?.offset ?? requestedOffset));
      ui.renderRows("admin-users-body", state.users, [
        (user) => user.login_email || "未设置登录邮箱",
        (user) => ui.points(user.total_points_hundredths),
        (user) => ui.points(user.yesterday_points_hundredths),
        (user) => ui.money(user.total_spend_microusd),
        (user) => ui.money(user.yesterday_spend_microusd),
        (user) => ui.date(user.snapshot_business_date),
        (user) => ui.statusChip(user.snapshot_status)
      ], "暂无用户积分账户");

      const first = page.total === 0 ? 0 : page.offset + 1;
      const last = Math.min(page.total, page.offset + state.users.length);
      ui.byId("admin-users-page-summary").textContent = page.total === 0
        ? "共 0 位用户"
        : `第 ${first}-${last} 位，共 ${page.total} 位用户`;
    } finally {
      if (requestSequence === state.requestSequence.users) {
        page.loading = false;
        syncAdminUsersPager();
      }
    }
  }

  function actionButton(label, className, iconName, handler) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = `button ${className} table-action`;
    const text = document.createElement("span");
    text.dataset.buttonLabel = "";
    text.textContent = label;
    button.append(ui.icon(iconName), text);
    button.addEventListener("click", handler);
    return button;
  }

  function syncAdminGrantsPager() {
    const page = state.grantsPage;
    ui.byId("admin-grants-page").textContent = `第 ${page.backPages.length + 1} 页`;
    ui.byId("admin-grants-prev").disabled = page.loading || page.backPages.length === 0;
    ui.byId("admin-grants-next").disabled = page.loading ||
      (page.forwardPages.length === 0 && !page.nextCursor);
  }

  function renderAdminGrants() {
    ui.renderRows("admin-grants-body", state.grants, [
      (grant) => ui.dateTime(grant.created_at),
      (grant) => grant.login_email || "未设置登录邮箱",
      (grant) => ui.money(grant.amount_microusd),
      (grant) => ui.kindText(grant.kind),
      (grant) => ui.statusChip(grant.status),
      "attempts",
      (grant) => grant.last_error || "-",
      (grant, cell) => {
        cell.className = "actions";
        const retryable = ["failed", "permanently_failed", "reversal_permanently_failed"].includes(grant.status);
        if (retryable) cell.append(actionButton("重试", "secondary", "refresh-cw", (event) => retryGrant(grant.id, event.currentTarget)));
        const reversible = grant.status === "settled" || (grant.status === "pending" && grant.attempts === 0);
        if (reversible) cell.append(actionButton("冲正", "danger", "rotate-ccw", (event) => openReverseDialog(grant.id, event.currentTarget)));
        if (!retryable && !reversible) cell.textContent = "-";
      }
    ], "暂无赠送任务");
    renderOverview();
  }

  function adminGrantPageSnapshot() {
    return {
      cursor: state.grantsPage.cursor,
      items: state.grants,
      nextCursor: state.grantsPage.nextCursor
    };
  }

  function applyAdminGrantPage(snapshot) {
    state.grantsPage.cursor = snapshot.cursor;
    state.grantsPage.nextCursor = snapshot.nextCursor;
    state.grants = snapshot.items;
    renderAdminGrants();
    syncAdminGrantsPager();
  }

  async function loadAdminGrants({ cursor = state.grantsPage.cursor, navigation = "replace" } = {}) {
    const page = state.grantsPage;
    const requestSequence = ++state.requestSequence.grants;
    page.loading = true;
    syncAdminGrantsPager();
    try {
      const [data, summary] = await Promise.all([
        ui.api(`/api/v1/admin/balance-grants?cursor=${encodeURIComponent(cursor)}`),
        ui.api("/api/v1/admin/balance-grants/summary")
      ]);
      if (requestSequence !== state.requestSequence.grants) return;
      const previous = adminGrantPageSnapshot();
      if (navigation === "next") {
        page.backPages.push(previous);
        page.forwardPages = [];
      } else if (navigation === "reset") {
        page.backPages = [];
        page.forwardPages = [];
      } else {
        page.forwardPages = [];
      }
      state.grantSummary = summary?.counts && typeof summary.counts === "object" ? summary.counts : {};
      applyAdminGrantPage({
        cursor,
        items: Array.isArray(data?.items) ? data.items : [],
        nextCursor: typeof data?.next_cursor === "string" ? data.next_cursor : ""
      });
    } finally {
      if (requestSequence === state.requestSequence.grants) {
        page.loading = false;
        syncAdminGrantsPager();
      }
    }
  }

  function previousAdminGrantPage() {
    const page = state.grantsPage;
    const target = page.backPages.pop();
    if (!target) return;
    page.forwardPages.push(adminGrantPageSnapshot());
    applyAdminGrantPage(target);
  }

  async function nextAdminGrantPage() {
    const page = state.grantsPage;
    if (page.loading) return;
    if (page.forwardPages.length > 0) {
      const target = page.forwardPages.pop();
      page.backPages.push(adminGrantPageSnapshot());
      applyAdminGrantPage(target);
      return;
    }
    if (!page.nextCursor) return;
    await loadAdminGrants({ cursor: page.nextCursor, navigation: "next" });
  }

  async function retryGrant(id, button) {
    ui.setButtonBusy(button, true, "重试中");
    try {
      await ui.api(`/api/v1/admin/balance-grants/${encodeURIComponent(id)}/retry`, { method: "POST" });
      ui.notice("重试任务已加入队列");
      await loadAdminGrants({ cursor: "", navigation: "reset" });
    } catch (error) {
      ui.notice(error.message, true);
    } finally {
      if (button.isConnected) ui.setButtonBusy(button, false);
    }
  }

  function openReverseDialog(id, trigger) {
    state.reverseID = id;
    state.reverseReturnFocus = trigger || document.activeElement;
    ui.byId("reverse-reason").value = "";
    const dialog = ui.byId("reverse-dialog");
    if (typeof dialog.showModal === "function") dialog.showModal();
    else dialog.setAttribute("open", "");
    requestAnimationFrame(() => ui.byId("reverse-reason").focus());
  }

  function finishReverseDialog() {
    state.reverseID = "";
    const target = state.reverseReturnFocus;
    state.reverseReturnFocus = null;
    if (target?.isConnected) requestAnimationFrame(() => target.focus());
  }

  function closeReverseDialog() {
    const dialog = ui.byId("reverse-dialog");
    if (typeof dialog.close === "function") dialog.close();
    else {
      dialog.removeAttribute("open");
      finishReverseDialog();
    }
  }

  function tierField(labelText, name, type, value = "", step = "0.01", mode = "") {
    const label = document.createElement("label");
    label.textContent = labelText;
    if (mode) label.dataset.rewardFields = mode;
    let input;
    if (type === "select") {
      input = document.createElement("select");
      [["fixed_range", "固定金额区间"], ["percentage_range", "消费比例区间"]].forEach(([optionValue, title]) => {
        const option = document.createElement("option");
        option.value = optionValue;
        option.textContent = title;
        input.append(option);
      });
      input.value = value || "fixed_range";
    } else {
      input = document.createElement("input");
      input.type = type;
      input.min = "0";
      input.step = step;
      input.value = value;
      if (name === "upper") input.placeholder = "不限";
    }
    input.dataset.field = name;
    label.append(input);
    return label;
  }

  function syncTierMode(row) {
    const mode = row.querySelector('[data-field="mode"]').value;
    row.querySelectorAll("[data-reward-fields]").forEach((label) => {
      const active = label.dataset.rewardFields === mode;
      label.hidden = !active;
      label.querySelector("input").disabled = !active;
    });
  }

  function addTier(tier = null) {
    const source = tier || {};
    const mode = source.reward_mode || "fixed_range";
    const row = document.createElement("div");
    row.className = "tier-row";
    row.append(
      tierField("起始积分", "lower", "number", decimalValue(source.lower_points_hundredths, 100, "0.00")),
      tierField("结束积分", "upper", "number", source.upper_points_hundredths == null ? "" : decimalValue(source.upper_points_hundredths, 100, "")),
      tierField("赠送方式", "mode", "select", mode),
      tierField("固定最低（U）", "fixedMin", "number", decimalValue(source.fixed_reward_min_microusd, 1_000_000, "0.01"), "0.01", "fixed_range"),
      tierField("固定最高（U）", "fixedMax", "number", decimalValue(source.fixed_reward_max_microusd, 1_000_000, "0.01"), "0.01", "fixed_range"),
      tierField("比例最低（%）", "percentageMin", "number", decimalValue(source.reward_percentage_min_ppm, 10_000, "0.00"), "0.01", "percentage_range"),
      tierField("比例最高（%）", "percentageMax", "number", decimalValue(source.reward_percentage_max_ppm, 10_000, "5.00"), "0.01", "percentage_range")
    );
    row.querySelector('[data-field="mode"]').addEventListener("change", () => syncTierMode(row));
    const remove = document.createElement("button");
    remove.type = "button";
    remove.className = "button danger remove-tier";
    remove.title = "删除阶梯";
    remove.setAttribute("aria-label", "删除阶梯");
    remove.append(ui.icon("trash-2"));
    remove.addEventListener("click", () => row.remove());
    row.append(remove);
    ui.byId("tiers").append(row);
    syncTierMode(row);
  }

  function scaledInteger(raw, decimals, label) {
    const value = String(raw).trim();
    const match = value.match(new RegExp(`^(\\d+)(?:\\.(\\d{1,${decimals}}))?$`));
    if (!match) throw new Error(`${label}最多保留 ${decimals} 位小数`);
    const fraction = (match[2] || "").padEnd(decimals, "0");
    const result = BigInt(match[1]) * (10n ** BigInt(decimals)) + BigInt(fraction || "0");
    if (result > BigInt(Number.MAX_SAFE_INTEGER)) throw new Error(`${label}数值过大`);
    return Number(result);
  }

  function multiplySafe(value, multiplier, label) {
    if (value > Math.floor(Number.MAX_SAFE_INTEGER / multiplier)) throw new Error(`${label}数值过大`);
    return value * multiplier;
  }

  function moneyInput(input, label) {
    return multiplySafe(scaledInteger(input.value, 2, label), 10_000, label);
  }

  function pointsInput(input, label) {
    return scaledInteger(input.value, 2, label);
  }

  function percentageInput(input, label) {
    return multiplySafe(scaledInteger(input.value, 2, label), 100, label);
  }

  function nullableScaled(input, converter, label) {
    return input.value === "" ? null : converter(input, label);
  }

  function tiersPayload() {
    return [...document.querySelectorAll(".tier-row")].map((row, index) => {
      const field = (name) => row.querySelector(`[data-field="${name}"]`);
      const mode = field("mode").value;
      const label = `第 ${index + 1} 档`;
      const lower = pointsInput(field("lower"), `${label}起始积分`);
      const upper = nullableScaled(field("upper"), pointsInput, `${label}结束积分`);
      if (upper != null && upper <= lower) throw new Error(`${label}结束积分必须大于起始积分`);
      return {
        lower_points_hundredths: lower,
        upper_points_hundredths: upper,
        reward_mode: mode,
        fixed_reward_min_microusd: mode === "fixed_range" ? moneyInput(field("fixedMin"), `${label}固定最低金额`) : null,
        fixed_reward_max_microusd: mode === "fixed_range" ? moneyInput(field("fixedMax"), `${label}固定最高金额`) : null,
        reward_percentage_min_ppm: mode === "percentage_range" ? percentageInput(field("percentageMin"), `${label}比例最低值`) : null,
        reward_percentage_max_ppm: mode === "percentage_range" ? percentageInput(field("percentageMax"), `${label}比例最高值`) : null
      };
    });
  }

  function refreshMinuteInput() {
    const match = ui.byId("policy-refresh-time").value.match(/^(\d{2}):(\d{2})$/);
    if (!match) throw new Error("请选择每日刷新时间");
    const hour = Number(match[1]);
    const minute = Number(match[2]);
    if (hour > 23 || minute > 59) throw new Error("每日刷新时间不正确");
    return hour * 60 + minute;
  }

  function integerInput(id, label) {
    const raw = ui.byId(id).value;
    const value = Number(raw);
    if (!/^\d+$/.test(raw) || value < 1 || !Number.isSafeInteger(value)) throw new Error(`${label}必须是正整数`);
    return value;
  }

  function setControlDescription(control, descriptionID) {
    if (descriptionID) control.setAttribute("aria-describedby", descriptionID);
    else control.removeAttribute("aria-describedby");
  }

  function syncConsumerOnly() {
    const consumerOnly = ui.byId("policy-consumer-only").checked;
    const basis = ui.byId("policy-basis");
    if (consumerOnly) basis.value = "yesterday";
    const checkinEnabled = ui.byId("policy-enabled").checked && ui.byId("policy-checkin-enabled").checked;
    basis.disabled = !checkinEnabled || consumerOnly;
    const basisLocked = checkinEnabled && consumerOnly;
    ui.byId("basis-lock-reason").classList.toggle("hidden", !basisLocked);
    setControlDescription(basis, !checkinEnabled ? "checkin-lock-reason" : basisLocked ? "basis-lock-reason" : "");
  }

  function syncCheckinControls() {
    const pointsEnabled = ui.byId("policy-enabled").checked;
    const checkinToggle = ui.byId("policy-checkin-enabled");
    checkinToggle.disabled = !pointsEnabled;
    if (!pointsEnabled) checkinToggle.checked = false;
    const checkinEnabled = pointsEnabled && checkinToggle.checked;
    const lockReason = ui.byId("checkin-lock-reason");
    lockReason.textContent = pointsEnabled
      ? "启用签到赠送后，可配置签到资格、赠送上限和奖励阶梯。"
      : "请先开放用户积分功能，再启用签到赠送。";
    lockReason.classList.toggle("hidden", checkinEnabled);
    setControlDescription(checkinToggle, checkinEnabled ? "" : "checkin-lock-reason");
    ["policy-consumer-only", "policy-checkin-limit", "policy-minimum-spend", "policy-single-cap", "policy-user-cap", "policy-platform-cap"].forEach((id) => {
      const control = ui.byId(id);
      control.disabled = !checkinEnabled;
      setControlDescription(control, checkinEnabled ? "" : "checkin-lock-reason");
      if (id !== "policy-consumer-only" && id !== "policy-minimum-spend") control.required = checkinEnabled;
    });
    const addTier = ui.byId("add-tier");
    addTier.disabled = !checkinEnabled;
    setControlDescription(addTier, checkinEnabled ? "" : "checkin-lock-reason");
    const settings = ui.byId("checkin-settings");
    const tiers = ui.byId("checkin-tiers");
    settings.classList.toggle("is-locked", !checkinEnabled);
    tiers.classList.toggle("is-locked", !checkinEnabled);
    if (checkinEnabled) {
      settings.removeAttribute("aria-describedby");
      tiers.removeAttribute("aria-describedby");
      tiers.removeAttribute("aria-disabled");
    } else {
      settings.setAttribute("aria-describedby", "checkin-lock-reason");
      tiers.setAttribute("aria-describedby", "checkin-lock-reason");
      tiers.setAttribute("aria-disabled", "true");
    }
    document.querySelectorAll(".tier-row").forEach((row) => {
      row.querySelectorAll("input, select, button").forEach((control) => {
        control.disabled = !checkinEnabled;
        setControlDescription(control, checkinEnabled ? "" : "checkin-lock-reason");
      });
      if (checkinEnabled) syncTierMode(row);
    });
    syncConsumerOnly();
  }

  function moveAdminTab(event) {
    const keys = ["ArrowLeft", "ArrowRight", "Home", "End"];
    if (!keys.includes(event.key)) return;
    const buttons = [...document.querySelectorAll(".admin-nav-button")];
    const current = buttons.indexOf(event.currentTarget);
    let next = current;
    if (event.key === "ArrowLeft") next = (current - 1 + buttons.length) % buttons.length;
    if (event.key === "ArrowRight") next = (current + 1) % buttons.length;
    if (event.key === "Home") next = 0;
    if (event.key === "End") next = buttons.length - 1;
    event.preventDefault();
    setView(buttons[next].dataset.view);
    buttons[next].focus();
  }

  async function refreshAll() {
    const results = await Promise.allSettled([loadPolicies(), loadAdminGrants(), loadAdminUsers()]);
    const failed = results.find((result) => result.status === "rejected");
    if (failed) throw failed.reason;
  }

  function bindEvents() {
    document.querySelectorAll(".admin-nav-button").forEach((button) => {
      button.addEventListener("click", () => setView(button.dataset.view));
      button.addEventListener("keydown", moveAdminTab);
    });
    ui.byId("logout").addEventListener("click", () => ui.logout().catch((error) => ui.notice(error.message, true)));
    ui.byId("refresh-admin").addEventListener("click", async (event) => {
      const button = event.currentTarget;
      setAdminSyncState("loading", "正在同步控制台数据");
      ui.setButtonBusy(button, true, "刷新中");
      try {
        await refreshAll();
        setAdminSyncState("ready", "控制台数据已同步");
        ui.notice("后台数据已刷新");
      } catch (error) {
        setAdminSyncState("error", "控制台数据同步失败");
        ui.notice(error.message, true);
      } finally {
        ui.setButtonBusy(button, false);
      }
    });
    ui.byId("refresh-admin-grants").addEventListener("click", async (event) => {
      const button = event.currentTarget;
      ui.setButtonBusy(button, true, "刷新中");
      try {
        await loadAdminGrants();
      } catch (error) {
        ui.notice(error.message, true);
      } finally {
        ui.setButtonBusy(button, false);
      }
    });
    ui.byId("admin-grants-prev").addEventListener("click", () => {
      if (state.grantsPage.loading) return;
      previousAdminGrantPage();
    });
    ui.byId("admin-grants-next").addEventListener("click", () => {
      nextAdminGrantPage().catch((error) => ui.notice(error.message, true));
    });
    ui.byId("refresh-admin-users").addEventListener("click", async (event) => {
      const button = event.currentTarget;
      ui.setButtonBusy(button, true, "刷新中");
      try {
        await loadAdminUsers();
      } catch (error) {
        ui.notice(error.message, true);
      } finally {
        ui.setButtonBusy(button, false);
      }
    });
    ui.byId("admin-users-prev").addEventListener("click", () => {
      if (state.usersPage.loading) return;
      const offset = Math.max(0, state.usersPage.offset - state.usersPage.limit);
      loadAdminUsers({ offset }).catch((error) => ui.notice(error.message, true));
    });
    ui.byId("admin-users-next").addEventListener("click", () => {
      if (state.usersPage.loading || state.usersPage.offset + state.usersPage.limit >= state.usersPage.total) return;
      const offset = state.usersPage.offset + state.usersPage.limit;
      loadAdminUsers({ offset }).catch((error) => ui.notice(error.message, true));
    });
    ui.byId("add-tier").addEventListener("click", () => addTier());
    ui.byId("policy-consumer-only").addEventListener("change", syncConsumerOnly);
    ui.byId("policy-checkin-enabled").addEventListener("change", syncCheckinControls);
    ui.byId("policy-enabled").addEventListener("change", syncCheckinControls);

    ui.byId("policy-form").addEventListener("submit", async (event) => {
      event.preventDefault();
      const button = event.currentTarget.querySelector('[type="submit"]');
      ui.setButtonBusy(button, true, "保存中");
      try {
        const effectiveDate = setPolicyEffectiveDate();
        const checkinEnabled = ui.byId("policy-checkin-enabled").checked;
        const tiers = tiersPayload();
        if (checkinEnabled && tiers.length === 0) throw new Error("启用签到赠送时至少需要一个赠送阶梯");
        await ui.api("/api/v1/admin/policies", {
          method: "POST",
          body: JSON.stringify({
            effective_date: effectiveDate,
            enabled: ui.byId("policy-enabled").checked,
            mode: ui.byId("policy-consumer-only").checked ? "consumer_only" : "all_users",
            basis: ui.byId("policy-basis").value,
            checkin_enabled: checkinEnabled,
            checkin_daily_limit: integerInput("policy-checkin-limit", "每日签到次数"),
            minimum_checkin_spend_microusd: moneyInput(ui.byId("policy-minimum-spend"), "最低昨日消费"),
            checkin_platform_daily_cap_microusd: moneyInput(ui.byId("policy-platform-cap"), "全平台每日上限"),
            checkin_user_daily_cap_microusd: moneyInput(ui.byId("policy-user-cap"), "单用户每日上限"),
            checkin_single_award_cap_microusd: moneyInput(ui.byId("policy-single-cap"), "单次赠送上限"),
            points_per_usd_hundredths: pointsInput(ui.byId("policy-points-rate"), "每 U 积分"),
            refresh_minute: refreshMinuteInput(),
            tiers
          })
        });
        ui.notice(`策略已保存，将于 ${effectiveDate} 生效`);
        await loadPolicies();
      } catch (error) {
        ui.notice(error.message, true);
      } finally {
        ui.setButtonBusy(button, false);
      }
    });

    ui.byId("cancel-reverse").addEventListener("click", closeReverseDialog);
    ui.byId("reverse-dialog").addEventListener("close", finishReverseDialog);
    ui.byId("reverse-form").addEventListener("submit", async (event) => {
      event.preventDefault();
      const reason = ui.byId("reverse-reason").value.trim();
      if (!reason) {
        ui.notice("请填写冲正原因", true);
        return;
      }
      const button = event.currentTarget.querySelector('[type="submit"]');
      ui.setButtonBusy(button, true, "提交中");
      try {
        await ui.api(`/api/v1/admin/balance-grants/${encodeURIComponent(state.reverseID)}/reverse`, {
          method: "POST",
          body: JSON.stringify({ reason })
        });
        closeReverseDialog();
        ui.notice("冲正任务已加入队列");
        await loadAdminGrants({ cursor: "", navigation: "reset" });
      } catch (error) {
        ui.notice(error.message, true);
      } finally {
        ui.setButtonBusy(button, false);
      }
    });
  }

  async function bootstrap() {
    const access = ui.byId("admin-access-state");
    const app = ui.byId("admin-app");
    const retryButton = ui.byId("retry-admin-bootstrap");
    if (bootstrapInFlight) return;
    bootstrapInFlight = true;
    setAdminSyncState("loading", "正在同步控制台数据");
    ui.setButtonBusy(retryButton, true, "加载中");
    setAccessState("正在加载积分控制台");
    try {
      const data = await ui.api("/api/v1/admin/me");
      if (data?.role !== "admin") {
        app.remove();
        setAccessState("当前账户无权访问管理后台", { error: true });
        return;
      }
      ui.setSession(data);
      state.businessDate = typeof data.business_date === "string"
        ? data.business_date.slice(0, 10)
        : "";
      state.nextPolicyDate = typeof data.next_policy_date === "string"
        ? data.next_policy_date.slice(0, 10)
        : "";
      ui.byId("admin-login-email").textContent = data.login_email || "未设置登录邮箱";
      initializeAdminWorkspace();
      await refreshAll();
      setAdminSyncState("ready", "控制台数据已同步");
      access.remove();
      app.hidden = false;
    } catch (error) {
      setAdminSyncState("error", "控制台数据同步失败");
      app.hidden = true;
      setAccessState(error.message, { error: true, retry: true });
    } finally {
      bootstrapInFlight = false;
      ui.setButtonBusy(retryButton, false);
      ui.notifyReady();
    }
  }

  ui.byId("retry-admin-bootstrap").addEventListener("click", bootstrap);
  bootstrap();
})();
