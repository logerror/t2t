const params = new URLSearchParams(window.location.search);
const hostTag = params.get('hostTag');
const clientId = params.get('clientId');

if (!hostTag || !clientId) {
    term.write('Error: Missing hostTag or clientId\r\n');
} else {
    // 创建终端实例
    const term = new Terminal({
        cursorBlink: true,
        convertEol: true,
        fontSize: 16,
        lineHeight: 1.2,
        fontFamily: "'Fira Code', 'JetBrains Mono', monospace",
        letterSpacing: 1.0,
        allowProposedGlyphs: true,
        scrollback: 5000,
        theme: {
            background: '#0d0f17',
            foreground: '#f5f5f5',
            cursor: '#4EC9B0',
            selection: 'rgba(78, 201, 176, 0.2)',
            black: '#1e1e1e',
            red: '#e6838d',
            green: '#a0cfa1',
            yellow: '#eab97a',
            blue: '#5cbcf6',
            magenta: '#b39df7',
            cyan: '#4EC9B0',
            white: '#dcdcdc'
        },
        smoothScrollDuration: 80,
        disableStdin: false,
        allowTransparency: false,
        overviewRulerTop: 15,
        screenReaderMode: false,
        disableScrollToInput: true
    });

    const fitAddon = new FitAddon.FitAddon();
    term.loadAddon(fitAddon);

    const container = document.getElementById('terminal-dr');
    term.open(container);
    fitAddon.fit();
    term.focus();

    const params = new URLSearchParams(window.location.search);
    const hostTag = params.get('hostTag');
    const clientId = params.get('clientId');

    const connStatus = document.getElementById('connStatus');

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const socket = new WebSocket(`${protocol}//${window.location.host}/terminal/ws?hostTag=${hostTag}&clientId=${clientId}`);

    helpUrl = `https://${window.location.host}/help`;
    socket.onopen = () => {
        term.write(`\x1b[32mConnected to ${hostTag}:${clientId} terminal. For more information: ${helpUrl} \x1b[0m\r`);
        if (connStatus) {
            connStatus.textContent = '已连接';
            connStatus.classList.remove('disconnected');
            connStatus.classList.add('connected');
        }
        // 发送初始终端大小
        const dims = fitAddon.proposeDimensions();
        if (dims) {
            const resizeMessage = `\x1b[8;${dims.rows};${dims.cols}t`;
            socket.send(resizeMessage);
        }
    };

    socket.onclose = () => {
        if (connStatus) {
            connStatus.textContent = '已断开';
            connStatus.classList.remove('connected');
            connStatus.classList.add('disconnected');
        }
        term.write('\r\n\x1b[31m[连接已断开]\x1b[0m\r\n');
    };

    const attachAddon = new AttachAddon.AttachAddon(socket);
    term.loadAddon(attachAddon);

    window.addEventListener('resize', () => {
        fitAddon.fit();
    });
}