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
      fetchBattleHistory();
    }
    if (target === "teams") {
      fetchTeams();
    }
    if (target === "commands") {
      fetchCommands();
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
            if (data.is_guest) {
              statusText.textContent = "Online (Guest)";
            } else if (data.logged_in) {
              statusText.textContent = "Online (" + (data.username || "Bot") + ")";
            } else {
              statusText.textContent = "Connecting...";
            }
          } else {
            dot.className = "status-dot offline";
            statusText.textContent = "Offline";
          }
          if (statusBadge) {
            statusBadge.title = statusText.textContent;
          }
        }

        // banner notice for bot connection state
        const guestBanner = document.getElementById("guest-notice-banner");
        if (guestBanner) {
          if (isStopped || !data.config_username) {
            guestBanner.style.display = "block";
            guestBanner.innerHTML = '<strong>Notice:</strong> Bot is not connected. Please add your bot login details in the <a href="javascript:void(0)" onclick="document.querySelector(\'[data-tab=\\\'config\\\']\').click();" style="color:var(--primary);text-decoration:underline;font-weight:600;">Configuration</a> tab or <a href="javascript:void(0)" onclick="document.querySelector(\'[data-tab=\\\'tools\\\']\').click();" style="color:var(--primary);text-decoration:underline;font-weight:600;">Bot Login Tool</a> to connect your bot to the server.';
          } else if (isConnected && data.is_guest) {
            guestBanner.style.display = "block";
            guestBanner.innerHTML = '<strong>Notice:</strong> The bot is currently connected as an anonymous Guest. Pokémon Showdown requires a registered bot account (username &amp; password) to accept battle challenges and talk from cloud hosting. Please configure credentials in the <a href="javascript:void(0)" onclick="document.querySelector(\'[data-tab=\\\'config\\\']\').click();" style="color:var(--primary);text-decoration:underline;font-weight:600;">Configuration</a> tab or <a href="javascript:void(0)" onclick="document.querySelector(\'[data-tab=\\\'tools\\\']\').click();" style="color:var(--primary);text-decoration:underline;font-weight:600;">Bot Login Tool</a>.';
          } else {
            guestBanner.style.display = "none";
          }
        }

        // overview cards
        const statConn = document.getElementById("stat-conn");
        const statServer = document.getElementById("stat-server");
        const statUser = document.getElementById("stat-user");
        const statRooms = document.getElementById("stat-rooms");
        const statBattles = document.getElementById("stat-battles");

        if (statConn) {
          if (isStopped) statConn.textContent = "Not Connected";
          else if (isConnected) {
            if (data.is_guest) statConn.textContent = "Connected (Guest)";
            else if (data.logged_in) statConn.textContent = "Connected";
            else statConn.textContent = "Authenticating";
          }
          else statConn.textContent = "Not Connected";
        }

        if (statServer) {
          statServer.textContent = (data.server_id || "showdown") + " (" + (data.server_host || "sim3.psim.us") + ":" + (data.server_port || 443) + ")";
        }

        if (statUser) {
          if (isStopped || !data.config_username) {
            statUser.textContent = data.config_username || "Not Configured";
          } else if (data.is_guest) {
            statUser.innerHTML = escapeHTML(data.username || "Guest") + ' <span class="badge" style="background:rgba(245,158,11,0.2);color:#d97706;border:1px solid rgba(245,158,11,0.4);font-size:10px;padding:2px 6px;margin-left:4px;border-radius:4px;" title="Registered account required on Pokémon Showdown to accept challenges and battle">Guest</span>';
          } else {
            statUser.textContent = data.username || "Guest";
          }
        }

        if (statRooms) {
          statRooms.textContent = data.chat_rooms_count !== undefined ? data.chat_rooms_count : (data.rooms ? data.rooms.length : 0);
        }

        if (statBattles) {
          statBattles.textContent = data.active_battles_count !== undefined ? data.active_battles_count : (data.active_battles ? data.active_battles.length : 0);
        }

        const statWinRate = document.getElementById("stat-winrate");
        const statWinRateSub = document.getElementById("stat-winrate-sub");
        if (statWinRate) {
          const rate = data.win_rate !== undefined ? data.win_rate : 0;
          statWinRate.textContent = rate + "%";
        }
        if (statWinRateSub) {
          const w = data.wins || 0;
          const l = data.losses || 0;
          const tot = data.total_battles || 0;
          statWinRateSub.textContent = w + "W / " + l + "L (" + tot + " battles)";
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

        if (cachedFormats.length === 0) {
          fetchFormats();
        }

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
    if (userEl) {
      const u = data.config_username || data.username || "";
      userEl.value = u.toLowerCase().startsWith("guest") ? "" : u;
    }
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
        <td><span class="chip" style="background:var(--success-light);color:#34d399;border-color:rgba(16,185,129,0.3);">Active</span></td>
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
          <div class="table-actions">
            <a href="https://play.pokemonshowdown.com/${escapeHTML(b.room)}" target="_blank" class="btn btn-secondary btn-sm">Watch</a>
            <button type="button" class="btn btn-secondary btn-sm btn-forfeit-battle" data-room="${escapeHTML(b.room)}">Forfeit</button>
            <button type="button" class="btn btn-danger btn-sm btn-leave-battle" data-room="${escapeHTML(b.room)}">Leave</button>
          </div>
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

  // fetch and render match history
  function fetchBattleHistory() {
    fetch("/api/battles/history?limit=50")
      .then((res) => res.json())
      .then((data) => {
        renderBattleHistory(data.records || [], data.stats || {});
      })
      .catch((err) => console.error("failed to fetch battle history", err));
  }

  function renderBattleHistory(records, stats) {
    const container = document.getElementById("battle-history-container");
    if (!container) return;

    if (!records || records.length === 0) {
      container.innerHTML = '<p style="color:var(--text-dim);text-align:center;padding:24px;">No completed battles recorded yet.</p>';
      return;
    }

    let html = '<div class="table-wrapper"><table class="data-table"><thead><tr><th>Time</th><th>Format</th><th>Opponent</th><th>Turns</th><th>Result</th><th style="text-align:right;">Replay</th></tr></thead><tbody>';
    records.forEach((r) => {
      const outcome = (r.outcome || "loss").toLowerCase();
      let badgeClass = "badge-danger";
      let badgeText = "Loss";
      if (outcome === "win") {
        badgeClass = "badge-success";
        badgeText = "Victory";
      } else if (outcome === "tie") {
        badgeClass = "badge-secondary";
        badgeText = "Tie";
      }

      const dateStr = r.finished_at ? new Date(r.finished_at).toLocaleTimeString() : "-";
      const replayLink = r.replay_url || ("https://replay.pokemonshowdown.com/" + (r.room ? r.room.replace("battle-", "") : ""));

      html += `<tr>
        <td style="color:var(--text-dim);font-size:12px;">${escapeHTML(dateStr)}</td>
        <td><span class="chip">${escapeHTML(r.format || "custom")}</span></td>
        <td><strong>${escapeHTML(r.opponent || "Unknown")}</strong></td>
        <td>${escapeHTML(r.turns || 0)}</td>
        <td><span class="badge ${badgeClass}">${badgeText}</span></td>
        <td style="text-align:right;">
          <a href="${escapeHTML(replayLink)}" target="_blank" class="btn btn-secondary btn-sm" title="View Showdown Replay">Replay</a>
        </td>
      </tr>`;
    });
    html += "</tbody></table></div>";
    container.innerHTML = html;
  }

  const btnClearHistory = document.getElementById("btn-clear-history");
  if (btnClearHistory) {
    btnClearHistory.addEventListener("click", function () {
      postJSON("/api/battles/history/clear", {}, function (err) {
        if (err) showAlert("error", "Failed to clear history: " + err);
        else {
          showAlert("success", "Match history cleared.");
          fetchBattleHistory();
          updateStatus();
        }
      });
    });
  }

  // teams vault handlers
  function fetchTeams() {
    fetch("/api/teams")
      .then((res) => res.json())
      .then((teams) => renderTeams(teams || []))
      .catch((err) => console.error("failed to fetch teams", err));
  }

  function renderTeams(teams) {
    const container = document.getElementById("teams-list-container");
    if (!container) return;

    if (!teams || teams.length === 0) {
      container.innerHTML = '<p style="color:var(--text-dim);text-align:center;padding:32px;">No battle teams saved in the vault yet. Click <strong>+ New Team</strong> above to add your first team!</p>';
      return;
    }

    let html = '<div class="teams-grid" style="display:grid;grid-template-columns:repeat(auto-fill, minmax(320px, 1fr));gap:16px;">';
    teams.forEach((t) => {
      const pokes = t.pokemon || [];
      const pokeBadges = pokes.map((p) => `<span class="badge" style="background:var(--muted);color:var(--text-main);border:1px solid var(--border);font-size:11px;padding:3px 6px;border-radius:4px;">${escapeHTML(p)}</span>`).join(" ");

      html += `<div class="card team-card" style="padding:16px;border:1px solid var(--border);border-radius:var(--radius);background:var(--card);">
        <div style="display:flex;align-items:flex-start;justify-content:space-between;gap:8px;margin-bottom:10px;">
          <div>
            <h4 style="margin:0 0 4px 0;font-size:15px;font-weight:600;">${escapeHTML(t.name)}</h4>
            <span class="chip">${escapeHTML(t.format)}</span>
          </div>
          <div style="display:flex;align-items:center;gap:8px;">
            <label class="switch" title="Active for challenges">
              <input type="checkbox" class="team-toggle-active" data-id="${escapeHTML(t.id)}" ${t.active ? "checked" : ""}>
              <span class="slider"></span>
            </label>
            <button type="button" class="btn btn-secondary btn-sm btn-delete-team" data-id="${escapeHTML(t.id)}" title="Delete Team" style="padding:4px 8px;color:var(--destructive);">&times;</button>
          </div>
        </div>

        <div style="display:flex;flex-wrap:wrap;gap:6px;margin:12px 0;">
          ${pokeBadges || '<span style="color:var(--text-dim);font-size:12px;">Custom set</span>'}
        </div>

        <details style="margin-top:10px;">
          <summary style="font-size:12px;color:var(--text-dim);cursor:pointer;user-select:none;">View Pokepaste Text</summary>
          <pre style="background:var(--muted);padding:10px;border-radius:var(--radius-sm);font-size:11px;overflow-x:auto;max-height:160px;margin-top:8px;white-space:pre-wrap;">${escapeHTML(t.team_raw || t.team_packed || "")}</pre>
        </details>
      </div>`;
    });
    html += "</div>";
    container.innerHTML = html;

    // toggle handlers
    container.querySelectorAll(".team-toggle-active").forEach((sw) => {
      sw.addEventListener("change", function () {
        const teamId = this.getAttribute("data-id");
        postJSON("/api/teams/toggle", { id: teamId }, (err) => {
          if (err) showAlert("error", "Failed to toggle team: " + err);
          else showAlert("success", "Updated team active status");
        });
      });
    });

    // delete handlers
    container.querySelectorAll(".btn-delete-team").forEach((btn) => {
      btn.addEventListener("click", function () {
        const teamId = this.getAttribute("data-id");
        postJSON("/api/teams/delete", { id: teamId }, (err) => {
          if (err) showAlert("error", "Failed to delete team: " + err);
          else {
            showAlert("success", "Team deleted from vault");
            fetchTeams();
          }
        });
      });
    });
  }

  // teams form toggle and submit
  const btnShowAddTeam = document.getElementById("btn-show-add-team");
  const btnCancelTeam = document.getElementById("btn-cancel-team");
  const teamFormContainer = document.getElementById("team-form-container");
  const formBattleTeam = document.getElementById("form-battle-team");

  if (btnShowAddTeam && teamFormContainer) {
    btnShowAddTeam.addEventListener("click", function () {
      const isVisible = teamFormContainer.style.display !== "none";
      teamFormContainer.style.display = isVisible ? "none" : "block";
      if (!isVisible) {
        document.getElementById("input-team-name")?.focus();
      }
    });
  }

  if (btnCancelTeam && teamFormContainer) {
    btnCancelTeam.addEventListener("click", function () {
      teamFormContainer.style.display = "none";
    });
  }

  if (formBattleTeam) {
    formBattleTeam.addEventListener("submit", function (e) {
      e.preventDefault();
      const payload = {
        name: document.getElementById("input-team-name")?.value.trim() || "",
        format: document.getElementById("input-team-format")?.value.trim() || "",
        team_raw: document.getElementById("input-team-raw")?.value.trim() || "",
        active: document.getElementById("switch-team-active")?.checked ?? true,
      };

      if (!payload.name || !payload.format || !payload.team_raw) {
        showAlert("error", "Please fill in team name, format, and pokepaste export text.");
        return;
      }

      postJSON("/api/teams/save", payload, function (err) {
        if (err) showAlert("error", "Failed to save team: " + err);
        else {
          showAlert("success", "Battle team saved to vault!");
          formBattleTeam.reset();
          if (teamFormContainer) teamFormContainer.style.display = "none";
          fetchTeams();
        }
      });
    });
  }

  // custom commands handlers
  function fetchCommands() {
    fetch("/api/commands")
      .then((res) => res.json())
      .then((cmds) => renderCommands(cmds || []))
      .catch((err) => console.error("failed to fetch commands", err));
  }

  function renderCommands(commands) {
    const container = document.getElementById("commands-list-container");
    if (!container) return;

    if (!commands || commands.length === 0) {
      container.innerHTML = '<p style="color:var(--text-dim);text-align:center;padding:32px;">No dynamic custom commands created yet. Click <strong>+ New Command</strong> above to create your first command!</p>';
      return;
    }

    let html = '<div class="table-wrapper"><table class="data-table"><thead><tr><th>Trigger</th><th>Rank</th><th>Scope</th><th>Response</th><th>Active</th><th style="text-align:right;">Action</th></tr></thead><tbody>';
    commands.forEach((c) => {
      const scope = (c.rooms && c.rooms.length > 0) ? c.rooms.join(", ") : "All Rooms & PMs";
      const rankText = c.min_rank === "all" ? "Anyone" : `${c.min_rank}+`;

      html += `<tr>
        <td><strong>.${escapeHTML(c.name)}</strong></td>
        <td><span class="chip">${escapeHTML(rankText)}</span></td>
        <td style="color:var(--text-dim);font-size:12px;">${escapeHTML(scope)}</td>
        <td style="max-width:280px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;" title="${escapeHTML(c.response)}">${escapeHTML(c.response)}</td>
        <td>
          <label class="switch">
            <input type="checkbox" class="cmd-toggle-enabled" data-name="${escapeHTML(c.name)}" ${c.enabled ? "checked" : ""}>
            <span class="slider"></span>
          </label>
        </td>
        <td style="text-align:right;">
          <button type="button" class="btn btn-secondary btn-sm btn-delete-command" data-name="${escapeHTML(c.name)}" title="Delete Command" style="color:var(--destructive);">&times;</button>
        </td>
      </tr>`;
    });
    html += "</tbody></table></div>";
    container.innerHTML = html;

    // toggle handlers
    container.querySelectorAll(".cmd-toggle-enabled").forEach((sw) => {
      sw.addEventListener("change", function () {
        const cmdName = this.getAttribute("data-name");
        postJSON("/api/commands/toggle", { name: cmdName }, (err) => {
          if (err) showAlert("error", "Failed to toggle command: " + err);
          else showAlert("success", "Updated command status");
        });
      });
    });

    // delete handlers
    container.querySelectorAll(".btn-delete-command").forEach((btn) => {
      btn.addEventListener("click", function () {
        const cmdName = this.getAttribute("data-name");
        postJSON("/api/commands/delete", { name: cmdName }, (err) => {
          if (err) showAlert("error", "Failed to delete command: " + err);
          else {
            showAlert("success", "Command deleted");
            fetchCommands();
          }
        });
      });
    });
  }

  // command form toggle and submit
  const btnShowAddCommand = document.getElementById("btn-show-add-command");
  const btnCancelCommand = document.getElementById("btn-cancel-command");
  const commandFormContainer = document.getElementById("command-form-container");
  const formCustomCommand = document.getElementById("form-custom-command");

  if (btnShowAddCommand && commandFormContainer) {
    btnShowAddCommand.addEventListener("click", function () {
      const isVisible = commandFormContainer.style.display !== "none";
      commandFormContainer.style.display = isVisible ? "none" : "block";
      if (!isVisible) {
        document.getElementById("input-cmd-name")?.focus();
      }
    });
  }

  if (btnCancelCommand && commandFormContainer) {
    btnCancelCommand.addEventListener("click", function () {
      commandFormContainer.style.display = "none";
    });
  }

  if (formCustomCommand) {
    formCustomCommand.addEventListener("submit", function (e) {
      e.preventDefault();
      const rawRooms = document.getElementById("input-cmd-rooms")?.value.trim() || "";
      const rooms = rawRooms ? rawRooms.split(",").map((s) => s.trim()).filter(Boolean) : [];

      const payload = {
        name: document.getElementById("input-cmd-name")?.value.trim() || "",
        response: document.getElementById("input-cmd-response")?.value.trim() || "",
        min_rank: document.getElementById("select-cmd-rank")?.value || "all",
        rooms: rooms,
        enabled: document.getElementById("switch-cmd-enabled")?.checked ?? true,
      };

      if (!payload.name || !payload.response) {
        showAlert("error", "Please enter a command trigger and response message.");
        return;
      }

      postJSON("/api/commands/save", payload, function (err) {
        if (err) showAlert("error", "Failed to save command: " + err);
        else {
          showAlert("success", "Custom command saved successfully!");
          formCustomCommand.reset();
          if (commandFormContainer) commandFormContainer.style.display = "none";
          fetchCommands();
        }
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

  // custom format combobox implementation
  let cachedFormats = [];

  function getSelectedFormat(hiddenInputId, customInputId) {
    const hidden = document.getElementById(hiddenInputId);
    const custom = document.getElementById(customInputId);
    if (hidden && hidden.value === "__custom__" && custom && custom.value.trim()) {
      return custom.value.trim();
    }
    if (hidden && hidden.value && hidden.value !== "__custom__") {
      return hidden.value;
    }
    return "gen9randombattle";
  }

  const comboboxInstances = [
    {
      wrapperId: "combobox-challenge-format",
      btnId: "btn-challenge-format",
      textId: "text-challenge-format",
      inputId: "input-challenge-format",
      dropdownId: "dropdown-challenge-format",
      customId: "input-challenge-format-custom",
    },
    {
      wrapperId: "combobox-quick-challenge-format",
      btnId: "btn-quick-challenge-format",
      textId: "text-quick-challenge-format",
      inputId: "input-quick-challenge-format",
      dropdownId: "dropdown-quick-challenge-format",
      customId: "input-quick-challenge-format-custom",
    },
  ];

  function setComboboxSelection(id, name) {
    comboboxInstances.forEach((inst) => {
      const input = document.getElementById(inst.inputId);
      const text = document.getElementById(inst.textId);
      const custom = document.getElementById(inst.customId);
      if (input) input.value = id;
      if (text) text.textContent = name;
      if (custom) {
        custom.style.display = id === "__custom__" ? "block" : "none";
        if (id === "__custom__") custom.focus();
      }

      const dropdown = document.getElementById(inst.dropdownId);
      if (dropdown) {
        dropdown.querySelectorAll(".combobox-option").forEach((opt) => {
          if (opt.getAttribute("data-id") === id) {
            opt.classList.add("selected");
          } else {
            opt.classList.remove("selected");
          }
        });
      }
    });
  }

  function renderComboboxOptions(inst, formats, query) {
    const dropdown = document.getElementById(inst.dropdownId);
    if (!dropdown) return;
    const list = dropdown.querySelector(".combobox-options-list");
    if (!list) return;

    const q = (query || "").trim().toLowerCase();
    const filtered = q
      ? formats.filter(
          (f) =>
            (f.name && f.name.toLowerCase().includes(q)) ||
            (f.id && f.id.toLowerCase().includes(q)) ||
            (f.section && f.section.toLowerCase().includes(q))
        )
      : formats;

    if (filtered.length === 0) {
      list.innerHTML = `<div style="padding:14px;text-align:center;color:var(--text-dim);font-size:13px;">No formats match "${escapeHTML(q)}"</div>`;
      return;
    }

    const currentId = document.getElementById(inst.inputId)?.value || "gen9randombattle";
    const groups = {};
    filtered.forEach((f) => {
      const id = f.id || f.ID;
      const name = f.name || f.Name || id;
      const sec = f.section || f.Section || "Other Formats";
      if (!id) return;
      if (!groups[sec]) groups[sec] = [];
      groups[sec].push({ id: id, name: name });
    });

    let html = "";
    Object.keys(groups).forEach((sec) => {
      html += `<div class="combobox-section-title">${escapeHTML(sec)}</div>`;
      groups[sec].forEach((f) => {
        const isSel = f.id === currentId ? " selected" : "";
        html += `<div class="combobox-option${isSel}" data-id="${escapeHTML(f.id)}" data-name="${escapeHTML(f.name)}">
          <span>${escapeHTML(f.name)}</span>
        </div>`;
      });
    });

    html += `<div class="combobox-option combobox-option-custom" data-id="__custom__" data-name="Custom Format...">
      <span>+ Custom Format...</span>
    </div>`;

    list.innerHTML = html;

    list.querySelectorAll(".combobox-option").forEach((opt) => {
      opt.addEventListener("click", function (e) {
        e.stopPropagation();
        const id = this.getAttribute("data-id");
        const name = this.getAttribute("data-name");
        setComboboxSelection(id, name);
        const wrapper = document.getElementById(inst.wrapperId);
        if (wrapper) wrapper.classList.remove("open");
      });
    });
  }

  function initComboboxes() {
    comboboxInstances.forEach((inst) => {
      const wrapper = document.getElementById(inst.wrapperId);
      const btn = document.getElementById(inst.btnId);
      const dropdown = document.getElementById(inst.dropdownId);
      if (!wrapper || !btn || !dropdown) return;

      btn.addEventListener("click", function (e) {
        e.stopPropagation();
        const isOpen = wrapper.classList.contains("open");
        document.querySelectorAll(".custom-combobox").forEach((c) => c.classList.remove("open"));
        if (!isOpen) {
          wrapper.classList.add("open");
          const searchInput = dropdown.querySelector(".combobox-search-input");
          if (searchInput) {
            searchInput.value = "";
            renderComboboxOptions(inst, cachedFormats, "");
            setTimeout(() => searchInput.focus(), 50);
          }
        }
      });

      const searchInput = dropdown.querySelector(".combobox-search-input");
      if (searchInput) {
        searchInput.addEventListener("input", function () {
          renderComboboxOptions(inst, cachedFormats, this.value);
        });
        searchInput.addEventListener("click", function (e) {
          e.stopPropagation();
        });
      }
    });

    document.addEventListener("click", function (e) {
      if (!e.target.closest(".custom-combobox")) {
        document.querySelectorAll(".custom-combobox").forEach((c) => c.classList.remove("open"));
      }
    });
  }

  function populateCustomComboboxes(formats) {
    if (!Array.isArray(formats) || formats.length === 0) return;

    const defaultFmt = formats.find((f) => (f.id || f.ID) === "gen9randombattle") || formats[0];
    if (defaultFmt) {
      const defId = defaultFmt.id || defaultFmt.ID;
      const defName = defaultFmt.name || defaultFmt.Name || defId;
      comboboxInstances.forEach((inst) => {
        const input = document.getElementById(inst.inputId);
        const text = document.getElementById(inst.textId);
        if (input && (!input.value || input.value === "gen9randombattle")) {
          input.value = defId;
          if (text) text.textContent = defName;
        }
      });
    }

    comboboxInstances.forEach((inst) => {
      renderComboboxOptions(inst, formats, "");
    });
  }

  initComboboxes();

  // destination combobox component handler
  function initDestinationCombobox() {
    const wrapper = document.getElementById("combobox-send-type");
    const btn = document.getElementById("btn-send-type");
    const dropdown = document.getElementById("dropdown-send-type");
    const input = document.getElementById("select-send-type");
    const text = document.getElementById("text-send-type");
    const targetInput = document.getElementById("input-send-target");
    const targetLabel = document.getElementById("label-send-target");
    if (!wrapper || !btn || !dropdown) return;

    btn.addEventListener("click", function (e) {
      e.stopPropagation();
      const isOpen = wrapper.classList.contains("open");
      document.querySelectorAll(".custom-combobox").forEach((c) => c.classList.remove("open"));
      if (!isOpen) {
        wrapper.classList.add("open");
      }
    });

    dropdown.querySelectorAll(".combobox-option").forEach((opt) => {
      opt.addEventListener("click", function (e) {
        e.stopPropagation();
        const val = this.getAttribute("data-id");
        const name = this.getAttribute("data-name");
        if (input) input.value = val;
        if (text) text.textContent = name;
        dropdown.querySelectorAll(".combobox-option").forEach((o) => o.classList.remove("selected"));
        this.classList.add("selected");
        wrapper.classList.remove("open");

        if (targetInput) {
          if (val === "pm") {
            targetInput.placeholder = "e.g. username";
            if (targetLabel) targetLabel.textContent = "Trainer Username";
          } else {
            targetInput.placeholder = "e.g. lobby";
            if (targetLabel) targetLabel.textContent = "Chatroom Name";
          }
        }
      });
    });
  }

  initDestinationCombobox();

  function fetchFormats() {
    fetch("/api/formats")
      .then((res) => res.json())
      .then((data) => {
        if (Array.isArray(data) && data.length > 0) {
          cachedFormats = data;
          populateCustomComboboxes(cachedFormats);
        }
      })
      .catch(() => {});
  }

  const challengeForm = document.getElementById("form-challenge");
  if (challengeForm) {
    challengeForm.addEventListener("submit", function (e) {
      e.preventDefault();
      const user = document.getElementById("input-challenge-user").value.trim();
      const format = getSelectedFormat("input-challenge-format", "input-challenge-format-custom");

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
      const format = getSelectedFormat("input-quick-challenge-format", "input-quick-challenge-format-custom");

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
    let user = document.getElementById("cfg-username").value.trim();
    if (user.toLowerCase().startsWith("guest")) {
      user = "";
    }
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
  fetchBattleHistory();
  fetchTeams();
  fetchCommands();
  setInterval(updateStatus, 3000);
  setInterval(updateLogs, 3000);
});
