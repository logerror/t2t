const wrState = {
    agents: [],
};

const wrElements = {
    refreshBtn: document.getElementById("wrRefreshBtn"),
    openBtn: document.getElementById("wrOpenBtn"),
    broadcastInput: document.getElementById("wrBroadcastInput"),
    broadcastBtn: document.getElementById("wrBroadcastBtn"),
    agentList: document.getElementById("wrAgentList"),
    roomGrid: document.getElementById("wrRoomGrid"),
    agentCount: document.getElementById("wrAgentCount"),
};

document.addEventListener("DOMContentLoaded", () => {
    bindWarRoomEvents();
    loadWarRoomAgents();
});

function bindWarRoomEvents() {
    wrElements.refreshBtn.addEventListener("click", loadWarRoomAgents);
    wrElements.openBtn.addEventListener("click", openWarRoomTerminals);
    wrElements.broadcastBtn.addEventListener("click", broadcastWarRoomCommand);
    wrElements.broadcastInput.addEventListener("keydown", (event) => {
        if (event.key === "Enter") {
            event.preventDefault();
            broadcastWarRoomCommand();
        }
    });
}

async function loadWarRoomAgents() {
    wrElements.agentList.innerHTML = `<div class="wr-empty">加载中...</div>`;
    try {
        const response = await fetch("/agents");
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }
        const data = await response.json();
        wrState.agents = Array.isArray(data) ? data : [];
        renderWarRoomAgents();
    } catch (error) {
        console.error("Failed to load war-room agents:", error);
        wrElements.agentList.innerHTML = `<div class="wr-empty">加载失败，请稍后再试</div>`;
        wrElements.agentCount.textContent = "0 台在线";
    }
}

function renderWarRoomAgents() {
    wrElements.agentCount.textContent = `${wrState.agents.length} 台在线`;
    if (wrState.agents.length === 0) {
        wrElements.agentList.innerHTML = `<div class="wr-empty">暂无在线主机</div>`;
        return;
    }

    wrElements.agentList.innerHTML = wrState.agents
        .map((agent, index) => {
            const value = `${encodeURIComponent(agent.hostTag)}::${encodeURIComponent(agent.clientId)}`;
            return `
            <label class="wr-agent-item">
                <input type="checkbox" class="wr-agent-checkbox" value="${value}" ${index === 0 ? "checked" : ""}>
                <div>
                    <div class="wr-agent-name">${escapeHtml(agent.hostTag)} / ${escapeHtml(agent.clientId)}</div>
                    <div class="wr-agent-sub">Agent ${escapeHtml(agent.agentVersion || "-")} | Client ${escapeHtml(agent.clientVersion || "-")}</div>
                </div>
            </label>
        `;
        })
        .join("");
}

function openWarRoomTerminals() {
    const selected = getSelectedWarRoomTargets();
    if (selected.length === 0) {
        showWarRoomToast("请至少选择一台主机");
        return;
    }

    wrElements.roomGrid.innerHTML = selected
        .map((target) => {
            const src = `/terminal?hostTag=${encodeURIComponent(target.hostTag)}&clientId=${encodeURIComponent(target.clientId)}`;
            const title = `${target.hostTag}:${target.clientId}`;
            return `
            <div class="wr-terminal-card">
                <div class="wr-terminal-title">${escapeHtml(title)}</div>
                <iframe class="wr-terminal-iframe" src="${src}" title="${escapeHtml(title)}"></iframe>
            </div>
        `;
        })
        .join("");

    showWarRoomToast(`已打开 ${selected.length} 个终端视窗`);
}

function broadcastWarRoomCommand() {
    const rawCommand = wrElements.broadcastInput.value;
    const command = typeof rawCommand === "string" ? rawCommand.trim() : "";
    if (!command) {
        showWarRoomToast("请输入要广播的命令");
        return;
    }

    const frames = wrElements.roomGrid.querySelectorAll(".wr-terminal-iframe");
    if (frames.length === 0) {
        showWarRoomToast("请先打开分屏终端");
        return;
    }

    const payload = command.endsWith("\n") ? command : `${command}\n`;
    frames.forEach((frame) => {
        if (frame.contentWindow) {
            frame.contentWindow.postMessage(
                {
                    type: "t2t:broadcastCommand",
                    command: payload,
                },
                window.location.origin,
            );
        }
    });

    showWarRoomToast(`已广播到 ${frames.length} 个终端`);
    wrElements.broadcastInput.value = "";
}

function getSelectedWarRoomTargets() {
    const checkboxes = wrElements.agentList.querySelectorAll(".wr-agent-checkbox:checked");
    return Array.from(checkboxes).map((checkbox) => {
        const [rawHostTag, rawClientId] = checkbox.value.split("::");
        return {
            hostTag: decodeURIComponent(rawHostTag),
            clientId: decodeURIComponent(rawClientId),
        };
    });
}

function showWarRoomToast(message) {
    const toast = document.createElement("div");
    toast.className = "wr-toast";
    toast.textContent = message;
    document.body.appendChild(toast);
    setTimeout(() => {
        toast.remove();
    }, 2200);
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
