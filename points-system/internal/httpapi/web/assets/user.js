"use strict";

(() => {
  const ui = window.PointsUI;
  const chartState = { rows: [], days: 30, geometry: null, hoverIndex: -1 };
  let profile = null;

  function signedPoints(value) {
    const amount = ui.number(value);
    const node = document.createElement("span");
    node.className = amount > 0 ? "number-positive" : amount < 0 ? "number-negative" : "";
    node.textContent = `${amount > 0 ? "+" : ""}${ui.points(amount)}`;
    return node;
  }

  function syncCheckin(data) {
    const button = ui.byId("checkin");
    const title = ui.byId("checkin-title");
    const detail = ui.byId("checkin-detail");
    const count = ui.number(data.checkin?.count);
    const features = data.features || {};
    const limit = ui.number(features.checkin_daily_limit);
    const available = features.checkin_available === true;
    const band = document.querySelector(".checkin-band");

    ui.byId("checkin-count").textContent = `今日已签到 ${count} 次`;
    button.disabled = !available;
    button.textContent = available ? "立即签到" : "暂不可签到";
    band.dataset.status = available ? "ready" : "off";

    if (features.points_enabled !== true) {
      band.dataset.status = "off";
      title.textContent = "积分功能暂未开放";
      detail.textContent = "请等待功能完成调试并开放";
      return;
    }
    if (features.checkin_enabled !== true) {
      band.dataset.status = "off";
      title.textContent = "签到赠送暂未开启";
      detail.textContent = "今日积分数据仍可正常查看";
      return;
    }
    if (limit > 0 && count >= limit) {
      band.dataset.status = "complete";
      title.textContent = "今日签到已完成";
      detail.textContent = `今日签到次数 ${count} / ${limit}`;
      button.textContent = "今日已签到";
      return;
    }
    title.textContent = "今日签到可参与";
    detail.textContent = limit > 0
      ? `今日签到次数 ${count} / ${limit}，奖励资格及金额将在提交时按完整规则校验`
      : "奖励资格及金额将在提交时按完整规则校验";
  }

  async function loadProfile() {
    const data = await ui.api("/api/v1/me");
    if (!data || data.role !== "user") {
      window.location.replace("/admin/");
      return null;
    }
    profile = data;
    ui.setSession(data);
    ui.byId("user-id").textContent = `用户 ${data.user_id}`;
    ui.byId("total-points").textContent = ui.points(data.account?.total_points_hundredths);
    ui.byId("today-rewards").textContent = ui.money(data.checkin?.awarded_microusd);
    ui.byId("total-checkin-rewards").textContent = ui.money(data.account?.settled_checkin_reward_microusd);

    const snapshot = data.yesterday_snapshot;
    ui.byId("yesterday-points").textContent = ui.points(snapshot?.awarded_points_hundredths);
    ui.byId("snapshot-date").textContent = snapshot ? `${ui.date(snapshot.business_date)} 结算` : "暂无结算";
    syncCheckin(data);
    return data;
  }

  async function loadLedger() {
    const rows = await ui.api("/api/v1/ledger?limit=100");
    ui.renderRows("ledger-body", rows, [
      (item) => ui.dateTime(item.created_at),
      (item) => ui.kindText(item.kind),
      (item) => signedPoints(item.delta_points_hundredths),
      (item) => ui.points(item.total_after_hundredths),
      (item) => ui.date(item.business_date)
    ], "暂无积分变动记录");
  }

  async function loadGrants() {
    const rows = await ui.api("/api/v1/balance-grants?limit=100");
    ui.renderRows("grants-body", rows, [
      (item) => ui.dateTime(item.created_at),
      (item) => ui.money(item.amount_microusd),
      (item) => ui.kindText(item.kind),
      (item) => ui.statusChip(item.status)
    ], "暂无签到赠送记录");
  }

  function chartColors() {
    const styles = getComputedStyle(document.documentElement);
    return {
      line: styles.getPropertyValue("--chart-line").trim() || "#147d64",
      fill: styles.getPropertyValue("--chart-fill").trim() || "rgba(20, 125, 100, .12)",
      grid: styles.getPropertyValue("--line").trim() || "#dfe4e8",
      muted: styles.getPropertyValue("--muted").trim() || "#657078",
      surface: styles.getPropertyValue("--surface").trim() || "#ffffff"
    };
  }

  function niceMaximum(value) {
    if (value <= 1) return 1;
    const power = 10 ** Math.floor(Math.log10(value));
    const scaled = value / power;
    const step = scaled <= 2 ? 2 : scaled <= 5 ? 5 : 10;
    return step * power;
  }

  function drawChart() {
    const canvas = ui.byId("points-chart");
    const rows = chartState.rows;
    const empty = ui.byId("chart-empty");
    empty.classList.toggle("hidden", rows.length > 0);
    canvas.classList.toggle("hidden", rows.length === 0);
    if (rows.length === 0) {
      chartState.geometry = null;
      return;
    }

    const bounds = canvas.getBoundingClientRect();
    const width = Math.max(320, Math.round(bounds.width));
    const height = Math.max(250, Math.round(bounds.height));
    const density = Math.min(window.devicePixelRatio || 1, 2);
    canvas.width = Math.round(width * density);
    canvas.height = Math.round(height * density);
    const context = canvas.getContext("2d");
    context.setTransform(density, 0, 0, density, 0, 0);
    context.clearRect(0, 0, width, height);

    const padding = { top: 24, right: 22, bottom: 40, left: 58 };
    const plotWidth = width - padding.left - padding.right;
    const plotHeight = height - padding.top - padding.bottom;
    const values = rows.map((item) => ui.number(item.awarded_points_hundredths) / 100);
    const maximum = niceMaximum(Math.max(...values, 0));
    const colors = chartColors();
    const xAt = (index) => rows.length === 1
      ? padding.left + plotWidth / 2
      : padding.left + (plotWidth * index) / (rows.length - 1);
    const yAt = (value) => padding.top + plotHeight - (value / maximum) * plotHeight;

    context.lineWidth = 1;
    context.font = "12px system-ui, sans-serif";
    context.textBaseline = "middle";
    for (let index = 0; index <= 4; index += 1) {
      const y = padding.top + (plotHeight * index) / 4;
      context.strokeStyle = colors.grid;
      context.beginPath();
      context.moveTo(padding.left, y);
      context.lineTo(width - padding.right, y);
      context.stroke();
      context.fillStyle = colors.muted;
      context.textAlign = "right";
      context.fillText(ui.points(((maximum * (4 - index)) / 4) * 100), padding.left - 10, y);
    }

    const labelCount = Math.min(rows.length, 6);
    const labelIndexes = new Set();
    for (let index = 0; index < labelCount; index += 1) {
      labelIndexes.add(Math.round((index * (rows.length - 1)) / Math.max(labelCount - 1, 1)));
    }
    context.fillStyle = colors.muted;
    context.textAlign = "center";
    context.textBaseline = "top";
    labelIndexes.forEach((index) => {
      context.fillText(ui.shortDate(rows[index].business_date), xAt(index), height - padding.bottom + 12);
    });

    context.beginPath();
    values.forEach((value, index) => {
      const x = xAt(index);
      const y = yAt(value);
      if (index === 0) context.moveTo(x, y);
      else context.lineTo(x, y);
    });
    context.lineTo(xAt(rows.length - 1), padding.top + plotHeight);
    context.lineTo(xAt(0), padding.top + plotHeight);
    context.closePath();
    context.fillStyle = colors.fill;
    context.fill();

    context.beginPath();
    values.forEach((value, index) => {
      const x = xAt(index);
      const y = yAt(value);
      if (index === 0) context.moveTo(x, y);
      else context.lineTo(x, y);
    });
    context.strokeStyle = colors.line;
    context.lineWidth = 2.5;
    context.lineJoin = "round";
    context.lineCap = "round";
    context.stroke();

    if (rows.length <= 31 || chartState.hoverIndex >= 0) {
      values.forEach((value, index) => {
        if (rows.length > 31 && index !== chartState.hoverIndex) return;
        context.beginPath();
        context.arc(xAt(index), yAt(value), index === chartState.hoverIndex ? 5 : 3, 0, Math.PI * 2);
        context.fillStyle = colors.surface;
        context.fill();
        context.strokeStyle = colors.line;
        context.lineWidth = 2;
        context.stroke();
      });
    }

    chartState.geometry = { padding, plotWidth, plotHeight, width, height, xAt, yAt };
    canvas.setAttribute("aria-label", `${chartState.days} 日每日消费积分折线图，区间积分 ${ui.points(values.reduce((sum, value) => sum + value, 0) * 100)}`);
  }

  function updateChartSummary(rows) {
    const totalPoints = rows.reduce((sum, item) => sum + ui.number(item.awarded_points_hundredths), 0);
    const activeDays = rows.filter((item) => ui.number(item.actual_cost_microusd) > 0 || ui.number(item.awarded_points_hundredths) > 0).length;
    ui.byId("period-points").textContent = ui.points(totalPoints);
    ui.byId("average-points").textContent = ui.points(rows.length === 0 ? 0 : Math.round(totalPoints / rows.length));
    ui.byId("active-days").textContent = String(activeDays);
  }

  async function loadDailyPoints(days = chartState.days) {
    const rows = await ui.api(`/api/v1/daily-points?days=${encodeURIComponent(days)}`);
    chartState.days = days;
    chartState.rows = Array.isArray(rows) ? rows : [];
    chartState.hoverIndex = -1;
    document.querySelectorAll(".period-button").forEach((button) => {
      button.classList.toggle("active", Number(button.dataset.days) === days);
    });
    updateChartSummary(chartState.rows);
    drawChart();
  }

  function showChartTooltip(event) {
    const geometry = chartState.geometry;
    if (!geometry || chartState.rows.length === 0) return;
    const canvas = ui.byId("points-chart");
    const bounds = canvas.getBoundingClientRect();
    const localX = event.clientX - bounds.left;
    const ratio = Math.max(0, Math.min(1, (localX - geometry.padding.left) / geometry.plotWidth));
    const index = chartState.rows.length === 1 ? 0 : Math.round(ratio * (chartState.rows.length - 1));
    const item = chartState.rows[index];
    chartState.hoverIndex = index;
    drawChart();

    const tooltip = ui.byId("chart-tooltip");
    tooltip.textContent = `${ui.date(item.business_date)} · ${ui.points(item.awarded_points_hundredths)} 积分`;
    tooltip.classList.remove("hidden");
    const x = geometry.xAt(index);
    const y = geometry.yAt(ui.number(item.awarded_points_hundredths) / 100);
    tooltip.style.left = `${Math.max(8, Math.min(geometry.width - tooltip.offsetWidth - 8, x - tooltip.offsetWidth / 2))}px`;
    tooltip.style.top = `${Math.max(8, y - tooltip.offsetHeight - 14)}px`;
  }

  function hideChartTooltip() {
    chartState.hoverIndex = -1;
    ui.byId("chart-tooltip").classList.add("hidden");
    drawChart();
  }

  async function refreshDashboard() {
    const page = document.querySelector(".dashboard-page");
    page.setAttribute("aria-busy", "true");
    try {
      const data = await loadProfile();
      if (!data) return;
      const results = await Promise.allSettled([loadDailyPoints(), loadLedger(), loadGrants()]);
      const failed = results.find((result) => result.status === "rejected");
      if (failed) throw failed.reason;
    } finally {
      page.setAttribute("aria-busy", "false");
    }
  }

  function bindEvents() {
    ui.byId("logout").addEventListener("click", () => ui.logout().catch((error) => ui.notice(error.message, true)));
    ui.byId("refresh-dashboard").addEventListener("click", async (event) => {
      ui.setButtonBusy(event.currentTarget, true, "刷新中");
      try {
        await refreshDashboard();
        ui.notice("数据已刷新");
      } catch (error) {
        ui.notice(error.message, true);
      } finally {
        ui.setButtonBusy(event.currentTarget, false);
      }
    });
    ui.byId("refresh-ledger").addEventListener("click", () => loadLedger().catch((error) => ui.notice(error.message, true)));
    ui.byId("refresh-grants").addEventListener("click", () => loadGrants().catch((error) => ui.notice(error.message, true)));
    ui.byId("checkin").addEventListener("click", async (event) => {
      const button = event.currentTarget;
      ui.setButtonBusy(button, true, "签到中");
      try {
        const result = await ui.api("/api/v1/checkins", {
          method: "POST",
          headers: { "Idempotency-Key": ui.idempotencyKey() }
        });
        ui.notice(`签到成功，赠送 ${ui.money(result.reward_microusd)}，${ui.statusText(result.delivery_status)}`);
        await Promise.all([loadProfile(), loadGrants()]);
      } catch (error) {
        ui.notice(error.message, true);
      } finally {
        ui.setButtonBusy(button, false);
        if (profile) syncCheckin(profile);
      }
    });
    document.querySelectorAll(".period-button").forEach((button) => {
      button.addEventListener("click", () => loadDailyPoints(Number(button.dataset.days)).catch((error) => ui.notice(error.message, true)));
    });
    const canvas = ui.byId("points-chart");
    canvas.addEventListener("pointermove", showChartTooltip);
    canvas.addEventListener("pointerleave", hideChartTooltip);
    if (typeof ResizeObserver === "function") new ResizeObserver(drawChart).observe(canvas);
    else window.addEventListener("resize", drawChart);
  }

  bindEvents();
  refreshDashboard()
    .catch((error) => ui.notice(error.message, true))
    .finally(ui.notifyReady);
})();
