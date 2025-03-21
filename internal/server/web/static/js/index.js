document.addEventListener('DOMContentLoaded', function() {
    loadAgents();
    document.getElementById('refreshBtn').addEventListener('click', function() {
        this.classList.add('refreshing');
        loadAgents();
    });
});

function loadAgents() {
    fetch('/agents')
        .then(response => response.json())
        .then(data => {
            updateAgentsList(data);
            updateStats(data);
            updateLastRefreshTime();
        })
        .catch(error => {
            console.error('Error loading agents:', error);
            showError('Failed to load agents');
        })
        .finally(() => {
            document.getElementById('refreshBtn').classList.remove('refreshing');
        });
}

function updateStats(agents) {
    document.getElementById('totalAgents').textContent = agents.length;
    const activeUsers = new Set(agents.map(agent => agent.clientUser).filter(Boolean)).size;
    document.getElementById('activeUsers').textContent = activeUsers;
}

function updateAgentsList(agents) {
    const tbody = document.querySelector('#agentsList tbody');
    tbody.innerHTML = '';

    if (agents.length === 0) {
        tbody.innerHTML = `
            <tr>
                <td colspan="8" class="no-agents">
                    <div>暂无连接的 Agents</div>
                </td>
            </tr>`;
        return;
    }

    agents.forEach((agent, index) => {
        const row = document.createElement('tr');
        row.style.animationDelay = `${index * 50}ms`;
        const command = `t2t-client ${agent.hostTag} ${agent.clientId}`;
        row.innerHTML = `
            <td>${agent.hostTag}</td>
            <td>${agent.clientId}</td>
            <td>${agent.agentArch || '-'}</td>
            <td>${agent.agentVersion || '-'}</td>
            <td>${agent.clientVersion || '-'}</td>
            <td>
                <span class="user-badge ${agent.clientUser ? 'active' : ''}">${agent.clientUser || '-'}</span>
            </td>
            <td>${formatDate(agent.createdAt)}</td>
            <td class="actions">
                <button class="action-btn copy-btn" onclick="copyCommand('${command}', this)">
                    Copy Command
                </button>
                <button class="action-btn connect-btn" onclick="openTerminal('${agent.hostTag}', '${agent.clientId}')">
                    Web Terminal
                </button>
            </td>
        `;
        tbody.appendChild(row);
    });
}

async function copyCommand(command, button) {
    try {
        await navigator.clipboard.writeText(command);
        button.textContent = 'Copied!';
        button.classList.add('copied');
        
        setTimeout(() => {
            button.textContent = 'Copy Command';
            button.classList.remove('copied');
        }, 2000);
    } catch (err) {
        console.error('Failed to copy command:', err);
        showError('Failed to copy command');
    }
}

async function openTerminal(hostTag, clientId) {
    try {
        // 先检查连接状态
        const response = await fetch(`/check/${hostTag}/${clientId}`);
        const data = await response.json();
        
        if (!data.available) {
            showError('Agent is not connected. Please ensure the agent is online before connecting.');
            return;
        }
        
        // 连接可用，打开终端
        window.open(`/terminal?hostTag=${encodeURIComponent(hostTag)}&clientId=${encodeURIComponent(clientId)}`, '_blank');
    } catch (error) {
        console.error('Error checking connection:', error);
        showError('Failed to check agent connection status');
    }
}

function showDetails(hostTag, clientId) {
    // 实现查看详情的功能
    console.log('Show details for:', hostTag, clientId);
}

function formatDate(timestamp) {
    if (!timestamp) return 'N/A';
    const date = new Date(timestamp);
    return new Intl.DateTimeFormat('zh-CN', {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
        hour12: false
    }).format(date);
}

function updateLastRefreshTime() {
    const lastUpdate = document.getElementById('lastUpdate');
    lastUpdate.textContent = `最后更新: ${formatDate(new Date())}`;
}

function showError(message) {
    // 改进错误提示
    const errorDiv = document.createElement('div');
    errorDiv.className = 'error-toast';
    errorDiv.textContent = message;
    document.body.appendChild(errorDiv);

    // 3秒后自动消失
    setTimeout(() => {
        errorDiv.remove();
    }, 3000);
} 