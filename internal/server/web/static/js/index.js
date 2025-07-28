let allAgents = [];

// 生成粒子背景
function createParticles() {
    const particlesBg = document.getElementById('particlesBg');
    const particleCount = 50;
    
    for (let i = 0; i < particleCount; i++) {
        const particle = document.createElement('div');
        particle.className = 'particle';
        particle.style.left = Math.random() * 100 + '%';
        particle.style.top = Math.random() * 100 + '%';
        particle.style.animationDelay = Math.random() * 5 + 's';
        particle.style.animationDuration = (Math.random() * 5 + 5) + 's';
        particlesBg.appendChild(particle);
    }
}

document.addEventListener('DOMContentLoaded', function() {
    createParticles();
    loadAgents();
    
    // 添加页面加载动画
    document.body.style.opacity = '0';
    setTimeout(() => {
        document.body.style.transition = 'opacity 0.5s ease-in-out';
        document.body.style.opacity = '1';
    }, 100);
    
    document.getElementById('refreshBtn').addEventListener('click', function() {
        this.classList.add('refreshing');
        loadAgents();
    });
    
    // 优化搜索功能，添加防抖
    let searchTimeout;
    document.getElementById('agentSearch').addEventListener('input', function() {
        clearTimeout(searchTimeout);
        searchTimeout = setTimeout(() => {
            const keyword = this.value.trim().toLowerCase();
            const filtered = allAgents.filter(agent =>
                agent.hostTag.toLowerCase().includes(keyword) ||
                agent.clientId.toLowerCase().includes(keyword) ||
                (agent.clientUser && agent.clientUser.toLowerCase().includes(keyword))
            );
            updateAgentsList(filtered);
            updateStats(filtered);
        }, 300); // 300ms 防抖延迟
    });
    
    // 添加搜索框聚焦效果
    document.getElementById('agentSearch').addEventListener('focus', function() {
        this.parentElement.style.transform = 'scale(1.02)';
    });
    
    document.getElementById('agentSearch').addEventListener('blur', function() {
        this.parentElement.style.transform = 'scale(1)';
    });
});

function loadAgents() {
    // 显示加载状态
    const tbody = document.querySelector('#agentsList tbody');
    tbody.innerHTML = `
        <tr>
            <td colspan="8" class="no-agents">
                <div class="loading-spinner"></div>
                <div style="margin-top: 1rem;">正在加载 Agents...</div>
            </td>
        </tr>`;
    
    fetch('/agents')
        .then(response => response.json())
        .then(data => {
            allAgents = data;
            
            // 获取当前搜索条件
            const searchInput = document.getElementById('agentSearch');
            const keyword = searchInput.value.trim().toLowerCase();
            
            // 如果有搜索条件，应用过滤
            if (keyword) {
                const filtered = allAgents.filter(agent =>
                    agent.hostTag.toLowerCase().includes(keyword) ||
                    agent.clientId.toLowerCase().includes(keyword) ||
                    (agent.clientUser && agent.clientUser.toLowerCase().includes(keyword))
                );
                updateAgentsList(filtered);
                updateStats(filtered);
            } else {
                // 没有搜索条件，显示全部
                updateAgentsList(data);
                updateStats(data);
            }
            updateLastRefreshTime();
        })
        .catch(error => {
            console.error('Error loading agents:', error);
            showError('Failed to load agents');
            // 错误时也要更新统计，显示0
            updateStats([]);
        })
        .finally(() => {
            document.getElementById('refreshBtn').classList.remove('refreshing');
        });
}

function updateStats(agents) {
    const totalAgentsElement = document.getElementById('totalAgents');
    const activeUsersElement = document.getElementById('activeUsers');
    
    const newTotalAgents = agents.length;
    const newActiveUsers = new Set(agents.map(agent => agent.clientUser).filter(Boolean)).size;
    
    // 只在数值真正改变时才触发动画
    if (parseInt(totalAgentsElement.textContent) !== newTotalAgents) {
        animateNumber('totalAgents', newTotalAgents);
    } else {
        totalAgentsElement.textContent = newTotalAgents;
    }
    
    if (parseInt(activeUsersElement.textContent) !== newActiveUsers) {
        animateNumber('activeUsers', newActiveUsers);
    } else {
        activeUsersElement.textContent = newActiveUsers;
    }
}

function animateNumber(elementId, targetValue) {
    const element = document.getElementById(elementId);
    const currentValue = parseInt(element.textContent) || 0;
    
    // 如果数值没有变化，直接返回
    if (currentValue === targetValue) {
        return;
    }
    
    const increment = (targetValue - currentValue) / 20;
    let current = currentValue;
    
    // 添加动画类
    element.classList.add('animate');
    
    const timer = setInterval(() => {
        current += increment;
        if ((increment > 0 && current >= targetValue) || (increment < 0 && current <= targetValue)) {
            element.textContent = targetValue;
            clearInterval(timer);
            
            // 移除动画类
            setTimeout(() => {
                element.classList.remove('animate');
            }, 600);
        } else {
            element.textContent = Math.floor(current);
        }
    }, 50);
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
        row.style.animationDelay = `${index * 100}ms`;
        const command = `t2t-client ${agent.hostTag} ${agent.clientId}`;
        
        row.innerHTML = `
            <td>
                <div style="font-weight: 600; color: var(--text-primary);">${agent.hostTag}</div>
            </td>
            <td>
                <div style="font-family: 'Courier New', monospace; background: rgba(0,0,0,0.05); padding: 0.25rem 0.5rem; border-radius: 0.25rem; font-size: 0.9rem;">${agent.clientId}</div>
            </td>
            <td>${agent.agentArch || '-'}</td>
            <td>
                <span style="background: var(--gradient-primary); -webkit-background-clip: text; -webkit-text-fill-color: transparent; background-clip: text; font-weight: 600;">${agent.agentVersion || '-'}</span>
            </td>
            <td>
                <span style="background: var(--gradient-secondary); -webkit-background-clip: text; -webkit-text-fill-color: transparent; background-clip: text; font-weight: 600;">${agent.clientVersion || '-'}</span>
            </td>
            <td>
                <span class="user-badge ${agent.clientUser ? 'active' : ''}">${agent.clientUser || '-'}</span>
            </td>
            <td>
                <div style="font-size: 0.85rem; color: var(--text-secondary);">${formatDate(agent.createdAt)}</div>
            </td>
            <td class="actions">
                <button class="action-btn copy-btn" onclick="copyCommand('${command}', this)" title="复制连接命令">
                    <svg style="width: 14px; height: 14px; margin-right: 4px;" viewBox="0 0 24 24" fill="none" stroke="currentColor">
                        <rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect>
                        <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path>
                    </svg>
                    Copy
                </button>
                <button class="action-btn connect-btn" onclick="openTerminal('${agent.hostTag}', '${agent.clientId}')" title="打开Web终端">
                    <svg style="width: 14px; height: 14px; margin-right: 4px;" viewBox="0 0 24 24" fill="none" stroke="currentColor">
                        <path d="M4 4h16c1.1 0 2 .9 2 2v12c0 1.1-.9 2-2 2H4c-1.1 0-2-.9-2-2V6c0-1.1.9-2 2-2z"></path>
                        <polyline points="4,7 20,7"></polyline>
                        <line x1="8" y1="11" x2="8" y2="11"></line>
                        <line x1="12" y1="11" x2="12" y2="11"></line>
                        <line x1="16" y1="11" x2="16" y2="11"></line>
                    </svg>
                    Terminal
                </button>
            </td>
        `;
        tbody.appendChild(row);
    });
    
    // 添加表格行效果
    addTableRowEffects();
}

async function copyCommand(command, button) {
    try {
        await navigator.clipboard.writeText(command);
        
        // 更炫酷的复制成功效果
        button.innerHTML = `
            <svg style="width: 14px; height: 14px; margin-right: 4px;" viewBox="0 0 24 24" fill="none" stroke="currentColor">
                <polyline points="20,6 9,17 4,12"></polyline>
            </svg>
            Copied!
        `;
        button.classList.add('copied');
        
        // 添加成功提示
        showSuccess('Command copied to clipboard!');
        
        setTimeout(() => {
            button.innerHTML = `
                <svg style="width: 14px; height: 14px; margin-right: 4px;" viewBox="0 0 24 24" fill="none" stroke="currentColor">
                    <rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect>
                    <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path>
                </svg>
                Copy
            `;
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
        const terminalWindow = window.open(`/terminal?hostTag=${encodeURIComponent(hostTag)}&clientId=${encodeURIComponent(clientId)}`, '_blank');
        
        if (terminalWindow) {
            showSuccess('Terminal window opened successfully!');
        } else {
            showError('Failed to open terminal window. Please check your popup blocker.');
        }
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
    errorDiv.innerHTML = `
        <svg style="width: 20px; height: 20px; margin-right: 8px;" viewBox="0 0 24 24" fill="none" stroke="currentColor">
            <circle cx="12" cy="12" r="10"></circle>
            <line x1="15" y1="9" x2="9" y2="15"></line>
            <line x1="9" y1="9" x2="15" y2="15"></line>
        </svg>
        ${message}
    `;
    document.body.appendChild(errorDiv);

    // 3秒后自动消失
    setTimeout(() => {
        errorDiv.style.animation = 'slideOut 0.3s ease-out forwards';
        setTimeout(() => {
            errorDiv.remove();
        }, 300);
    }, 3000);
}

function showSuccess(message) {
    // 成功提示
    const successDiv = document.createElement('div');
    successDiv.className = 'success-toast';
    successDiv.innerHTML = `
        <svg style="width: 20px; height: 20px; margin-right: 8px;" viewBox="0 0 24 24" fill="none" stroke="currentColor">
            <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"></path>
            <polyline points="22,4 12,14.01 9,11.01"></polyline>
        </svg>
        ${message}
    `;
    document.body.appendChild(successDiv);

    // 3秒后自动消失
    setTimeout(() => {
        successDiv.style.animation = 'slideOut 0.3s ease-out forwards';
        setTimeout(() => {
            successDiv.remove();
        }, 300);
    }, 3000);
}

// 添加键盘快捷键
document.addEventListener('keydown', function(e) {
    // Ctrl/Cmd + R 刷新
    if ((e.ctrlKey || e.metaKey) && e.key === 'r') {
        e.preventDefault();
        document.getElementById('refreshBtn').click();
    }
    
    // Ctrl/Cmd + F 聚焦搜索框
    if ((e.ctrlKey || e.metaKey) && e.key === 'f') {
        e.preventDefault();
        document.getElementById('agentSearch').focus();
    }
    
    // ESC 清空搜索框
    if (e.key === 'Escape') {
        const searchInput = document.getElementById('agentSearch');
        if (searchInput === document.activeElement) {
            searchInput.value = '';
            searchInput.dispatchEvent(new Event('input'));
        }
    }
});

// 添加表格行点击效果
function addTableRowEffects() {
    const rows = document.querySelectorAll('#agentsList tbody tr');
    rows.forEach(row => {
        row.addEventListener('click', function(e) {
            // 避免点击按钮时触发行点击
            if (e.target.closest('.action-btn')) {
                return;
            }
            
            // 添加点击效果
            this.style.transform = 'scale(0.98)';
            setTimeout(() => {
                this.style.transform = '';
            }, 150);
        });
    });
}

// 添加CSS动画
const style = document.createElement('style');
style.textContent = `
    @keyframes slideOut {
        from {
            transform: translateX(0);
            opacity: 1;
        }
        to {
            transform: translateX(100%);
            opacity: 0;
        }
    }
    
    .success-toast {
        position: fixed;
        top: 20px;
        right: 20px;
        background: linear-gradient(135deg, #dcfce7 0%, #bbf7d0 100%);
        color: #16a34a;
        padding: 1.5rem 2rem;
        border-radius: 1rem;
        box-shadow: 0 20px 40px rgba(22, 163, 74, 0.2);
        z-index: 1000;
        animation: slideInFromLeft 0.5s ease-out;
        border: 1px solid #bbf7d0;
        backdrop-filter: blur(10px);
        font-weight: 600;
        max-width: 400px;
        display: flex;
        align-items: center;
    }
    
    .error-toast {
        display: flex;
        align-items: center;
    }
    
    /* 按钮悬停音效 */
    .action-btn:hover {
        transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
    }
    
    /* 表格行悬停效果 */
    #agentsList tbody tr {
        cursor: pointer;
        transition: all 0.3s cubic-bezier(0.4, 0, 0.2, 1);
    }
    
    #agentsList tbody tr:hover {
        background: linear-gradient(135deg, rgba(102, 126, 234, 0.05) 0%, rgba(118, 75, 162, 0.05) 100%);
    }
    
    /* 搜索框聚焦动画 */
    .refresh-container {
        transition: transform 0.3s cubic-bezier(0.4, 0, 0.2, 1);
    }
    
    /* 页面加载动画 */
    body {
        opacity: 0;
        transition: opacity 0.5s ease-in-out;
    }
    
    /* 响应式优化 */
    @media (max-width: 768px) {
        .particle {
            display: none;
        }
    }
    
    /* 减少动画偏好 */
    @media (prefers-reduced-motion: reduce) {
        .particle {
            display: none;
        }
        
        * {
            animation-duration: 0.01ms !important;
            animation-iteration-count: 1 !important;
            transition-duration: 0.01ms !important;
        }
    }
`;
document.head.appendChild(style); 