// ghosthaze thinker control panel client script

document.addEventListener("DOMContentLoaded", function () {
  // theme management (dark, light, system)
  let currentTheme = localStorage.getItem("ghosthaze_theme") || "dark";

  function applyTheme(theme) {
    let effective = theme;
    if (theme === "system") {
      effective = window.matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light";
    }
    document.documentElement.setAttribute("data-theme", effective);

    const lightIcon = document.getElementById("theme-icon-light");
    const darkIcon = document.getElementById("theme-icon-dark");
    const systemIcon = document.getElementById("theme-icon-system");

    if (lightIcon && darkIcon && systemIcon) {
      lightIcon.style.display = theme === "light" ? "block" : "none";
      darkIcon.style.display = theme === "dark" ? "block" : "none";
      systemIcon.style.display = theme === "system" ? "block" : "none";
    }
  }

  applyTheme(currentTheme);

  window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", function () {
    if (currentTheme === "system") {
      applyTheme("system");
    }
  });

  const themeToggleBtn = document.getElementById("theme-toggle-btn");
  const themeDropdown = document.getElementById("theme-dropdown");
  if (themeToggleBtn && themeDropdown) {
    themeToggleBtn.addEventListener("click", function (e) {
      e.stopPropagation();
      themeDropdown.classList.toggle("show");
    });

    document.querySelectorAll(".theme-option").forEach((opt) => {
      opt.addEventListener("click", function () {
        const selected = this.getAttribute("data-theme-val");
        currentTheme = selected;
        localStorage.setItem("ghosthaze_theme", selected);
        applyTheme(selected);
        themeDropdown.classList.remove("show");
      });
    });

    document.addEventListener("click", function () {
      themeDropdown.classList.remove("show");
    });
  }

  // hamburger mobile navigation drawer with 90deg animation
  const hamburger = document.getElementById("hamburger");
  const mobileMenu = document.getElementById("mobile-menu");
  if (hamburger && mobileMenu) {
    hamburger.addEventListener("click", function () {
      this.classList.toggle("active");
      mobileMenu.classList.toggle("open");
    });
  }

  // tab navigation switching
  const tabs = document.querySelectorAll(".nav-tab-btn");
  const mobileNavBtns = document.querySelectorAll(".mobile-nav-btn");
  const panes = document.querySelectorAll(".tab-pane");

  function switchTab(target) {
    tabs.forEach((t) => {
      if (t.getAttribute("data-tab") === target) t.classList.add("active");
      else t.classList.remove("active");
    });

    mobileNavBtns.forEach((b) => {
      if (b.getAttribute("data-tab") === target) b.classList.add("active");
      else b.classList.remove("active");
    });

    panes.forEach((p) => p.classList.remove("active"));
    const activePane = document.getElementById("tab-" + target);
    if (activePane) {
      activePane.classList.add("active");
    }

    if (target === "battles") {
      fetchFormats();
    }

    if (hamburger && mobileMenu) {
      hamburger.classList.remove("active");
      mobileMenu.classList.remove("open");
    }
  }

  tabs.forEach(function (tab) {
    tab.addEventListener("click", function () {
      const target = this.getAttribute("data-tab");
      if (target) switchTab(target);
    });
  });

  mobileNavBtns.forEach(function (btn) {
    btn.addEventListener("click", function () {
      const target = this.getAttribute("data-tab");
      if (target) switchTab(target);
    });
  });

  // state variables for dual timers
  let isConnected = false;
  let isStopped = false;
  let connectionStartMs = 0;
  let serverStartMs = 0;
  let activeLogFilter = "all";
  let cachedLogs = [];

  // format seconds into clean human readable duration
  function formatSeconds(seconds) {
    if (seconds <= 0) return "0s";
    const s = seconds % 60;
    const m = Math.floor(seconds / 60) % 60;
    const h = Math.floor(seconds / 3600) % 24;
    const d = Math.floor(seconds / 86400);

    const parts = [];
    if (d > 0) parts.push(d + "d");
    if (h > 0) parts.push(h + "h");
    if (m > 0) parts.push(m + "m");
    if (s > 0 || parts.length === 0) parts.push(s + "s");
    return parts.join(" ");
  }

  // ticker updating both uptime and contime every second
  function tickTimers() {
    // 1. connection uptime (contime)
    const contimeEl = document.getElementById("stat-contime");
    if (contimeEl) {
      if (isStopped) {
        contimeEl.textContent = "Stopped";
      } else if (!isConnected || connectionStartMs <= 0) {
        contimeEl.textContent = "Disconnected";
      } else {
        const sec = Math.max(0, Math.floor((Date.now() - connectionStartMs) / 1000));
        contimeEl.textContent = formatSeconds(sec);
      }
    }

    // 2. process uptime (uptime)
    const uptimeEl = document.getElementById("stat-uptime");
    if (uptimeEl) {
      if (serverStartMs > 0) {
        const sec = Math.max(0, Math.floor((Date.now() - serverStartMs) / 1000));
        uptimeEl.textContent = formatSeconds(sec);
      }
    }
  }

  setInterval(tickTimers, 1000);

  // periodic status polling
  function updateStatus(isInitial) {
    fetch("/api/status")
      .then((res) => res.json())
      .then((data) => {
        isConnected = !!data.connected;
        isStopped = !!data.stopped;

        if (data.connected_at_ms > 0 && isConnected) {
          connectionStartMs = data.connected_at_ms;
        } else if (data.connection_uptime_seconds > 0 && isConnected) {
          if (connectionStartMs <= 0) {
            connectionStartMs = Date.now() - data.connection_uptime_seconds * 1000;
          }
        } else {
          connectionStartMs = 0;
        }

        if (data.server_started_at_ms > 0) {
          serverStartMs = data.server_started_at_ms;
        } else if (data.server_uptime_seconds > 0 && serverStartMs <= 0) {
          serverStartMs = Date.now() - data.server_uptime_seconds * 1000;
        }

        tickTimers();

        // status badge
        const dot = document.getElementById("status-dot");
        const statusText = document.getElementById("status-text");
        const statusBadge = document.getElementById("status-badge");
        if (dot && statusText) {
          if (isStopped) {
            dot.className = "status-dot offline";
            statusText.textContent = "Stopped";
          } else if (isConnected) {
            dot.className = "status-dot online";
            statusText.textContent = data.logged_in ? "Online (" + (data.username || "Bot") + ")" : "Connecting...";
          } else {
            dot.className = "status-dot offline";
            statusText.textContent = "Offline";
          }
          if (statusBadge) {
            statusBadge.title = statusText.textContent;
          }
        }

        // overview cards
        const statConn = document.getElementById("stat-conn");
        const statServer = document.getElementById("stat-server");
        const statUser = document.getElementById("stat-user");
        const statRooms = document.getElementById("stat-rooms");
        const statBattles = document.getElementById("stat-battles");

        if (statConn) {
          if (isStopped) statConn.textContent = "Stopped";
          else if (isConnected) statConn.textContent = data.logged_in ? "Connected" : "Authenticating";
          else statConn.textContent = "Disconnected";
        }

        if (statServer) {
          statServer.textContent = (data.server_id || "showdown") + " (" + (data.server_host || "sim3.psim.us") + ":" + (data.server_port || 443) + ")";
        }

        if (statUser) {
          statUser.textContent = data.username || "Guest";
        }

        if (statRooms) {
          statRooms.textContent = data.chat_rooms_count !== undefined ? data.chat_rooms_count : (data.rooms ? data.rooms.length : 0);
        }

        if (statBattles) {
          statBattles.textContent = data.active_battles_count !== undefined ? data.active_battles_count : (data.active_battles ? data.active_battles.length : 0);
        }

        // toggle stop/start button state
        const toggleBtn = document.getElementById("btn-bot-toggle-state");
        if (toggleBtn) {
          if (isStopped) {
            toggleBtn.className = "btn btn-success";
            toggleBtn.textContent = "Start Bot";
          } else {
            toggleBtn.className = "btn btn-danger";
            toggleBtn.textContent = "Stop Bot";
          }
        }

        // filter chatrooms vs battle rooms
        const chatRooms = (data.rooms || []).filter((r) => !r.startsWith("battle-"));
        renderRooms(chatRooms);
        renderBattles(data.active_battles || []);

        if (isInitial) {
          populateConfigForm(data);
        }
      })
      .catch((err) => {
        console.error("status fetch error:", err);
      });
  }

  // populate configuration form fields
  function populateConfigForm(data) {
    const hostEl = document.getElementById("cfg-server-host");
    const portEl = document.getElementById("cfg-server-port");
    const idEl = document.getElementById("cfg-server-id");
    const sslEl = document.getElementById("cfg-server-ssl");
    const userEl = document.getElementById("cfg-username");
    const avatarEl = document.getElementById("cfg-avatar");
    const cmdEl = document.getElementById("cfg-command-char");
    const roomsEl = document.getElementById("cfg-rooms");
    const autoBattleEl = document.getElementById("cfg-auto-battle");
    const autoLeaveEl = document.getElementById("cfg-auto-leave-battle");
    const maxBattlesEl = document.getElementById("cfg-max-battles");
    const startMsgEl = document.getElementById("cfg-battle-start-msg");
    const winMsgEl = document.getElementById("cfg-battle-win-msg");
    const loseMsgEl = document.getElementById("cfg-battle-lose-msg");
    const formatsEl = document.getElementById("cfg-battle-formats");
    const teamEl = document.getElementById("cfg-battle-team");

    if (hostEl) hostEl.value = data.server_host || "sim3.psim.us";
    if (portEl) portEl.value = data.server_port || 443;
    if (idEl) idEl.value = data.server_id || "showdown";
    if (sslEl) sslEl.checked = data.server_ssl !== false;
    if (userEl) userEl.value = data.username || "";
    if (avatarEl) avatarEl.value = data.avatar || "";
    if (cmdEl) cmdEl.value = data.command_char || ".";
    if (roomsEl) roomsEl.value = data.config_rooms ? data.config_rooms.join(", ") : "";
    if (autoBattleEl) autoBattleEl.checked = !!data.auto_battle;
    if (autoLeaveEl) autoLeaveEl.checked = data.auto_leave_battle !== false;
    if (maxBattlesEl) maxBattlesEl.value = data.max_battles !== undefined ? data.max_battles : 1;
    if (startMsgEl) startMsgEl.value = data.battle_start_msg || "";
    if (winMsgEl) winMsgEl.value = data.battle_win_msg || "";
    if (loseMsgEl) loseMsgEl.value = data.battle_lose_msg || "";
    if (formatsEl) formatsEl.value = data.battle_formats ? data.battle_formats.join(", ") : "";
    if (teamEl) teamEl.value = data.battle_team || "";
  }

  // in-page modal dialog for chatroom messages (replaces prompt())
  const modalMsg = document.getElementById("modal-room-msg");
  const modalTarget = document.getElementById("modal-room-target");
  const modalInputMsg = document.getElementById("modal-input-msg");
  const modalForm = document.getElementById("form-modal-room-msg");
  const btnCloseModal = document.getElementById("btn-close-modal");
  const btnCancelModal = document.getElementById("btn-cancel-modal");
  let activeModalTarget = "";

  function openRoomModal(roomName) {
    activeModalTarget = roomName;
    if (modalTarget) modalTarget.textContent = roomName;
    if (modalInputMsg) modalInputMsg.value = "";
    if (modalMsg) modalMsg.style.display = "flex";
    if (modalInputMsg) modalInputMsg.focus();
  }

  function closeRoomModal() {
    if (modalMsg) modalMsg.style.display = "none";
    activeModalTarget = "";
  }

  if (btnCloseModal) btnCloseModal.addEventListener("click", closeRoomModal);
  if (btnCancelModal) btnCancelModal.addEventListener("click", closeRoomModal);
  if (modalMsg) {
    modalMsg.addEventListener("click", function (e) {
      if (e.target === modalMsg) closeRoomModal();
    });
  }

  if (modalForm) {
    modalForm.addEventListener("submit", function (e) {
      e.preventDefault();
      const msg = modalInputMsg ? modalInputMsg.value.trim() : "";
      if (!msg || !activeModalTarget) return;

      postJSON("/api/send", { target: activeModalTarget, message: msg, is_pm: false }, function (err) {
        if (err) {
          showAlert("error", "Failed to send: " + err);
        } else {
          showAlert("success", "Message sent to " + activeModalTarget);
          closeRoomModal();
          updateLogs();
        }
      });
    });
  }

  // render active chatroom table
  function renderRooms(rooms) {
    const tbody = document.getElementById("rooms-table-body");
    if (!tbody) return;

    if (rooms.length === 0) {
      tbody.innerHTML = '<tr><td colspan="3" style="text-align:center;color:var(--text-dim);padding:16px;">No chatrooms joined yet. Add a chatroom above or in Configuration.</td></tr>';
      return;
    }

    let html = "";
    rooms.forEach((r) => {
      html += `<tr>
        <td><strong>${escapeHTML(r)}</strong></td>
        <td><span class="chip" style="background:var(--success-light);color:#34d399;">Active</span></td>
        <td style="text-align:right;">
          <div class="table-actions">
            <button type="button" class="btn btn-secondary btn-sm btn-quick-msg" data-room="${escapeHTML(r)}">Message</button>
            <button type="button" class="btn btn-danger btn-sm btn-leave-room" data-room="${escapeHTML(r)}">Leave</button>
          </div>
        </td>
      </tr>`;
    });
    tbody.innerHTML = html;
  }

  // render active battle list
  function renderBattles(battles) {
    const container = document.getElementById("battles-container");
    if (!container) return;

    if (battles.length === 0) {
      container.innerHTML = '<p style="color:var(--text-dim);text-align:center;padding:24px;">No active battles currently in progress</p>';
      return;
    }

    let html = '<div class="table-wrapper"><table class="data-table"><thead><tr><th>Room</th><th>Format</th><th>Turn</th><th>Opponent</th><th style="text-align:right;">Actions</th></tr></thead><tbody>';
    battles.forEach((b) => {
      html += `<tr>
        <td><strong>${escapeHTML(b.room)}</strong></td>
        <td><span class="chip">${escapeHTML(b.format || "random")}</span></td>
        <td>${escapeHTML(b.turn || 0)}</td>
        <td>${escapeHTML(b.opponent || "Unknown")}</td>
        <td style="text-align:right;">
          <a href="https://play.pokemonshowdown.com/${escapeHTML(b.room)}" target="_blank" class="btn btn-secondary btn-sm" style="margin-right:6px;">Watch</a>
          <button type="button" class="btn btn-secondary btn-sm btn-forfeit-battle" data-room="${escapeHTML(b.room)}" style="margin-right:6px;">Forfeit</button>
          <button type="button" class="btn btn-danger btn-sm btn-leave-battle" data-room="${escapeHTML(b.room)}">Leave</button>
        </td>
      </tr>`;
    });
    html += "</tbody></table></div>";
    container.innerHTML = html;

    container.querySelectorAll(".btn-forfeit-battle").forEach((btn) => {
      btn.addEventListener("click", function () {
        const roomName = this.getAttribute("data-room");
        postJSON("/api/battles/forfeit", { room: roomName }, function (err) {
          if (err) showAlert("error", "Failed to forfeit battle: " + err);
          else {
            showAlert("success", "Forfeited battle " + roomName);
            updateStatus();
          }
        });
      });
    });

    container.querySelectorAll(".btn-leave-battle").forEach((btn) => {
      btn.addEventListener("click", function () {
        const roomName = this.getAttribute("data-room");
        postJSON("/api/battles/leave", { room: roomName }, function (err) {
          if (err) showAlert("error", "Failed to leave battle: " + err);
          else {
            showAlert("success", "Left battle room " + roomName);
            updateStatus();
          }
        });
      });
    });
  }

  // render activity logs with category filtering
  function renderLogs() {
    const logBox = document.getElementById("activity-log-box");
    if (!logBox) return;

    const isNearBottom = logBox.scrollHeight - logBox.scrollTop - logBox.clientHeight < 80;

    let filtered = cachedLogs;
    if (activeLogFilter !== "all") {
      filtered = cachedLogs.filter((e) => e.type === activeLogFilter);
    }

    if (!filtered || filtered.length === 0) {
      logBox.innerHTML = '<div style="color:var(--text-dim);padding:8px;">No log events found for this filter.</div>';
      return;
    }

    let html = "";
    filtered.forEach((e) => {
      let badgeClass = "badge-system";
      if (e.type === "chat") badgeClass = "badge-chat";
      else if (e.type === "pm") badgeClass = "badge-pm";
      else if (e.type === "battle") badgeClass = "badge-battle";
      else if (e.type === "room") badgeClass = "badge-room";

      html += `<div class="log-entry">
        <span class="log-time">${escapeHTML(e.time)}</span>
        <span class="log-badge ${badgeClass}">${escapeHTML(e.type)}</span>
        <span class="log-msg"><strong>${escapeHTML(e.source)}:</strong> ${escapeHTML(e.message)}</span>
      </div>`;
    });
    logBox.innerHTML = html;

    if (isNearBottom) {
      logBox.scrollTop = logBox.scrollHeight;
    }
  }

  function updateLogs() {
    fetch("/api/logs")
      .then((res) => res.json())
      .then((entries) => {
        cachedLogs = entries || [];
        renderLogs();
      })
      .catch((err) => {
        console.error("logs fetch error:", err);
      });
  }

  // log filter buttons (distinct class, no nav-tab-btn conflicts)
  document.querySelectorAll(".log-filter-btn").forEach((btn) => {
    btn.addEventListener("click", function (e) {
      e.preventDefault();
      document.querySelectorAll(".log-filter-btn").forEach((b) => b.classList.remove("active"));
      this.classList.add("active");
      activeLogFilter = this.getAttribute("data-filter") || "all";
      renderLogs();
    });
  });

  const clearLogsBtn = document.getElementById("btn-clear-logs");
  if (clearLogsBtn) {
    clearLogsBtn.addEventListener("click", function () {
      postJSON("/api/logs/clear", {}, function (err) {
        if (err) {
          showAlert("error", "Failed to clear logs: " + err);
        } else {
          cachedLogs = [];
          renderLogs();
          showAlert("success", "Activity logs cleared");
        }
      });
    });
  }

  // leave room helper
  function leaveRoom(room) {
    postJSON("/api/rooms/leave", { room: room }, function (err) {
      if (err) {
        showAlert("error", "Failed to leave room: " + err);
      } else {
        showAlert("success", "Left room " + room);
        updateStatus();
      }
    });
  }

  // join room form
  const joinForm = document.getElementById("form-join-room");
  if (joinForm) {
    joinForm.addEventListener("submit", function (e) {
      e.preventDefault();
      const input = document.getElementById("input-join-room");
      const room = input.value.trim();
      if (!room) return;

      postJSON("/api/rooms/join", { room: room }, function (err) {
        if (err) {
          showAlert("error", "Failed to join room: " + err);
        } else {
          showAlert("success", "Joined room " + room);
          input.value = "";
          updateStatus();
        }
      });
    });
  }

  // direct leave room form
  const leaveDirectForm = document.getElementById("form-leave-room-direct");
  if (leaveDirectForm) {
    leaveDirectForm.addEventListener("submit", function (e) {
      e.preventDefault();
      const input = document.getElementById("input-leave-room-direct");
      const room = input.value.trim();
      if (!room) return;
      leaveRoom(room);
      input.value = "";
    });
  }

  // delegate table actions for chatroom rows
  const roomsTableBody = document.getElementById("rooms-table-body");
  if (roomsTableBody) {
    roomsTableBody.addEventListener("click", function (e) {
      const leaveBtn = e.target.closest(".btn-leave-room");
      if (leaveBtn) {
        const room = leaveBtn.getAttribute("data-room");
        if (room) leaveRoom(room);
      }
      const msgBtn = e.target.closest(".btn-quick-msg");
      if (msgBtn) {
        const room = msgBtn.getAttribute("data-room");
        if (room) openRoomModal(room);
      }
    });
  }

  // quick announcement form
  const sendForm = document.getElementById("form-send-message");
  if (sendForm) {
    sendForm.addEventListener("submit", function (e) {
      e.preventDefault();
      const target = document.getElementById("input-send-target").value.trim();
      const message = document.getElementById("input-send-msg").value.trim();
      const isPM = document.getElementById("select-send-type").value === "pm";

      if (!target || !message) {
        showAlert("error", "Target and message cannot be empty");
        return;
      }

      postJSON("/api/send", { target: target, message: message, is_pm: isPM }, function (err) {
        if (err) {
          showAlert("error", "Failed to send message: " + err);
        } else {
          showAlert("success", "Message successfully sent!");
          document.getElementById("input-send-msg").value = "";
          updateLogs();
        }
      });
    });
  }

  // challenge user form
  const challengeForm = document.getElementById("form-challenge");
  if (challengeForm) {
    challengeForm.addEventListener("submit", function (e) {
      e.preventDefault();
      const user = document.getElementById("input-challenge-user").value.trim();
      const format = document.getElementById("input-challenge-format").value.trim() || "gen9randombattle";

      if (!user) {
        showAlert("error", "Username cannot be empty");
        return;
      }

      postJSON("/api/challenge", { user: user, format: format }, function (err) {
        if (err) {
          showAlert("error", "Failed to challenge user: " + err);
        } else {
          showAlert("success", "Challenge sent to " + user + " in " + format);
          document.getElementById("input-challenge-user").value = "";
          updateLogs();
        }
      });
    });
  }

  // quick challenge form in battles tab
  const quickChallengeForm = document.getElementById("form-quick-challenge");
  if (quickChallengeForm) {
    quickChallengeForm.addEventListener("submit", function (e) {
      e.preventDefault();
      const user = document.getElementById("input-quick-challenge-user").value.trim();
      const format = document.getElementById("input-quick-challenge-format").value.trim() || "gen9randombattle";

      if (!user) {
        showAlert("error", "Username cannot be empty");
        return;
      }

      postJSON("/api/challenge", { user: user, format: format }, function (err) {
        if (err) {
          showAlert("error", "Failed to challenge user: " + err);
        } else {
          showAlert("success", "Challenge sent to " + user + " in " + format);
          document.getElementById("input-quick-challenge-user").value = "";
          updateLogs();
          updateStatus();
        }
      });
    });
  }

  // preset format buttons
  document.querySelectorAll(".preset-chip-btn").forEach((btn) => {
    btn.addEventListener("click", function () {
      const fmt = this.getAttribute("data-format");
      const targetId = this.getAttribute("data-target");
      if (fmt && targetId) {
        const targetInput = document.getElementById(targetId);
        if (targetInput) {
          targetInput.value = fmt;
          showAlert("success", "Selected format: " + fmt);
        }
      }
    });
  });

  // server formats state and rendering
  let cachedFormats = [];

  function fetchFormats() {
    fetch("/api/formats")
      .then((res) => res.json())
      .then((data) => {
        if (Array.isArray(data)) {
          cachedFormats = data;
          renderFormats(cachedFormats);
        }
      })
      .catch(() => {});
  }

  function renderFormats(formats, query) {
    const container = document.getElementById("formats-list-container");
    const countBadge = document.getElementById("formats-count-badge");
    if (!container) return;

    const q = (query || "").trim().toLowerCase();
    const filtered = q
      ? formats.filter(
          (f) =>
            (f.name && f.name.toLowerCase().includes(q)) ||
            (f.id && f.id.toLowerCase().includes(q)) ||
            (f.section && f.section.toLowerCase().includes(q))
        )
      : formats;

    if (countBadge) {
      countBadge.textContent = `${filtered.length} Formats`;
    }

    if (filtered.length === 0) {
      container.innerHTML = `<p style="color:var(--text-dim);text-align:center;padding:16px;">${
        formats.length === 0
          ? "No formats received from server yet."
          : "No formats match your search."
      }</p>`;
      return;
    }

    const groups = {};
    filtered.forEach((f) => {
      const sec = f.section || "Other Formats";
      if (!groups[sec]) groups[sec] = [];
      groups[sec].push(f);
    });

    let html = "";
    Object.keys(groups).forEach((sec) => {
      html += `<div class="formats-section">
        <div class="formats-section-title">${escapeHTML(sec)}</div>
        <div class="formats-badges-wrap">`;
      groups[sec].forEach((f) => {
        html += `<div class="format-chip" data-format-id="${escapeHTML(
          f.id
        )}" title="Click to select ${escapeHTML(f.name || f.id)}">
          <span>${escapeHTML(f.name || f.id)}</span>
          <span class="format-chip-id">${escapeHTML(f.id)}</span>
        </div>`;
      });
      html += `</div></div>`;
    });

    container.innerHTML = html;

    container.querySelectorAll(".format-chip").forEach((chip) => {
      chip.addEventListener("click", function () {
        const fmtId = this.getAttribute("data-format-id");
        if (!fmtId) return;

        const quickFmt = document.getElementById("input-quick-challenge-format");
        if (quickFmt) quickFmt.value = fmtId;

        const toolFmt = document.getElementById("input-challenge-format");
        if (toolFmt) toolFmt.value = fmtId;

        showAlert("success", "Selected format: " + fmtId);

        const quickOpponent = document.getElementById("input-quick-challenge-user");
        if (quickOpponent) quickOpponent.focus();
      });
    });
  }

  const searchFormatsInput = document.getElementById("input-search-formats");
  if (searchFormatsInput) {
    searchFormatsInput.addEventListener("input", function () {
      renderFormats(cachedFormats, this.value);
    });
  }

  // get-server discovery tool
  const getServerForm = document.getElementById("form-get-server");
  if (getServerForm) {
    getServerForm.addEventListener("submit", function (e) {
      e.preventDefault();
      const input = document.getElementById("input-get-server-url");
      const url = input.value.trim();
      const resultBox = document.getElementById("get-server-result");
      if (!url) return;

      postJSON("/api/tools/get-server", { url: url }, function (err, data) {
        if (err) {
          showAlert("error", "Discovery error: " + err);
          if (resultBox) resultBox.style.display = "none";
        } else {
          if (resultBox) {
            resultBox.style.display = "block";
            document.getElementById("res-host").textContent = data.host;
            document.getElementById("res-port").textContent = data.port;
            document.getElementById("res-id").textContent = data.id;
            document.getElementById("res-tls").textContent = data.ssl ? "YES (TLS)" : "NO";
            document.getElementById("res-ws").textContent = data.ws_url;
            document.getElementById("res-login").textContent = data.login_url;

            const applyBtn = document.getElementById("btn-apply-discovered");
            if (applyBtn) {
              applyBtn.onclick = function () {
                document.getElementById("cfg-server-host").value = data.host;
                document.getElementById("cfg-server-port").value = data.port;
                document.getElementById("cfg-server-id").value = data.id;
                document.getElementById("cfg-server-ssl").checked = !!data.ssl;
                showAlert("success", "Applied discovered server values to Configuration tab!");
              };
            }
          }
        }
      });
    });
  }

  // set showdown defaults
  const btnSetDefault = document.getElementById("btn-set-default-server");
  if (btnSetDefault) {
    btnSetDefault.addEventListener("click", function () {
      document.getElementById("cfg-server-host").value = "sim3.psim.us";
      document.getElementById("cfg-server-port").value = 443;
      document.getElementById("cfg-server-id").value = "showdown";
      document.getElementById("cfg-server-ssl").checked = true;
      showAlert("success", "Loaded official Pokémon Showdown server defaults");
    });
  }

  // configuration save form
  const configForm = document.getElementById("form-bot-config");
  if (configForm) {
    configForm.addEventListener("submit", function (e) {
      e.preventDefault();
      saveBotConfig(false);
    });
  }

  const btnSaveReconnect = document.getElementById("btn-save-reconnect");
  if (btnSaveReconnect) {
    btnSaveReconnect.addEventListener("click", function () {
      saveBotConfig(true);
    });
  }

  // toggle stop / start bot button
  const btnBotToggle = document.getElementById("btn-bot-toggle-state");
  if (btnBotToggle) {
    btnBotToggle.addEventListener("click", function () {
      if (isStopped) {
        postJSON("/api/bot/reconnect", {}, function (err) {
          if (err) showAlert("error", "Failed to start bot: " + err);
          else {
            showAlert("success", "Starting bot...");
            updateStatus();
          }
        });
      } else {
        postJSON("/api/bot/stop", {}, function (err) {
          if (err) showAlert("error", "Failed to stop bot: " + err);
          else {
            showAlert("success", "Bot stopped successfully.");
            updateStatus();
          }
        });
      }
    });
  }

  // manual reconnect button
  const btnReconnect = document.getElementById("btn-manual-reconnect");
  if (btnReconnect) {
    btnReconnect.addEventListener("click", function () {
      postJSON("/api/bot/reconnect", {}, function (err) {
        if (err) showAlert("error", "Reconnect failed: " + err);
        else showAlert("success", "Reconnection signal sent");
      });
    });
  }

  // quick avatar form
  const avatarForm = document.getElementById("form-quick-avatar");
  if (avatarForm) {
    avatarForm.addEventListener("submit", function (e) {
      e.preventDefault();
      const av = document.getElementById("input-quick-avatar").value.trim();
      if (!av) return;
      postJSON("/api/bot/avatar", { avatar: av }, function (err) {
        if (err) showAlert("error", "Avatar change failed: " + err);
        else {
          showAlert("success", "Avatar set to " + av);
          document.getElementById("input-quick-avatar").value = "";
        }
      });
    });
  }

  // toggle password visibility buttons
  const btnTogglePw = document.getElementById("btn-toggle-pw");
  if (btnTogglePw) {
    btnTogglePw.addEventListener("click", function () {
      const pwInput = document.getElementById("cfg-password");
      if (!pwInput) return;
      if (pwInput.type === "password") {
        pwInput.type = "text";
        this.textContent = "Hide";
      } else {
        pwInput.type = "password";
        this.textContent = "Show";
      }
    });
  }

  const btnToggleLoginPw = document.getElementById("btn-toggle-login-pw");
  if (btnToggleLoginPw) {
    btnToggleLoginPw.addEventListener("click", function () {
      const pwInput = document.getElementById("input-login-password");
      if (!pwInput) return;
      if (pwInput.type === "password") {
        pwInput.type = "text";
        this.textContent = "Hide";
      } else {
        pwInput.type = "password";
        this.textContent = "Show";
      }
    });
  }

  // bot login tool form
  const loginForm = document.getElementById("form-bot-login");
  const loginFeedback = document.getElementById("bot-login-feedback");
  if (loginForm) {
    loginForm.addEventListener("submit", function (e) {
      e.preventDefault();
      const user = document.getElementById("input-login-username").value.trim();
      const pass = document.getElementById("input-login-password").value;

      if (!user) {
        showAlert("error", "Username cannot be empty");
        return;
      }

      if (loginFeedback) {
        loginFeedback.style.display = "block";
        loginFeedback.innerHTML = '<span style="color:var(--text-muted);">Sending login request...</span>';
      }

      postJSON("/api/bot/login", { username: user, password: pass }, function (err) {
        if (err) {
          showAlert("error", "Login error: " + err);
          if (loginFeedback) {
            loginFeedback.innerHTML = `<span style="color:#f87171;font-weight:600;">Login failed: ${escapeHTML(err)}</span>`;
          }
        } else {
          showAlert("success", "Login submitted for " + user);
          if (loginFeedback) {
            loginFeedback.innerHTML = `<span style="color:#34d399;font-weight:600;">Login initiated for ${escapeHTML(user)}. Check Activity Log for status.</span>`;
          }
          updateStatus();
          updateLogs();
        }
      });
    });
  }

  // restore backup form handler
  const restoreForm = document.getElementById("form-restore-backup");
  const restoreStatus = document.getElementById("backup-status-msg");
  if (restoreForm) {
    restoreForm.addEventListener("submit", function (e) {
      e.preventDefault();
      const fileInput = document.getElementById("input-backup-file");
      if (!fileInput.files || fileInput.files.length === 0) {
        showAlert("error", "Please select a backup file to restore");
        return;
      }

      const file = fileInput.files[0];
      const formData = new FormData();
      formData.append("backupfile", file);

      if (restoreStatus) {
        restoreStatus.style.display = "block";
        restoreStatus.innerHTML = '<span style="color:var(--text-muted);">Restoring configuration from backup...</span>';
      }

      fetch("/api/backup/restore", {
        method: "POST",
        body: formData,
      })
        .then((res) => res.json())
        .then((data) => {
          if (data.ok) {
            showAlert("success", "Backup restored successfully!");
            if (restoreStatus) {
              restoreStatus.innerHTML = '<span style="color:#34d399;font-weight:600;">Backup restored successfully! Updating configuration...</span>';
            }
            fileInput.value = "";
            updateStatus(true);
            updateLogs();
          } else {
            showAlert("error", "Restore failed: " + (data.error || "unknown error"));
            if (restoreStatus) {
              restoreStatus.innerHTML = `<span style="color:#f87171;font-weight:600;">Restore error: ${escapeHTML(data.error || "unknown error")}</span>`;
            }
          }
        })
        .catch((err) => {
          showAlert("error", "Network error restoring backup: " + err);
          if (restoreStatus) {
            restoreStatus.innerHTML = `<span style="color:#f87171;font-weight:600;">Network error: ${escapeHTML(err.message || err)}</span>`;
          }
        });
    });
  }

  // logout handlers
  function handleLogout() {
    postJSON("/api/auth/logout", {}, function () {
      window.location.href = "/login";
    });
  }

  const btnLogout = document.getElementById("btn-logout");
  if (btnLogout) btnLogout.addEventListener("click", handleLogout);
  const mobileBtnLogout = document.getElementById("mobile-btn-logout");
  if (mobileBtnLogout) mobileBtnLogout.addEventListener("click", handleLogout);

  // save configuration helper
  function saveBotConfig(reconnect) {
    const host = document.getElementById("cfg-server-host").value.trim();
    const port = parseInt(document.getElementById("cfg-server-port").value.trim(), 10) || 443;
    const id = document.getElementById("cfg-server-id").value.trim();
    const ssl = document.getElementById("cfg-server-ssl").checked;
    const user = document.getElementById("cfg-username").value.trim();
    const pass = document.getElementById("cfg-password").value;
    const avatar = document.getElementById("cfg-avatar").value.trim();
    const cmdChar = document.getElementById("cfg-command-char") ? document.getElementById("cfg-command-char").value.trim() : ".";
    const roomsRaw = document.getElementById("cfg-rooms") ? document.getElementById("cfg-rooms").value.trim() : "";
    const autoBattle = document.getElementById("cfg-auto-battle") ? document.getElementById("cfg-auto-battle").checked : false;
    const autoLeave = document.getElementById("cfg-auto-leave-battle") ? document.getElementById("cfg-auto-leave-battle").checked : true;
    const maxBattles = parseInt(document.getElementById("cfg-max-battles") ? document.getElementById("cfg-max-battles").value.trim() : "1", 10) || 0;
    const startMsg = document.getElementById("cfg-battle-start-msg") ? document.getElementById("cfg-battle-start-msg").value.trim() : "";
    const winMsg = document.getElementById("cfg-battle-win-msg") ? document.getElementById("cfg-battle-win-msg").value.trim() : "";
    const loseMsg = document.getElementById("cfg-battle-lose-msg") ? document.getElementById("cfg-battle-lose-msg").value.trim() : "";
    const formatsRaw = document.getElementById("cfg-battle-formats").value.trim();
    const team = document.getElementById("cfg-battle-team").value.trim();
    const adminPw = document.getElementById("cfg-web-admin-password") ? document.getElementById("cfg-web-admin-password").value : "";

    const formats = formatsRaw ? formatsRaw.split(",").map((f) => f.trim()).filter((f) => f !== "") : [];
    const rooms = roomsRaw ? roomsRaw.split(",").map((r) => r.trim()).filter((r) => r !== "") : [];

    const payload = {
      server_id: id,
      server_host: host,
      server_port: port,
      server_ssl: ssl,
      username: user,
      password: pass,
      avatar: avatar,
      command_char: cmdChar,
      rooms: rooms,
      auto_battle: autoBattle,
      auto_leave_battle: autoLeave,
      max_battles: maxBattles,
      battle_start_msg: startMsg,
      battle_win_msg: winMsg,
      battle_lose_msg: loseMsg,
      battle_formats: formats,
      battle_team: team,
      web_admin_password: adminPw,
      reconnect: reconnect,
    };

    postJSON("/api/config/update", payload, function (err, res) {
      if (err) {
        showAlert("error", "Failed to save configuration: " + err);
      } else {
        showAlert("success", reconnect ? "Configuration saved! Bot is reconnecting..." : "Configuration saved successfully!");
        updateStatus();
      }
    });
  }

  // helper post function
  function postJSON(url, body, callback) {
    fetch(url, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body),
    })
      .then((res) => {
        return res.json().then((data) => {
          if (!res.ok) {
            callback(data.error || ("HTTP status " + res.status));
          } else {
            callback(null, data);
          }
        });
      })
      .catch((err) => callback(err.message || err));
  }

  // alert banner helper
  function showAlert(type, msg) {
    const alertBox = document.getElementById("global-alert");
    if (!alertBox) return;
    alertBox.className = "alert " + type;
    alertBox.textContent = msg;
    alertBox.style.display = "block";
    setTimeout(() => {
      alertBox.style.display = "none";
    }, 4000);
  }

  // escape html helper
  function escapeHTML(str) {
    if (typeof str !== "string") return "" + str;
    return str
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;")
      .replace(/'/g, "&#039;");
  }

  // initial fetch & interval loops
  updateStatus(true);
  updateLogs();
  fetchFormats();
  setInterval(updateStatus, 3000);
  setInterval(updateLogs, 3000);
});
