const params = new URLSearchParams(window.location.search);
const hostTag = params.get('hostTag');
const clientId = params.get('clientId');

if (!hostTag || !clientId) {
    console.error('Error: Missing hostTag or clientId');
} else {
    // 创建终端实例
    const term = new Terminal({
        cursorBlink: true,
        convertEol: true,
        fontSize: 16,
        lineHeight: 1.2,
        fontFamily: "'Fira Code', 'JetBrains Mono', 'Consolas', monospace",
        letterSpacing: 1.0,
        allowProposedGlyphs: true,
        scrollback: 10000,
        theme: {
            background: '#0d0f17',
            foreground: '#f5f5f5',
            cursor: '#4EC9B0',
            selection: 'rgba(78, 201, 176, 0.3)',
            black: '#1e1e1e',
            red: '#e6838d',
            green: '#a0cfa1',
            yellow: '#eab97a',
            blue: '#5cbcf6',
            magenta: '#b39df7',
            cyan: '#4EC9B0',
            white: '#dcdcdc',
            brightBlack: '#666666',
            brightRed: '#ff6b6b',
            brightGreen: '#51cf66',
            brightYellow: '#ffd43b',
            brightBlue: '#74c0fc',
            brightMagenta: '#da77f2',
            brightCyan: '#4dabf7',
            brightWhite: '#ffffff'
        },
        smoothScrollDuration: 80,
        disableStdin: false,
        allowTransparency: false,
        overviewRulerTop: 15,
        screenReaderMode: false,
        disableScrollToInput: true,
        rightClickSelectsWord: true,
        fastScrollModifier: 'alt',
        fastScrollSensitivity: 5
    });

    const fitAddon = new FitAddon.FitAddon();
    term.loadAddon(fitAddon);

    const container = document.getElementById('terminal-dr');
    term.open(container);
    fitAddon.fit();
    term.focus();

    const helpUrl = `https://${window.location.host}/help`;

    // 立即显示欢迎信息，不等待WebSocket连接
    const welcomeMessage = `
\x1b[32m\x1b[0m
\x1b[32m╔════════════════════════════════════════════════════════════════════╗\x1b[0m
\x1b[32m║                    Terminal 2 Terminal                             ║\x1b[0m
\x1b[32m║                                                                    ║\x1b[0m
\x1b[32m║  \x1b[36mConnecting to: ${hostTag}:${clientId}\x1b[32m              ║\x1b[0m
\x1b[32m║  \x1b[33mHelp: ${helpUrl}\x1b[32m                                  ║\x1b[0m
\x1b[32m╚════════════════════════════════════════════════════════════════════╝\x1b[0m

\x1b[36m🔄 Connecting... Please wait...\x1b[0m

`;
    
    // 直接显示欢迎信息
    term.write(welcomeMessage);

    const connStatus = document.getElementById('connStatus');

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const socket = new WebSocket(`${protocol}//${window.location.host}/terminal/ws?hostTag=${hostTag}&clientId=${clientId}`);

    
    // 连接成功时的效果
    socket.onopen = () => {
        // 清屏并重新显示连接成功的信息
        term.write('\x1b[2J'); // 清屏
        term.write('\x1b[H'); // 光标移到开头
        
        // 显示连接成功的欢迎信息
        const connectedMessage = `
\x1b[32m\x1b[0m
\x1b[32m╔════════════════════════════════════════════════════════════════════╗\x1b[0m
\x1b[32m║                    Terminal 2 Terminal                             ║\x1b[0m
\x1b[32m║                                                                    ║\x1b[0m
\x1b[32m║  \x1b[36mConnected to: ${hostTag}:${clientId}\x1b[32m              ║\x1b[0m
\x1b[32m║  \x1b[33mHelp: ${helpUrl}\x1b[32m                                  ║\x1b[0m
\x1b[32m╚════════════════════════════════════════════════════════════════════╝\x1b[0m

\x1b[36m🚀 Ready to use! Type your commands or Enter to continue...\x1b[0m

`;
        
        // 直接显示连接成功的欢迎信息
        term.write(connectedMessage);
        
        if (connStatus) {
            connStatus.textContent = '已连接';
            connStatus.classList.remove('disconnected');
            connStatus.classList.add('connected');
            
            // 添加连接成功的动画效果
            connStatus.style.animation = 'pulse 0.5s ease-in-out';
            setTimeout(() => {
                connStatus.style.animation = '';
            }, 500);
        }
        
        // 发送初始终端大小
        const dims = fitAddon.proposeDimensions();
        if (dims) {
            const resizeMessage = `\x1b[8;${dims.rows};${dims.cols}t`;
            socket.send(resizeMessage);
        }
    };

    // 连接断开时的效果
    socket.onclose = () => {
        if (connStatus) {
            connStatus.textContent = '已断开';
            connStatus.classList.remove('connected');
            connStatus.classList.add('disconnected');
        }
        
        // 显示断开连接的消息
        term.write('\r\n\x1b[31m╔══════════════════════════════════════════════════════════════╗\x1b[0m\r\n');
        term.write('\x1b[31m║                       连接已断开                                ║\x1b[0m\r\n');
        term.write('\x1b[31m║                    Connection Lost                              ║\x1b[0m\r\n');
        term.write('\x1b[31m╚══════════════════════════════════════════════════════════════╝\x1b[0m\r\n');
        term.write('\x1b[33m💡 请刷新页面重新连接或关闭此窗口\x1b[0m\r\n');
    };

    // 连接错误时的效果
    socket.onerror = (error) => {
        console.error('WebSocket error:', error);
        term.write('\r\n\x1b[31m❌ 连接错误，请检查网络连接\x1b[0m\r\n');
        
        if (connStatus) {
            connStatus.textContent = '连接错误';
            connStatus.classList.remove('connected');
            connStatus.classList.add('disconnected');
        }
    };

    const attachAddon = new AttachAddon.AttachAddon(socket);
    term.loadAddon(attachAddon);

    // 窗口大小改变时的处理
    window.addEventListener('resize', () => {
        fitAddon.fit();
        
        // 发送新的终端大小
        const dims = fitAddon.proposeDimensions();
        if (dims && socket.readyState === WebSocket.OPEN) {
            const resizeMessage = `\x1b[8;${dims.rows};${dims.cols}t`;
            socket.send(resizeMessage);
        }
    });

    // 添加终端焦点效果
    term.onFocus(() => {
        container.style.boxShadow = '0 0 20px rgba(78, 201, 176, 0.3)';
    });

    term.onBlur(() => {
        container.style.boxShadow = 'none';
    });

    // 添加键盘快捷键
    term.onKey(({ key, domEvent }) => {
        // Ctrl + L 清屏
        if (domEvent.ctrlKey && key === 'l') {
            domEvent.preventDefault();
            term.clear();
        }
        
        // Ctrl + K 清空当前行
        if (domEvent.ctrlKey && key === 'k') {
            domEvent.preventDefault();
            term.write('\x1b[K');
        }
    });

    // 添加鼠标滚轮支持
    term.onMouse(({ type, x, y, button }) => {
        if (type === 'wheel') {
            // 处理滚轮事件
            term.scrollLines(-1);
        }
    });
}

// 添加CSS动画
const style = document.createElement('style');
style.textContent = `
    @keyframes pulse {
        0% {
            transform: scale(1);
        }
        50% {
            transform: scale(1.05);
        }
        100% {
            transform: scale(1);
        }
    }
    
    @keyframes fadeIn {
        from {
            opacity: 0;
            transform: translateY(10px);
        }
        to {
            opacity: 1;
            transform: translateY(0);
        }
    }
    
    #terminal-dr {
        animation: fadeIn 0.5s ease-out;
    }
    
    .terminal-card {
        animation: fadeIn 0.8s ease-out;
    }
`;
document.head.appendChild(style);