let allAgents = [];

document.addEventListener("DOMContentLoaded", () => {
    createParticles();
    bindEvents();
    loadAgents();
});

function bindEvents() {
    const refreshBtn = document.getElementById("refreshBtn");
    const searchInput = document.getElementById("agentSearch");
    const timeMachineBtn = document.getElementById("openTimeMachineBtn");
    const warRoomBtn = document.getElementById("openWarRoomBtn");

    refreshBtn.addEventListener("click", () => {
        refreshBtn.classList.add("refreshing");
        loadAgents();
    });

    let searchTimeout;
    searchInput.addEventListener("input", () => {
        clearTimeout(searchTimeout);
        searchTimeout = setTimeout(() => {
            applySearchFilter(searchInput.value || "");
        }, 180);
    });

    if (timeMachineBtn) {
        timeMachineBtn.addEventListener("click", openTimeMachine);
    }
    if (warRoomBtn) {
        warRoomBtn.addEventListener("click", openWarRoom);
    }

    document.addEventListener("keydown", (event) => {
        if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "r") {
            event.preventDefault();
            refreshBtn.click();
        }
        if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "f") {
            event.preventDefault();
            searchInput.focus();
        }
        if (event.key === "Escape" && document.activeElement === searchInput) {
            searchInput.value = "";
            applySearchFilter("");
        }
    });
}

function createParticles() {
    const prefersReducedMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
    if (prefersReducedMotion) {
        return;
    }

    const particlesBg = document.getElementById("particlesBg");
    if (!particlesBg) {
        return;
    }

    const count = 18;
    for (let i = 0; i < count; i += 1) {
        const particle = document.createElement("div");
        particle.className = "particle";
        particle.style.left = `${Math.random() * 100}%`;
        particle.style.top = `${Math.random() * 100}%`;
        particle.style.animationDelay = `${Math.random() * 6}s`;
        particle.style.animationDuration = `${8 + Math.random() * 8}s`;
        particlesBg.appendChild(particle);
    }
}

function loadAgents() {
    const tbody = document.querySelector("#agentsList tbody");
    tbody.innerHTML = `
        <tr>
            <td colspan="9" class="loading-state">
                <div class="loading-container">
                    <svg class="loading-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor">
                        <circle cx="12" cy="12" r="10" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" stroke-dasharray="31.416" stroke-dashoffset="31.416"></circle>
                        <path d="M12 2a10 10 0 0 1 10 10" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"></path>
                    </svg>
                    <span>正在加载 Agents...</span>
                </div>
            </td>
        </tr>
    `;

    fetch("/agents")
        .then((response) => {
            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }
            return response.json();
        })
        .then((data) => {
            allAgents = Array.isArray(data) ? data : [];
            applySearchFilter(document.getElementById("agentSearch").value || "");
            updateLastRefreshTime();
        })
        .catch((error) => {
            console.error("Error loading agents:", error);
            allAgents = [];
            updateAgentsList([]);
            updateStats([]);
            showToast("加载 Agent 列表失败，请稍后重试", "error");
        })
        .finally(() => {
            document.getElementById("refreshBtn").classList.remove("refreshing");
        });
}

function applySearchFilter(keyword) {
    const normalized = keyword.trim().toLowerCase();
    let result = allAgents;
    if (normalized) {
        result = allAgents.filter((agent) => {
            const hostTag = String(agent.hostTag || "").toLowerCase();
            const clientId = String(agent.clientId || "").toLowerCase();
            const clientUser = String(agent.clientUser || "").toLowerCase();
            return hostTag.includes(normalized) || clientId.includes(normalized) || clientUser.includes(normalized);
        });
    }
    updateAgentsList(result);
    updateStats(result);
}

function updateStats(agents) {
    const totalAgents = agents.length;
    const activeUsers = new Set(agents.map((agent) => agent.clientUser).filter(Boolean)).size;

    setMetric("totalAgents", totalAgents);
    setMetric("activeUsers", activeUsers);
}

function setMetric(elementId, nextValue) {
    const element = document.getElementById(elementId);
    const previous = Number.parseInt(element.textContent || "0", 10);
    element.textContent = String(nextValue);
    if (previous !== nextValue) {
        element.classList.remove("animate");
        window.requestAnimationFrame(() => {
            element.classList.add("animate");
        });
    }
}

function updateAgentsList(agents) {
    const tbody = document.querySelector("#agentsList tbody");
    tbody.innerHTML = "";

    if (!agents.length) {
        tbody.innerHTML = `
            <tr>
                <td colspan="9" class="no-agents">
                    <div>暂无在线 Agent，或当前搜索无匹配结果</div>
                </td>
            </tr>
        `;
        return;
    }

    agents.forEach((agent) => {
        const row = document.createElement("tr");
        const hostTag = escapeHtml(agent.hostTag || "-");
        const clientId = escapeHtml(agent.clientId || "-");
        const command = `t2t-client connect -t ${agent.hostTag} -c ${agent.clientId}`;
        const isOccupied = Boolean(agent.clientUser);

        row.innerHTML = `
            <td class="status-cell">
                <span class="status-dot ${isOccupied ? "active" : ""}" aria-hidden="true"></span>
                <span class="status-text">${isOccupied ? "占用中" : "空闲"}</span>
            </td>
            <td>
                <div class="host-cell">
                    <span class="host-primary">${hostTag}</span>
                </div>
            </td>
            <td><code class="code-chip">${clientId}</code></td>
            <td><span class="tag tag-neutral">${escapeHtml(agent.agentArch || "-")}</span></td>
            <td><span class="tag tag-primary">${escapeHtml(agent.agentVersion || "-")}</span></td>
            <td><span class="tag tag-secondary">${escapeHtml(agent.clientVersion || "-")}</span></td>
            <td><span class="user-badge ${isOccupied ? "active" : ""}">${escapeHtml(agent.clientUser || "未占用")}</span></td>
            <td><span class="time-text">${formatDate(agent.createdAt)}</span></td>
            <td class="actions">
                <button class="action-btn copy-btn" title="复制连接命令">Copy</button>
                <button class="action-btn connect-btn" title="打开 Web Terminal">Terminal</button>
            </td>
        `;

        const [copyBtn, connectBtn] = row.querySelectorAll(".action-btn");
        copyBtn.addEventListener("click", () => copyCommand(command, copyBtn));
        connectBtn.addEventListener("click", () => openTerminal(agent.hostTag, agent.clientId));

        tbody.appendChild(row);
    });
}

async function copyCommand(command, button) {
    try {
        await navigator.clipboard.writeText(command);
        const original = button.textContent;
        button.textContent = "Copied";
        button.classList.add("copied");
        showToast("连接命令已复制到剪贴板", "success");
        setTimeout(() => {
            button.textContent = original;
            button.classList.remove("copied");
        }, 1400);
    } catch (error) {
        console.error("Failed to copy command:", error);
        showToast("复制失败，请手动复制", "error");
    }
}

async function openTerminal(hostTag, clientId) {
    try {
        const response = await fetch(`/check/${hostTag}/${clientId}`);
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }

        const data = await response.json();
        if (!data.available) {
            showToast("该 Agent 当前不可用，请稍后重试", "error");
            return;
        }

        const url = `/terminal?hostTag=${encodeURIComponent(hostTag)}&clientId=${encodeURIComponent(clientId)}`;
        const terminalWindow = window.open(url, "_blank");
        if (!terminalWindow) {
            showToast("浏览器阻止了弹窗，请允许后重试", "error");
        }
    } catch (error) {
        console.error("Error checking connection:", error);
        showToast("连接检查失败，请确认 Server 与 Agent 状态", "error");
    }
}

function openTimeMachine() {
    const page = window.open("/time-machine", "_blank");
    if (!page) {
        showToast("无法打开时光机页面，请检查弹窗设置", "error");
    }
}

function openWarRoom() {
    const page = window.open("/war-room", "_blank");
    if (!page) {
        showToast("无法打开战情室页面，请检查弹窗设置", "error");
    }
}

function updateLastRefreshTime() {
    const lastUpdate = document.getElementById("lastUpdate");
    lastUpdate.textContent = `最后更新: ${formatDate(Date.now())}`;
}

function formatDate(timestamp) {
    if (!timestamp) {
        return "-";
    }
    const date = new Date(timestamp);
    if (Number.isNaN(date.getTime())) {
        return "-";
    }
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

function showToast(message, type) {
    const toast = document.createElement("div");
    const toastType = type === "success" ? "app-toast--success" : type === "error" ? "app-toast--error" : "";
    toast.className = `app-toast ${toastType}`.trim();
    toast.textContent = message;
    document.body.appendChild(toast);

    setTimeout(() => {
        toast.remove();
    }, 2200);
}

function escapeHtml(value) {
    return String(value)
        .replace(/&/g, "&amp;")
        .replace(/</g, "&lt;")
        .replace(/>/g, "&gt;")
        .replace(/"/g, "&quot;")
        .replace(/'/g, "&#39;");
}
