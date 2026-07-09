const tmState = {
    sessions: [],
    filteredSessions: [],
    commands: [],
    currentSession: null,
    currentIndex: 0,
    playbackTimer: null,
};

const tmElements = {
    sessionsBody: document.getElementById("tmSessionsBody"),
    refreshBtn: document.getElementById("tmRefreshBtn"),
    searchInput: document.getElementById("tmSessionSearch"),
    playBtn: document.getElementById("tmPlayBtn"),
    pauseBtn: document.getElementById("tmPauseBtn"),
    resetBtn: document.getElementById("tmResetBtn"),
    speedSelect: document.getElementById("tmSpeed"),
    slider: document.getElementById("tmSlider"),
    progress: document.getElementById("tmProgress"),
    currentTime: document.getElementById("tmCurrentTime"),
    screen: document.getElementById("tmScreen"),
    currentSessionTitle: document.getElementById("tmCurrentSession"),
    meta: document.getElementById("tmMeta"),
};

document.addEventListener("DOMContentLoaded", () => {
    bindTimeMachineEvents();
    loadTimeMachineSessions();
});

function bindTimeMachineEvents() {
    tmElements.refreshBtn.addEventListener("click", loadTimeMachineSessions);
    tmElements.searchInput.addEventListener("input", () => {
        const keyword = tmElements.searchInput.value.trim().toLowerCase();
        tmState.filteredSessions = tmState.sessions.filter((session) => {
            return (
                session.hostTag.toLowerCase().includes(keyword) ||
                session.clientId.toLowerCase().includes(keyword)
            );
        });
        renderTimeMachineSessions();
    });

    tmElements.playBtn.addEventListener("click", startTimeMachinePlayback);
    tmElements.pauseBtn.addEventListener("click", stopTimeMachinePlayback);
    tmElements.resetBtn.addEventListener("click", () => {
        tmState.currentIndex = 0;
        renderTimeMachineScreen();
    });
    tmElements.slider.addEventListener("input", (event) => {
        tmState.currentIndex = Number(event.target.value);
        renderTimeMachineScreen();
    });
}

async function loadTimeMachineSessions() {
    tmElements.sessionsBody.innerHTML = `<tr><td colspan="4" class="tm-empty">加载会话中...</td></tr>`;

    try {
        const response = await fetch("/api/time-machine/sessions");
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }
        const sessions = await response.json();
        tmState.sessions = Array.isArray(sessions) ? sessions : [];
        tmState.filteredSessions = tmState.sessions.slice();
        renderTimeMachineSessions();
    } catch (error) {
        console.error("Failed to load time-machine sessions:", error);
        tmElements.sessionsBody.innerHTML = `<tr><td colspan="4" class="tm-empty">加载失败，请稍后重试</td></tr>`;
    }
}

function renderTimeMachineSessions() {
    if (tmState.filteredSessions.length === 0) {
        tmElements.sessionsBody.innerHTML = `<tr><td colspan="4" class="tm-empty">暂无可回放会话</td></tr>`;
        return;
    }

    tmElements.sessionsBody.innerHTML = tmState.filteredSessions
        .map((session) => {
            const lastTime = formatTime(session.lastTimestamp);
            return `
            <tr>
                <td>
                    <div>${escapeHtml(session.hostTag)}</div>
                    <div class="tm-sub">${escapeHtml(session.clientId)}</div>
                </td>
                <td>
                    ${session.commandCount}
                    ${
                        session.dangerousCount > 0
                            ? `<div class="tm-danger">危险:${session.dangerousCount}</div>`
                            : ""
                    }
                </td>
                <td>${lastTime}</td>
                <td>
                    <button class="tm-open-btn" data-host="${encodeURIComponent(session.hostTag)}" data-client="${encodeURIComponent(session.clientId)}">回放</button>
                </td>
            </tr>
        `;
        })
        .join("");

    tmElements.sessionsBody.querySelectorAll(".tm-open-btn").forEach((button) => {
        button.addEventListener("click", async () => {
            const hostTag = decodeURIComponent(button.dataset.host);
            const clientId = decodeURIComponent(button.dataset.client);
            await loadTimeMachineCommands(hostTag, clientId);
        });
    });
}

async function loadTimeMachineCommands(hostTag, clientId) {
    stopTimeMachinePlayback();
    tmElements.screen.textContent = "加载命令回放中...";
    tmElements.currentSessionTitle.textContent = `${hostTag}:${clientId}`;
    tmElements.meta.textContent = "";

    try {
        const response = await fetch(
            `/api/time-machine/commands?hostTag=${encodeURIComponent(hostTag)}&clientId=${encodeURIComponent(clientId)}&limit=1200`,
        );
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }
        const output = await response.json();
        tmState.commands = Array.isArray(output.commands) ? output.commands : [];
        tmState.currentSession = { hostTag, clientId };
        tmState.currentIndex = 0;

        const truncatedTips = output.truncated ? `，仅展示最后 ${tmState.commands.length} 条` : "";
        tmElements.meta.textContent = `总命令 ${output.total || tmState.commands.length} 条${truncatedTips}`;
        setTimeMachineControlsEnabled(tmState.commands.length > 0);
        renderTimeMachineScreen();
    } catch (error) {
        console.error("Failed to load time-machine commands:", error);
        tmElements.screen.textContent = "加载回放失败，请稍后重试。";
        setTimeMachineControlsEnabled(false);
    }
}

function setTimeMachineControlsEnabled(enabled) {
    tmElements.playBtn.disabled = !enabled;
    tmElements.pauseBtn.disabled = !enabled;
    tmElements.resetBtn.disabled = !enabled;
    tmElements.speedSelect.disabled = !enabled;
    tmElements.slider.disabled = !enabled;
}

function renderTimeMachineScreen() {
    if (tmState.commands.length === 0) {
        tmElements.screen.textContent = "该会话暂无命令记录。";
        tmElements.progress.textContent = "0 / 0";
        tmElements.currentTime.textContent = "--";
        tmElements.slider.min = "0";
        tmElements.slider.max = "0";
        tmElements.slider.value = "0";
        return;
    }

    if (tmState.currentIndex >= tmState.commands.length) {
        tmState.currentIndex = tmState.commands.length - 1;
    }

    const lines = tmState.commands.slice(0, tmState.currentIndex + 1).map((item) => {
        const ts = formatTime(item.timestamp);
        const marker = item.dangerous ? "[!]" : "[ ]";
        return `${marker} ${ts}  ${item.command}`;
    });

    tmElements.screen.textContent = lines.join("\n");
    tmElements.screen.scrollTop = tmElements.screen.scrollHeight;

    tmElements.slider.min = "0";
    tmElements.slider.max = String(Math.max(tmState.commands.length - 1, 0));
    tmElements.slider.value = String(tmState.currentIndex);
    tmElements.progress.textContent = `${tmState.currentIndex + 1} / ${tmState.commands.length}`;

    const current = tmState.commands[tmState.currentIndex];
    tmElements.currentTime.textContent = current ? formatTime(current.timestamp) : "--";
}

function startTimeMachinePlayback() {
    if (tmState.commands.length === 0 || tmState.playbackTimer) {
        return;
    }

    const speed = Number(tmElements.speedSelect.value) || 1;
    const intervalMs = Math.max(80, 700 / speed);

    tmState.playbackTimer = setInterval(() => {
        if (tmState.currentIndex >= tmState.commands.length - 1) {
            stopTimeMachinePlayback();
            return;
        }
        tmState.currentIndex += 1;
        renderTimeMachineScreen();
    }, intervalMs);
}

function stopTimeMachinePlayback() {
    if (!tmState.playbackTimer) {
        return;
    }
    clearInterval(tmState.playbackTimer);
    tmState.playbackTimer = null;
}

function formatTime(timestamp) {
    if (!timestamp) {
        return "--";
    }
    const date = new Date(timestamp);
    return new Intl.DateTimeFormat("zh-CN", {
        year: "numeric",
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
        hour12: false,
    }).format(date);
}

function escapeHtml(text) {
    if (typeof text !== "string") {
        return "";
    }
    return text
        .replace(/&/g, "&amp;")
        .replace(/</g, "&lt;")
        .replace(/>/g, "&gt;")
        .replace(/"/g, "&quot;")
        .replace(/'/g, "&#39;");
}
