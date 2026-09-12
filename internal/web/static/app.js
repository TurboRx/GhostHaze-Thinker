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

  // periodic status polling
  function updateStatus() {
    fetch("/api/status")
      .then((res) => res.json())
      .then((data) => {
        const dot = document.getElementById("status-dot");
        const statusText = document.getElementById("status-text");
        const statConn = document.getElementById("stat-conn");
        const statServer = document.getElementById("stat-server");
        const statUser = document.getElementById("stat-user");
        const statRooms = document.getElementById("stat-rooms");
        const statBattles = document.getElementById("stat-battles");
        const statUptime = document.getElementById("stat-uptime");

        if (dot && statusText) {
          if (data.connected) {
            dot.className = "status-dot online";
            statusText.textContent = data.logged_in ? "Online (" + data.username + ")" : "Connecting...";
          } else {
            dot.className = "status-dot offline";
            statusText.textContent = "Offline";
          }
        }

        if (statConn) {
          statConn.textContent = data.connected ? (data.logged_in ? "Connected" : "Authenticating") : "Disconnected";
        }
        if (statServer) {
          statServer.textContent = data.server_id + " (" + data.server_host + ")";
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
        if (statUptime) {
          statUptime.textContent = data.uptime || "0s";
        }

        renderRooms(data.rooms || []);
        renderBattles(data.active_battles || []);
      })
      .catch((err) => {
        console.error("status fetch error:", err);
      });
  }

  // render active room table
  function renderRooms(rooms) {
    const tbody = document.getElementById("rooms-table-body");
    if (!tbody) return;

    if (rooms.length === 0) {
      tbody.innerHTML = '<tr><td colspan="3" style="text-align:center;color:var(--text-dim);">No active rooms joined</td></tr>';
      return;
    }

    let html = "";
    rooms.forEach((r) => {
      html += `<tr>
        <td><strong>${escapeHTML(r)}</strong></td>
        <td><span class="chip">Active</span></td>
        <td style="text-align:right;">
          <button class="btn btn-danger btn-sm btn-leave-room" data-room="${escapeHTML(r)}">Leave</button>
        </td>
      </tr>`;
    });
    tbody.innerHTML = html;

    // attach leave handlers
    tbody.querySelectorAll(".btn-leave-room").forEach((btn) => {
      btn.addEventListener("click", function () {
        const roomName = this.getAttribute("data-room");
        leaveRoom(roomName);
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

    let html = '<div class="table-wrapper"><table class="data-table"><thead><tr><th>Room</th><th>Format</th><th>Turn</th><th>Opponent</th></tr></thead><tbody>';
    battles.forEach((b) => {
      html += `<tr>
        <td><strong>${escapeHTML(b.room)}</strong></td>
        <td>${escapeHTML(b.format || "random")}</td>
        <td>${escapeHTML(b.turn || 0)}</td>
        <td>${escapeHTML(b.opponent || "Unknown")}</td>
      </tr>`;
    });
    html += "</tbody></table></div>";
    container.innerHTML = html;
  }

  // fetch activity logs
  function updateLogs() {
    fetch("/api/logs")
      .then((res) => res.json())
      .then((entries) => {
        const logBox = document.getElementById("activity-log-box");
        if (!logBox) return;

        if (!entries || entries.length === 0) {
          logBox.innerHTML = '<div style="color:var(--text-dim);">No activity logs recorded yet.</div>';
          return;
        }

        let html = "";
        entries.forEach((e) => {
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
      })
      .catch((err) => {
        console.error("logs fetch error:", err);
      });
  }

  // leave room handler
  function leaveRoom(room) {
    if (!confirm("Leave room " + room + "?")) return;
    postJSON("/api/rooms/leave", { room: room }, function (err, res) {
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

      postJSON("/api/rooms/join", { room: room }, function (err, res) {
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

      postJSON("/api/send", { target: target, message: message, is_pm: isPM }, function (err, res) {
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

      postJSON("/api/challenge", { user: user, format: format }, function (err, res) {
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

  // get-server discovery tool form
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
          }
        }
      });
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
  updateStatus();
  updateLogs();
  setInterval(updateStatus, 3000);
  setInterval(updateLogs, 3000);
});
