// ghosthaze thinker control panel client script

document.addEventListener("DOMContentLoaded", function () {
  // tab navigation switching
  const tabs = document.querySelectorAll(".nav-tab-btn");
  const panes = document.querySelectorAll(".tab-pane");

  tabs.forEach(function (tab) {
    tab.addEventListener("click", function () {
      const target = this.getAttribute("data-tab");

      tabs.forEach((t) => t.classList.remove("active"));
      panes.forEach((p) => p.classList.remove("active"));

      this.classList.add("active");
      const activePane = document.getElementById("tab-" + target);
      if (activePane) {
        activePane.classList.add("active");
      }
    });
  });

  // state variables for smooth uptime timer
  let isConnected = false;
  let connectionTimeInitialMs = 0;
  let activeLogFilter = "all";
  let cachedLogs = [];

  // format seconds into smooth human readable duration
  function formatDuration(seconds) {
    if (seconds <= 0) return "0 seconds";
    const parts = [];
    const s = seconds % 60;
    const m = Math.floor(seconds / 60) % 60;
    const h = Math.floor(seconds / 3600) % 24;
    const d = Math.floor(seconds / 86400);

    if (d > 0) parts.push(d + (d === 1 ? " day" : " days"));
    if (h > 0) parts.push(h + (h === 1 ? " hour" : " hours"));
    if (m > 0) parts.push(m + (m === 1 ? " minute" : " minutes"));
    if (s > 0 || parts.length === 0) parts.push(s + (s === 1 ? " second" : " seconds"));
    return parts.join(", ");
  }

  // smooth 1-second ticker for uptime
  function tickUptime() {
    const el = document.getElementById("stat-uptime");
    const elDetail = document.getElementById("uptime-detail");

    if (!isConnected || !connectionTimeInitialMs) {
      if (el) el.textContent = isConnected ? "Connecting..." : "Offline";
      if (elDetail) elDetail.textContent = isConnected ? "Connecting..." : "Not connected";
      return;
    }

    const elapsedSec = Math.max(0, Math.floor((Date.now() - connectionTimeInitialMs) / 1000));
    const formatted = formatDuration(elapsedSec);

    if (el) el.textContent = formatted;
    if (elDetail) elDetail.textContent = formatted;
  }

  setInterval(tickUptime, 1000);

  // periodic status polling
  function updateStatus(isInitial) {
    fetch("/api/status")
      .then((res) => res.json())
      .then((data) => {
        isConnected = data.connected;

        if (data.connected && data.connected_at_ms > 0) {
          connectionTimeInitialMs = data.connected_at_ms;
        } else if (data.connected && data.uptime_seconds > 0) {
          connectionTimeInitialMs = Date.now() - data.uptime_seconds * 1000;
        } else {
          connectionTimeInitialMs = 0;
        }

        tickUptime();

        const dot = document.getElementById("status-dot");
        const statusText = document.getElementById("status-text");
        const statConn = document.getElementById("stat-conn");
        const statServer = document.getElementById("stat-server");
        const statUser = document.getElementById("stat-user");
        const statRooms = document.getElementById("stat-rooms");
        const statBattles = document.getElementById("stat-battles");
        const headerServerID = document.getElementById("header-server-id");

        if (headerServerID) {
          headerServerID.textContent = data.server_id || "showdown";
        }

        if (dot && statusText) {
          if (data.connected) {
            dot.className = "status-dot online";
            statusText.textContent = data.logged_in ? "Online (" + data.username + ")" : "Authenticating...";
          } else {
            dot.className = "status-dot offline";
            statusText.textContent = "Offline";
          }
        }

        if (statConn) {
          statConn.textContent = data.connected ? (data.logged_in ? "Connected" : "Authenticating") : "Disconnected";
        }
        if (statServer) {
          statServer.textContent = (data.server_id || "showdown") + " (" + (data.server_host || "sim3.psim.us") + ":" + (data.server_port || 443) + ")";
        }
        if (statUser) {
          statUser.textContent = data.username || "Guest";
        }
        if (statRooms) {
          statRooms.textContent = data.rooms ? data.rooms.length : 0;
        }
        if (statBattles) {
          statBattles.textContent = data.active_battles ? data.active_battles.length : 0;
        }

        renderRooms(data.rooms || []);
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
    const autoBattleEl = document.getElementById("cfg-auto-battle");
    const autoLeaveEl = document.getElementById("cfg-auto-leave-battle");
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
    const roomsEl = document.getElementById("cfg-rooms");
    if (roomsEl) roomsEl.value = data.config_rooms ? data.config_rooms.join(", ") : (data.rooms ? data.rooms.join(", ") : "");
    if (autoBattleEl) autoBattleEl.checked = !!data.auto_battle;
    if (autoLeaveEl) autoLeaveEl.checked = data.auto_leave_battle !== false;
    const maxBattlesEl = document.getElementById("cfg-max-battles");
    if (maxBattlesEl) maxBattlesEl.value = data.max_battles !== undefined ? data.max_battles : 1;
    if (winMsgEl) winMsgEl.value = data.battle_win_msg || "";
    if (loseMsgEl) loseMsgEl.value = data.battle_lose_msg || "";
    if (formatsEl) formatsEl.value = data.battle_formats ? data.battle_formats.join(", ") : "gen9randombattle";
    if (teamEl) teamEl.value = data.battle_team || "";
  }

  // render active room table
  function renderRooms(rooms) {
    const tbody = document.getElementById("rooms-table-body");
    if (!tbody) return;

    if (rooms.length === 0) {
      tbody.innerHTML = '<tr><td colspan="3" style="text-align:center;color:var(--text-dim);padding:16px;">No active rooms joined</td></tr>';
      return;
    }

    let html = "";
    rooms.forEach((r) => {
      html += `<tr>
        <td><strong>${escapeHTML(r)}</strong></td>
        <td><span class="chip" style="background:var(--success-light);color:#34d399;">Active</span></td>
        <td style="text-align:right;">
          <button class="btn btn-secondary btn-sm btn-quick-msg" data-room="${escapeHTML(r)}" style="margin-right:6px;">Message</button>
          <button class="btn btn-danger btn-sm btn-leave-room" data-room="${escapeHTML(r)}">Leave</button>
        </td>
      </tr>`;
    });
    tbody.innerHTML = html;

    // attach room action handlers
    tbody.querySelectorAll(".btn-leave-room").forEach((btn) => {
      btn.addEventListener("click", function () {
        const roomName = this.getAttribute("data-room");
        leaveRoom(roomName);
      });
    });

    tbody.querySelectorAll(".btn-quick-msg").forEach((btn) => {
      btn.addEventListener("click", function () {
        const roomName = this.getAttribute("data-room");
        const msg = prompt("Send message to " + roomName + ":");
        if (msg && msg.trim()) {
          postJSON("/api/send", { target: roomName, message: msg.trim(), is_pm: false }, function (err) {
            if (err) showAlert("error", "Failed to send: " + err);
            else showAlert("success", "Message sent to " + roomName);
          });
        }
      });
    });
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
          <button class="btn btn-secondary btn-sm btn-forfeit-battle" data-room="${escapeHTML(b.room)}" style="margin-right:6px;">Forfeit</button>
          <button class="btn btn-danger btn-sm btn-leave-battle" data-room="${escapeHTML(b.room)}">Leave</button>
        </td>
      </tr>`;
    });
    html += "</tbody></table></div>";
    container.innerHTML = html;

    // attach battle forfeit and leave handlers
    container.querySelectorAll(".btn-forfeit-battle").forEach((btn) => {
      btn.addEventListener("click", function () {
        const roomName = this.getAttribute("data-room");
        if (confirm("Are you sure you want the bot to forfeit battle " + roomName + "?")) {
          postJSON("/api/battles/forfeit", { room: roomName }, function (err) {
            if (err) showAlert("error", "Failed to forfeit battle: " + err);
            else {
              showAlert("success", "Forfeited and left battle " + roomName);
              updateStatus();
            }
          });
        }
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

  // render activity logs with filtering
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

  // fetch activity logs
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

  // log filter buttons
  document.querySelectorAll(".log-filter-btn").forEach((btn) => {
    btn.addEventListener("click", function () {
      document.querySelectorAll(".log-filter-btn").forEach((b) => b.classList.remove("active"));
      this.classList.add("active");
      activeLogFilter = this.getAttribute("data-filter") || "all";
      renderLogs();
    });
  });

  // clear logs button
  const clearLogsBtn = document.getElementById("btn-clear-logs");
  if (clearLogsBtn) {
    clearLogsBtn.addEventListener("click", function () {
      cachedLogs = [];
      renderLogs();
    });
  }

  // leave room helper
  function leaveRoom(room) {
    if (!confirm("Leave room " + room + "?")) return;
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

  // bot-send form
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
            document.getElementById("res-tls").textContent = data.https ? "YES (TLS)" : "NO";
            document.getElementById("res-ws").textContent = data.websocket_url;
            document.getElementById("res-login").textContent = data.login_url;

            // attach quick apply button
            const applyBtn = document.getElementById("btn-apply-discovered");
            if (applyBtn) {
              applyBtn.onclick = function () {
                document.getElementById("cfg-server-host").value = data.host;
                document.getElementById("cfg-server-port").value = data.port;
                document.getElementById("cfg-server-id").value = data.id;
                document.getElementById("cfg-server-ssl").checked = !!data.https;
                showAlert("success", "Applied discovered server values to Configuration tab!");
              };
            }
          }
        }
      });
    });
  }

  // quick set default showdown values
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

  // save and reconnect button
  const btnSaveReconnect = document.getElementById("btn-save-reconnect");
  if (btnSaveReconnect) {
    btnSaveReconnect.addEventListener("click", function () {
      if (confirm("Save configuration and reconnect the bot immediately?")) {
        saveBotConfig(true);
      }
    });
  }

  // manual reconnect button
  const btnReconnect = document.getElementById("btn-manual-reconnect");
  if (btnReconnect) {
    btnReconnect.addEventListener("click", function () {
      if (confirm("Reconnect bot now?")) {
        postJSON("/api/bot/reconnect", {}, function (err) {
          if (err) showAlert("error", "Reconnect failed: " + err);
          else showAlert("success", "Reconnection signal sent");
        });
      }
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

  // toggle password visibility
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
  if (loginForm) {
    loginForm.addEventListener("submit", function (e) {
      e.preventDefault();
      const user = document.getElementById("input-login-username").value.trim();
      const pass = document.getElementById("input-login-password").value;

      if (!user) {
        showAlert("error", "Username cannot be empty");
        return;
      }

      postJSON("/api/bot/login", { username: user, password: pass }, function (err) {
        if (err) {
          showAlert("error", "Login error: " + err);
        } else {
          showAlert("success", "Login submitted for " + user);
          updateStatus();
          updateLogs();
        }
      });
    });
  }

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
    const winMsg = document.getElementById("cfg-battle-win-msg") ? document.getElementById("cfg-battle-win-msg").value.trim() : "";
    const loseMsg = document.getElementById("cfg-battle-lose-msg") ? document.getElementById("cfg-battle-lose-msg").value.trim() : "";
    const formatsRaw = document.getElementById("cfg-battle-formats").value.trim();
    const team = document.getElementById("cfg-battle-team").value.trim();

    const formats = formatsRaw.split(",").map((f) => f.trim()).filter((f) => f !== "");
    const rooms = roomsRaw.split(",").map((r) => r.trim()).filter((r) => r !== "");

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
      battle_win_msg: winMsg,
      battle_lose_msg: loseMsg,
      battle_formats: formats,
      battle_team: team,
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

  // alert message banner
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
  setInterval(updateStatus, 3000);
  setInterval(updateLogs, 3000);
});
