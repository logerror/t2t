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
        fontFamily: "'Inter', monospace",
        letterSpacing: 0.5,
        allowProposedGlyphs: true,
        scrollback: 5000,
        theme: {
            background: '#1e1e1e',
            foreground: '#d4d4d4',
            cursor: '#aeafad',
            selection: 'rgba(255, 255, 255, 0.2)',
            black: '#1e1e1e',
            red: '#f44747',
            green: '#6a9955',
            yellow: '#d7ba7d',
            blue: '#569cd6',
            magenta: '#646695',
            cyan: '#4EC9B0',
            white: '#d4d4d4'
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

    const container = document.getElementById('terminal-t2t');
    term.open(container);
    fitAddon.fit();
    term.focus();

    const params = new URLSearchParams(window.location.search);
    const hostTag = params.get('hostTag');
    const clientId = params.get('clientId');

    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const socket = new WebSocket(`${protocol}//${window.location.host}/terminal/ws?hostTag=${hostTag}&clientId=${clientId}`);

    helpUrl = `https://${window.location.host}/help`;
    socket.onopen = () => {
        term.write(`\x1b[32mConnected to ${hostTag}:${clientId} terminal. For more information: ${helpUrl} \x1b[0m\r`);
        // 发送初始终端大小
        const dims = fitAddon.proposeDimensions();
        if (dims) {
            const resizeMessage = `\x1b[8;${dims.rows};${dims.cols}t`;
            socket.send(resizeMessage);
        }
    };

    const attachAddon = new AttachAddon.AttachAddon(socket);
    term.loadAddon(attachAddon);

    window.addEventListener('resize', () => {
        fitAddon.fit();
    });
}