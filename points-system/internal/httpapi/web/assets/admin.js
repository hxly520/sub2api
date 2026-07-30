"use strict";

(() => {
  const ui = window.PointsUI;
  const state = {
    policies: [],
    grants: [],
    users: [],
    usersPage: { limit: 50, offset: 0, total: 0 },
    reverseID: "",
    view: "overview"
  };
  const viewTitles = {
    overview: "运行总览",
    users: "用户明细",
    policies: "策略管理",
    operations: "签到发放记录"
  };

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

  function setView(name) {
    if (!viewTitles[name]) return;
    state.view = name;
    document.querySelectorAll(".admin-view").forEach((view) => view.classList.add("hidden"));
    document.querySelectorAll(".admin-nav-button").forEach((button) => {
      button.classList.toggle("active", button.dataset.view === name);
    });
    ui.byId(`view-${name}`).classList.remove("hidden");
    ui.byId("admin-page-title").textContent = viewTitles[name];
  }

  function currentPolicy() {
    const today = new Date();
    today.setHours(23, 59, 59, 999);
    const effective = state.policies.filter((policy) => new Date(policy.effective_date) <= today);
    return effective[0] || state.policies[0] || null;
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

    const pendingStatuses = new Set(["pending", "processing", "reversal_pending", "reversal_processing"]);
    ui.byId("overview-pending").textContent = String(state.grants.filter((grant) => pendingStatuses.has(grant.status)).length);
    const summary = ui.byId("grant-status-summary");
    summary.replaceChildren();
    const groups = [
      ["待处理", ["pending", "processing"]],
      ["已到账", ["settled"]],
      ["失败", ["failed", "permanently_failed"]],
      ["冲正中", ["reversal_pending", "reversal_processing"]],
      ["已冲正", ["reversed"]]
    ];
    groups.forEach(([label, statuses]) => {
      const item = document.createElement("div");
      const name = document.createElement("span");
      const count = document.createElement("strong");
      name.textContent = label;
      count.textContent = String(state.grants.filter((grant) => statuses.includes(grant.status)).length);
      item.append(name, count);
      summary.append(item);
    });
  }

  async function loadPolicies() {
    const rows = await ui.api("/api/v1/admin/policies?limit=50");
    state.policies = Array.isArray(rows) ? rows : [];
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
    ], "尚未创建策略版本");
    renderOverview();
  }

  async function loadAdminUsers() {
    const page = state.usersPage;
    const data = await ui.api(`/api/v1/admin/users/points?limit=${page.limit}&offset=${page.offset}`);
    state.users = Array.isArray(data?.items) ? data.items : [];
    page.total = Math.max(0, ui.number(data?.total));
    page.limit = Math.max(1, ui.number(data?.limit) || page.limit);
    page.offset = Math.max(0, ui.number(data?.offset));
    ui.renderRows("admin-users-body", state.users, [
      "user_id",
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
    ui.byId("admin-users-prev").disabled = page.offset === 0;
    ui.byId("admin-users-next").disabled = page.offset + state.users.length >= page.total;
  }

  function actionButton(label, className, handler) {
    const button = document.createElement("button");
    button.type = "button";
    button.className = `button ${className} table-action`;
    button.textContent = label;
    button.addEventListener("click", handler);
    return button;
  }

  async function loadAdminGrants() {
    const rows = await ui.api("/api/v1/admin/balance-grants?limit=100");
    state.grants = Array.isArray(rows) ? rows : [];
    ui.renderRows("admin-grants-body", state.grants, [
      (grant) => ui.dateTime(grant.created_at),
      "user_id",
      (grant) => ui.money(grant.amount_microusd),
      (grant) => ui.kindText(grant.kind),
      (grant) => ui.statusChip(grant.status),
      "attempts",
      (grant) => grant.last_error || "-",
      (grant, cell) => {
        cell.className = "actions";
        const retryable = ["failed", "permanently_failed", "reversal_permanently_failed"].includes(grant.status);
        if (retryable) cell.append(actionButton("重试", "secondary", () => retryGrant(grant.id)));
        const reversible = grant.status === "settled" || (grant.status === "pending" && grant.attempts === 0);
        if (reversible) cell.append(actionButton("冲正", "danger", () => openReverseDialog(grant.id)));
        if (!retryable && !reversible) cell.textContent = "-";
      }
    ], "暂无赠送任务");
    renderOverview();
  }

  async function retryGrant(id) {
    try {
      await ui.api(`/api/v1/admin/balance-grants/${encodeURIComponent(id)}/retry`, { method: "POST" });
      ui.notice("重试任务已加入队列");
      await loadAdminGrants();
    } catch (error) {
      ui.notice(error.message, true);
    }
  }

  function openReverseDialog(id) {
    state.reverseID = id;
    ui.byId("reverse-reason").value = "";
    const dialog = ui.byId("reverse-dialog");
    if (typeof dialog.showModal === "function") dialog.showModal();
    else dialog.setAttribute("open", "");
  }

  function closeReverseDialog() {
    state.reverseID = "";
    const dialog = ui.byId("reverse-dialog");
    if (typeof dialog.close === "function") dialog.close();
    else dialog.removeAttribute("open");
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

  function addTier() {
    const row = document.createElement("div");
    row.className = "tier-row";
    row.append(
      tierField("起始积分", "lower", "number", "0.00"),
      tierField("结束积分", "upper", "number"),
      tierField("赠送方式", "mode", "select"),
      tierField("固定最低（U）", "fixedMin", "number", "0.01", "0.01", "fixed_range"),
      tierField("固定最高（U）", "fixedMax", "number", "0.01", "0.01", "fixed_range"),
      tierField("比例最低（%）", "percentageMin", "number", "0.00", "0.01", "percentage_range"),
      tierField("比例最高（%）", "percentageMax", "number", "5.00", "0.01", "percentage_range")
    );
    row.querySelector('[data-field="mode"]').addEventListener("change", () => syncTierMode(row));
    const remove = document.createElement("button");
    remove.type = "button";
    remove.className = "button danger remove-tier";
    remove.title = "删除阶梯";
    remove.setAttribute("aria-label", "删除阶梯");
    remove.textContent = "×";
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

  function syncConsumerOnly() {
    const consumerOnly = ui.byId("policy-consumer-only").checked;
    const basis = ui.byId("policy-basis");
    if (consumerOnly) basis.value = "yesterday";
    basis.disabled = consumerOnly;
  }

  function syncCheckinControls() {
    const pointsEnabled = ui.byId("policy-enabled").checked;
    const checkinToggle = ui.byId("policy-checkin-enabled");
    checkinToggle.disabled = !pointsEnabled;
    if (!pointsEnabled) checkinToggle.checked = false;
    const checkinEnabled = pointsEnabled && checkinToggle.checked;
    ["policy-checkin-limit", "policy-single-cap", "policy-user-cap", "policy-platform-cap"].forEach((id) => {
      ui.byId(id).required = checkinEnabled;
    });
  }

  async function refreshAll() {
    const results = await Promise.allSettled([loadPolicies(), loadAdminGrants(), loadAdminUsers()]);
    const failed = results.find((result) => result.status === "rejected");
    if (failed) throw failed.reason;
  }

  function bindEvents() {
    document.querySelectorAll(".admin-nav-button").forEach((button) => {
      button.addEventListener("click", () => setView(button.dataset.view));
    });
    ui.byId("logout").addEventListener("click", () => ui.logout().catch((error) => ui.notice(error.message, true)));
    ui.byId("refresh-admin").addEventListener("click", async (event) => {
      ui.setButtonBusy(event.currentTarget, true, "刷新中");
      try {
        await refreshAll();
        ui.notice("后台数据已刷新");
      } catch (error) {
        ui.notice(error.message, true);
      } finally {
        ui.setButtonBusy(event.currentTarget, false);
      }
    });
    ui.byId("refresh-admin-grants").addEventListener("click", () => loadAdminGrants().catch((error) => ui.notice(error.message, true)));
    ui.byId("refresh-admin-users").addEventListener("click", () => loadAdminUsers().catch((error) => ui.notice(error.message, true)));
    ui.byId("admin-users-prev").addEventListener("click", () => {
      state.usersPage.offset = Math.max(0, state.usersPage.offset - state.usersPage.limit);
      loadAdminUsers().catch((error) => ui.notice(error.message, true));
    });
    ui.byId("admin-users-next").addEventListener("click", () => {
      if (state.usersPage.offset + state.usersPage.limit >= state.usersPage.total) return;
      state.usersPage.offset += state.usersPage.limit;
      loadAdminUsers().catch((error) => ui.notice(error.message, true));
    });
    ui.byId("toggle-policy-form").addEventListener("click", () => ui.byId("policy-form").classList.toggle("hidden"));
    ui.byId("cancel-policy").addEventListener("click", () => ui.byId("policy-form").classList.add("hidden"));
    ui.byId("add-tier").addEventListener("click", addTier);
    ui.byId("policy-consumer-only").addEventListener("change", syncConsumerOnly);
    ui.byId("policy-checkin-enabled").addEventListener("change", syncCheckinControls);
    ui.byId("policy-enabled").addEventListener("change", syncCheckinControls);

    ui.byId("policy-form").addEventListener("submit", async (event) => {
      event.preventDefault();
      const button = event.currentTarget.querySelector('[type="submit"]');
      ui.setButtonBusy(button, true, "创建中");
      try {
        const checkinEnabled = ui.byId("policy-checkin-enabled").checked;
        const tiers = tiersPayload();
        if (checkinEnabled && tiers.length === 0) throw new Error("启用签到赠送时至少需要一个赠送阶梯");
        await ui.api("/api/v1/admin/policies", {
          method: "POST",
          body: JSON.stringify({
            effective_date: ui.byId("policy-date").value,
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
        ui.byId("policy-form").classList.add("hidden");
        ui.notice("策略版本已创建");
        await loadPolicies();
      } catch (error) {
        ui.notice(error.message, true);
      } finally {
        ui.setButtonBusy(button, false);
      }
    });

    ui.byId("cancel-reverse").addEventListener("click", closeReverseDialog);
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
        await loadAdminGrants();
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
    try {
      const data = await ui.api("/api/v1/admin/me");
      if (data?.role !== "admin") {
        app.remove();
        access.textContent = "当前账户无权访问管理后台";
        return;
      }
      ui.setSession(data);
      ui.byId("admin-user-id").textContent = `管理员 ${data.user_id}`;
      access.remove();
      app.hidden = false;
      bindEvents();
      const tomorrow = localDate(1);
      ui.byId("policy-date").min = tomorrow;
      ui.byId("policy-date").value = tomorrow;
      addTier();
      syncConsumerOnly();
      syncCheckinControls();
      await refreshAll();
    } catch (error) {
      app.remove();
      access.textContent = error.message;
      access.classList.add("error");
    }
  }

  bootstrap();
})();
