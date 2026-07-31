"use strict";

(() => {
  const ui = window.PointsUI;
  const chartState = { rows: [], days: 30, geometry: null, hoverIndex: -1, requestSequence: 0 };
  const recordPageSize = 10;
  const recordPages = {
    ledger: createRecordPage("/api/v1/ledger"),
    grants: createRecordPage("/api/v1/balance-grants")
  };
  let profile = null;
  let checkinInFlight = false;
  let checkinNeedsConfirmation = false;
  let pendingCheckinKey = "";
  let pendingCheckinBaseline = null;
  const pendingCheckinStorageKey = "points.pending-checkin.v1";

  function checkinBusinessDate(data) {
    const serverDate = String(data?.business_date || "").slice(0, 10);
    if (/^\d{4}-\d{2}-\d{2}$/.test(serverDate)) return serverDate;
    const raw = String(data?.yesterday_snapshot?.business_date || "").slice(0, 10);
    const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(raw);
    if (!match) return "";
    const next = new Date(Date.UTC(Number(match[1]), Number(match[2]) - 1, Number(match[3]) + 1));
    return next.toISOString().slice(0, 10);
  }

  function removeStoredPendingCheckin() {
    try {
      sessionStorage.removeItem(pendingCheckinStorageKey);
    } catch {
      // Session storage is optional; the in-memory idempotency guard remains active.
    }
  }

  function clearPendingCheckin() {
    pendingCheckinKey = "";
    pendingCheckinBaseline = null;
    removeStoredPendingCheckin();
  }

  function savePendingCheckin(data) {
    if (!pendingCheckinKey || !pendingCheckinBaseline) return;
    const value = {
      key: pendingCheckinKey,
      login_email: String(data?.login_email || ""),
      business_date: pendingCheckinBaseline.businessDate,
      count: pendingCheckinBaseline.count
    };
    try {
      sessionStorage.setItem(pendingCheckinStorageKey, JSON.stringify(value));
    } catch {
      // The current page still reuses the in-memory key when storage is unavailable.
    }
  }

  function restorePendingCheckin(data) {
    if (pendingCheckinKey) return;
    let value = null;
    try {
      value = JSON.parse(sessionStorage.getItem(pendingCheckinStorageKey) || "null");
    } catch {
      removeStoredPendingCheckin();
      return;
    }
    const key = String(value?.key || "");
    const count = Number(value?.count);
    const businessDate = checkinBusinessDate(data);
    if (key.length < 16 || key.length > 128 || !Number.isInteger(count) || count < 0 ||
        !businessDate || value?.business_date !== businessDate ||
        value?.login_email !== String(data?.login_email || "")) {
      removeStoredPendingCheckin();
      return;
    }
    pendingCheckinKey = key;
    pendingCheckinBaseline = { count, businessDate };
    checkinNeedsConfirmation = true;
  }

  function beginPendingCheckin(data) {
    if (pendingCheckinKey) return;
    pendingCheckinKey = ui.idempotencyKey();
    pendingCheckinBaseline = {
      count: ui.number(data?.checkin?.count),
      businessDate: checkinBusinessDate(data)
    };
    savePendingCheckin(data);
  }

  function resolvePendingCheckin(data, confirmCheckin) {
    restorePendingCheckin(data);
    if (!confirmCheckin && !checkinNeedsConfirmation) return;
    if (!pendingCheckinKey) {
      checkinNeedsConfirmation = false;
      return;
    }
    const sameBusinessDate = pendingCheckinBaseline?.businessDate === checkinBusinessDate(data);
    const countAdvanced = ui.number(data?.checkin?.count) > ui.number(pendingCheckinBaseline?.count);
    if (!sameBusinessDate || countAdvanced) {
      clearPendingCheckin();
      checkinNeedsConfirmation = false;
    }
  }

  function createRecordPage(endpoint) {
    return {
      endpoint,
      cursor: "",
      items: [],
      nextCursor: "",
      backCursors: [],
      forwardCursors: [],
      loading: false,
      requestSequence: 0
    };
  }

  function signedPoints(value) {
    const amount = ui.number(value);
    const node = document.createElement("span");
    node.className = amount > 0 ? "number-positive" : amount < 0 ? "number-negative" : "";
    node.textContent = `${amount > 0 ? "+" : ""}${ui.points(amount)}`;
    return node;
  }

  function syncCheckin(data) {
    const button = ui.byId("checkin");
    const count = ui.number(data.checkin?.count);
    const features = data.features || {};
    const limit = ui.number(features.checkin_daily_limit);
    const available = features.checkin_available === true;
    const band = document.querySelector(".checkin-band");

    ui.byId("checkin-count").textContent = `今日已签到 ${count} 次`;
    if (checkinInFlight) {
      button.disabled = true;
      ui.setButtonLabel(button, "签到中");
      band.dataset.status = "off";
      return;
    }
    if (checkinNeedsConfirmation) {
      button.disabled = !pendingCheckinKey;
      ui.setButtonLabel(button, pendingCheckinKey ? "确认签到结果" : "状态待确认");
      band.dataset.status = "off";
      return;
    }
    button.disabled = !available;
    ui.setButtonLabel(button, available ? "立即签到" : "暂不可签到");
    band.dataset.status = available ? "ready" : "off";

    if (features.points_enabled !== true) {
      band.dataset.status = "off";
      return;
    }
    if (features.checkin_enabled !== true) {
      band.dataset.status = "off";
      return;
    }
    if (limit > 0 && count >= limit) {
      band.dataset.status = "complete";
      ui.setButtonLabel(button, "今日已签到");
      return;
    }
  }

  async function loadProfile({ confirmCheckin = false } = {}) {
    const data = await ui.api("/api/v1/me");
    if (!data || data.role !== "user") {
      window.location.replace("/admin/");
      return null;
    }
    resolvePendingCheckin(data, confirmCheckin);
    profile = data;
    ui.setSession(data);
    ui.byId("login-email").textContent = data.login_email || "未设置登录邮箱";
    ui.byId("total-points").textContent = ui.points(data.account?.total_points_hundredths);
    ui.byId("today-rewards").textContent = ui.money(data.checkin?.awarded_microusd);
    ui.byId("total-checkin-rewards").textContent = ui.money(data.account?.settled_checkin_reward_microusd);

    const snapshot = data.yesterday_snapshot;
    ui.byId("yesterday-points").textContent = ui.points(snapshot?.awarded_points_hundredths);
    ui.byId("snapshot-date").textContent = snapshot ? `${ui.date(snapshot.business_date)} 结算` : "暂无结算";
    syncCheckin(data);
    return data;
  }

  function syncRecordPager(name) {
    const page = recordPages[name];
    ui.byId(`${name}-page`).textContent = `第 ${page.backCursors.length + 1} 页`;
    ui.byId(`${name}-prev`).disabled = page.loading || page.backCursors.length === 0;
    ui.byId(`${name}-next`).disabled = page.loading ||
      (page.forwardCursors.length === 0 && !page.nextCursor);
  }

  function recordPageSnapshot(page) {
    return { cursor: page.cursor, items: page.items, nextCursor: page.nextCursor };
  }

  function renderRecordRows(name, items) {
    if (name === "ledger") {
      ui.renderRows("ledger-body", items, [
        (item) => ui.dateTime(item.awarded_at),
        (item) => ui.kindText(item.kind),
        (item) => signedPoints(item.delta_points_hundredths),
        (item) => ui.points(item.total_after_hundredths),
        (item) => ui.date(item.business_date)
      ], "暂无积分变动记录");
      return;
    }
    ui.renderRows("grants-body", items, [
      (item) => ui.dateTime(item.created_at),
      (item) => ui.money(item.amount_microusd),
      (item) => ui.kindText(item.kind),
      (item) => ui.statusChip(item.status)
    ], "暂无签到赠送记录");
  }

  function applyRecordPage(name, snapshot) {
    const page = recordPages[name];
    page.cursor = snapshot.cursor;
    page.items = snapshot.items;
    page.nextCursor = snapshot.nextCursor;
    renderRecordRows(name, page.items);
    syncRecordPager(name);
  }

  async function loadRecordPage(name, { cursor = recordPages[name].cursor, navigation = "replace" } = {}) {
    const page = recordPages[name];
    const requestSequence = ++page.requestSequence;
    page.loading = true;
    syncRecordPager(name);
    try {
      const data = await ui.api(`${page.endpoint}?limit=${recordPageSize}&cursor=${encodeURIComponent(cursor)}`);
      if (requestSequence !== page.requestSequence) return;
      const previous = recordPageSnapshot(page);
      if (navigation === "next") {
        page.backCursors.push(previous);
        page.forwardCursors = [];
      } else if (navigation === "reset") {
        page.backCursors = [];
        page.forwardCursors = [];
      } else {
        page.forwardCursors = [];
      }
      applyRecordPage(name, {
        cursor,
        items: Array.isArray(data?.items) ? data.items : [],
        nextCursor: typeof data?.next_cursor === "string" ? data.next_cursor : ""
      });
    } catch (error) {
      if (requestSequence === page.requestSequence && cursor && error?.code === "invalid_cursor") {
        return loadRecordPage(name, { cursor: "", navigation: "reset" });
      }
      throw error;
    } finally {
      if (requestSequence === page.requestSequence) {
        page.loading = false;
        syncRecordPager(name);
      }
    }
  }

  function restoreRecordPage(name, direction) {
    const page = recordPages[name];
    const source = direction === "prev" ? page.backCursors : page.forwardCursors;
    const target = source.pop();
    if (!target) return false;
    const destination = direction === "prev" ? page.forwardCursors : page.backCursors;
    destination.push(recordPageSnapshot(page));
    applyRecordPage(name, target);
    return true;
  }

  function previousRecordPage(name) {
    restoreRecordPage(name, "prev");
  }

  async function nextRecordPage(name) {
    const page = recordPages[name];
    if (page.forwardCursors.length > 0) {
      restoreRecordPage(name, "next");
      return;
    }
    if (!page.nextCursor) return;
    await loadRecordPage(name, { cursor: page.nextCursor, navigation: "next" });
  }

  function loadLedger(options) {
    return loadRecordPage("ledger", options);
  }

  function loadGrants(options) {
    return loadRecordPage("grants", options);
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
    // Use the actual CSS width. A fixed canvas floor makes narrow embedded
    // views scale down the plot and crowds the date labels on mobile.
    const width = Math.max(1, Math.round(bounds.width));
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

    // Keep enough horizontal breathing room for a date label while retaining
    // the first and last dates as useful anchors on small screens.
    const labelCount = Math.min(rows.length, 6, Math.max(2, Math.floor(plotWidth / 88) + 1));
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
    ui.byId("average-points").textContent = ui.points(Math.round(totalPoints / chartState.days));
    ui.byId("active-days").textContent = String(activeDays);
    ui.renderRows("chart-data-body", rows, [
      (item) => ui.date(item.business_date),
      (item) => ui.points(item.awarded_points_hundredths)
    ], "暂无每日积分记录");
  }

  async function loadDailyPoints(days = chartState.days) {
    const requestSequence = ++chartState.requestSequence;
    const panel = document.querySelector(".chart-panel");
    panel.setAttribute("aria-busy", "true");
    try {
      const rows = await ui.api(`/api/v1/daily-points?days=${encodeURIComponent(days)}`);
      if (requestSequence !== chartState.requestSequence) return;
      chartState.days = days;
      chartState.rows = Array.isArray(rows) ? rows : [];
      chartState.hoverIndex = -1;
      document.querySelectorAll(".period-button").forEach((button) => {
        const active = Number(button.dataset.days) === days;
        button.classList.toggle("active", active);
        button.setAttribute("aria-pressed", String(active));
      });
      updateChartSummary(chartState.rows);
      drawChart();
    } finally {
      if (requestSequence === chartState.requestSequence) panel.setAttribute("aria-busy", "false");
    }
  }

  function showChartPoint(index, announce = false) {
    if (!chartState.geometry || chartState.rows.length === 0) return;
    index = Math.max(0, Math.min(chartState.rows.length - 1, index));
    const item = chartState.rows[index];
    chartState.hoverIndex = index;
    drawChart();

    const geometry = chartState.geometry;
    const tooltip = ui.byId("chart-tooltip");
    tooltip.textContent = `${ui.date(item.business_date)} · ${ui.points(item.awarded_points_hundredths)} 积分`;
    tooltip.classList.remove("hidden");
    const x = geometry.xAt(index);
    const y = geometry.yAt(ui.number(item.awarded_points_hundredths) / 100);
    tooltip.style.left = `${Math.max(8, Math.min(geometry.width - tooltip.offsetWidth - 8, x - tooltip.offsetWidth / 2))}px`;
    tooltip.style.top = `${Math.max(8, y - tooltip.offsetHeight - 14)}px`;
    if (announce) ui.byId("chart-live").textContent = tooltip.textContent;
  }

  function showChartTooltip(event) {
    const geometry = chartState.geometry;
    if (!geometry || chartState.rows.length === 0) return;
    const bounds = ui.byId("points-chart").getBoundingClientRect();
    const localX = event.clientX - bounds.left;
    const ratio = Math.max(0, Math.min(1, (localX - geometry.padding.left) / geometry.plotWidth));
    const index = chartState.rows.length === 1 ? 0 : Math.round(ratio * (chartState.rows.length - 1));
    showChartPoint(index);
  }

  function hideChartTooltip() {
    chartState.hoverIndex = -1;
    ui.byId("chart-tooltip").classList.add("hidden");
    drawChart();
  }

  function moveChartPoint(event) {
    if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key) || chartState.rows.length === 0) return;
    event.preventDefault();
    let index = chartState.hoverIndex;
    if (event.key === "Home") index = 0;
    else if (event.key === "End") index = chartState.rows.length - 1;
    else if (index < 0) index = event.key === "ArrowLeft" ? chartState.rows.length - 1 : 0;
    else index += event.key === "ArrowLeft" ? -1 : 1;
    showChartPoint(index, true);
  }

  function movePeriod(event) {
    if (!["ArrowLeft", "ArrowRight", "Home", "End"].includes(event.key)) return;
    const buttons = [...document.querySelectorAll(".period-button")];
    const current = buttons.indexOf(event.currentTarget);
    let next = current;
    if (event.key === "ArrowLeft") next = (current - 1 + buttons.length) % buttons.length;
    if (event.key === "ArrowRight") next = (current + 1) % buttons.length;
    if (event.key === "Home") next = 0;
    if (event.key === "End") next = buttons.length - 1;
    event.preventDefault();
    buttons[next].focus();
    loadDailyPoints(Number(buttons[next].dataset.days)).catch((error) => ui.notice(error.message, true));
  }

  function showDashboardError(error) {
    const page = document.querySelector(".dashboard-page");
    page.dataset.loadState = "error";
    ui.byId("dashboard-error-message").textContent = error?.message || "请重新加载积分数据。";
    ui.byId("dashboard-error").classList.remove("hidden");
  }

  function clearDashboardError() {
    const page = document.querySelector(".dashboard-page");
    page.dataset.loadState = "loading";
    ui.byId("dashboard-error").classList.add("hidden");
  }

  async function refreshDashboard() {
    const page = document.querySelector(".dashboard-page");
    const confirmCheckin = checkinNeedsConfirmation && !checkinInFlight;
    clearDashboardError();
    page.setAttribute("aria-busy", "true");
    try {
      const data = await loadProfile({ confirmCheckin });
      if (!data) return;
      const results = await Promise.allSettled([loadDailyPoints(), loadLedger(), loadGrants()]);
      const failed = results.find((result) => result.status === "rejected");
      if (failed) throw failed.reason;
      page.dataset.loadState = "ready";
    } catch (error) {
      showDashboardError(error);
      throw error;
    } finally {
      page.setAttribute("aria-busy", "false");
    }
  }

  function bindEvents() {
    window.addEventListener("points:themechange", drawChart);
    ui.byId("logout").addEventListener("click", () => ui.logout().catch((error) => ui.notice(error.message, true)));
    ui.byId("retry-dashboard").addEventListener("click", async (event) => {
      const button = event.currentTarget;
      ui.setButtonBusy(button, true, "加载中");
      try {
        await refreshDashboard();
      } catch {
        // refreshDashboard renders the persistent retry state.
      } finally {
        ui.setButtonBusy(button, false);
      }
    });
    ui.byId("refresh-dashboard").addEventListener("click", async (event) => {
      const button = event.currentTarget;
      ui.setButtonBusy(button, true, "刷新中");
      try {
        await refreshDashboard();
        ui.notice("数据已刷新");
      } catch {
        // refreshDashboard renders the persistent retry state.
      } finally {
        ui.setButtonBusy(button, false);
      }
    });
    ui.byId("refresh-ledger").addEventListener("click", async (event) => {
      const button = event.currentTarget;
      ui.setButtonBusy(button, true, "刷新中");
      try {
        await loadLedger();
      } catch (error) {
        ui.notice(error.message, true);
      } finally {
        ui.setButtonBusy(button, false);
      }
    });
    ui.byId("refresh-grants").addEventListener("click", async (event) => {
      const button = event.currentTarget;
      ui.setButtonBusy(button, true, "刷新中");
      try {
        await loadGrants();
      } catch (error) {
        ui.notice(error.message, true);
      } finally {
        ui.setButtonBusy(button, false);
      }
    });
    ui.byId("ledger-prev").addEventListener("click", () => {
      previousRecordPage("ledger");
    });
    ui.byId("ledger-next").addEventListener("click", () => {
      nextRecordPage("ledger").catch((error) => ui.notice(error.message, true));
    });
    ui.byId("grants-prev").addEventListener("click", () => {
      previousRecordPage("grants");
    });
    ui.byId("grants-next").addEventListener("click", () => {
      nextRecordPage("grants").catch((error) => ui.notice(error.message, true));
    });
    ui.byId("checkin").addEventListener("click", async (event) => {
      const button = event.currentTarget;
      let refreshedProfile = null;
      let responseReceived = false;
      checkinInFlight = true;
      ui.setButtonBusy(button, true, "签到中");
      try {
        beginPendingCheckin(profile || {});
        const result = await ui.api("/api/v1/checkins", {
          method: "POST",
          headers: { "Idempotency-Key": pendingCheckinKey }
        });
        responseReceived = true;
        checkinNeedsConfirmation = true;
        savePendingCheckin(profile || {});
        ui.notice(`签到成功，赠送 ${ui.money(result.reward_microusd)}，${ui.statusText(result.delivery_status)}`);
        refreshedProfile = await loadProfile({ confirmCheckin: true });
        await loadGrants({ cursor: "", navigation: "reset" });
      } catch (error) {
        if (!responseReceived) {
          const status = Number(error?.status);
          const definitiveFailure = Number.isInteger(status) && status >= 400 && status < 500 && status !== 408 && status !== 429;
          if (definitiveFailure) {
            clearPendingCheckin();
            checkinNeedsConfirmation = false;
          } else {
            checkinNeedsConfirmation = true;
            savePendingCheckin(profile || {});
          }
        }
        ui.notice(error.message, true);
        if (!refreshedProfile) {
          try {
            refreshedProfile = await loadProfile({ confirmCheckin: true });
          } catch {
            // Keep the action locked until a later dashboard refresh confirms state.
          }
        }
      } finally {
        checkinInFlight = false;
        ui.setButtonBusy(button, false);
        syncCheckin(refreshedProfile || profile || {});
      }
    });
    document.querySelectorAll(".period-button").forEach((button) => {
      button.addEventListener("click", () => loadDailyPoints(Number(button.dataset.days)).catch((error) => ui.notice(error.message, true)));
      button.addEventListener("keydown", movePeriod);
    });
    const canvas = ui.byId("points-chart");
    canvas.addEventListener("pointermove", showChartTooltip);
    canvas.addEventListener("pointerleave", () => {
      if (document.activeElement !== canvas) hideChartTooltip();
    });
    canvas.addEventListener("keydown", moveChartPoint);
    canvas.addEventListener("focus", () => {
      if (chartState.rows.length > 0 && chartState.hoverIndex < 0) showChartPoint(chartState.rows.length - 1, true);
    });
    canvas.addEventListener("blur", hideChartTooltip);
    if (typeof ResizeObserver === "function") new ResizeObserver(drawChart).observe(canvas);
    else window.addEventListener("resize", drawChart);
  }

  bindEvents();
  refreshDashboard()
    .catch(() => {})
    .finally(ui.notifyReady);
})();
