package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	osuser "os/user"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// CommandMonitor 命令监控器
type CommandMonitor struct {
	hostTag         string
	clientID        string
	user            string
	logFile         string
	statsFile       string
	mutex           sync.RWMutex
	stats           map[string]int
	historyFile     string
	lastHistorySize int64
	stopChan        chan struct{}
}

// CommandInfo 命令信息结构
type CommandInfo struct {
	Timestamp   time.Time `json:"timestamp"`
	HostTag     string    `json:"host_tag"`
	ClientID    string    `json:"client_id"`
	Command     string    `json:"command"`
	User        string    `json:"user"`
	WorkingDir  string    `json:"working_dir"`
	ExitCode    int       `json:"exit_code,omitempty"`
	Duration    int64     `json:"duration_ms,omitempty"`
	IsDangerous bool      `json:"is_dangerous,omitempty"`
	RawCommand  string    `json:"raw_command,omitempty"` // 原始命令（包含参数）
}

// 危险命令模式
var dangerousPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)rm\s+-rf`),
	regexp.MustCompile(`(?i)dd\s+if=`),
	regexp.MustCompile(`(?i)mkfs\..*`),
	regexp.MustCompile(`(?i)chmod\s+777`),
	regexp.MustCompile(`(?i)chown\s+root`),
	regexp.MustCompile(`(?i)sudo\s+.*`),
	regexp.MustCompile(`(?i)su\s+.*`),
	regexp.MustCompile(`(?i)passwd\s+.*`),
	regexp.MustCompile(`(?i)useradd\s+.*`),
	regexp.MustCompile(`(?i)userdel\s+.*`),
	regexp.MustCompile(`(?i)groupadd\s+.*`),
	regexp.MustCompile(`(?i)groupdel\s+.*`),
	regexp.MustCompile(`(?i)iptables\s+.*`),
	regexp.MustCompile(`(?i)ufw\s+.*`),
	regexp.MustCompile(`(?i)systemctl\s+.*`),
	regexp.MustCompile(`(?i)service\s+.*`),
	regexp.MustCompile(`(?i)shutdown\s+.*`),
	regexp.MustCompile(`(?i)reboot\s+.*`),
	regexp.MustCompile(`(?i)halt\s+.*`),
	regexp.MustCompile(`(?i)poweroff\s+.*`),
	regexp.MustCompile(`(?i)killall\s+.*`),
	regexp.MustCompile(`(?i)pkill\s+.*`),
	regexp.MustCompile(`(?i)kill\s+-9`),
}

// NewCommandMonitor 创建新的命令监控器
func NewCommandMonitor(hostTag, clientID, username string) *CommandMonitor {
	// 确保日志目录存在
	logDir := "/tmp/t2t/commands"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Printf("无法创建日志目录: %v", err)
	}

	// 获取用户的主目录
	homeDir := "/home/" + username
	if userInfo, err := osuser.Lookup(username); err == nil {
		homeDir = userInfo.HomeDir
	}

	cm := &CommandMonitor{
		hostTag:     hostTag,
		clientID:    clientID,
		user:        username,
		logFile:     filepath.Join(logDir, fmt.Sprintf("commands_%s_%s.json", hostTag, clientID)),
		statsFile:   filepath.Join(logDir, fmt.Sprintf("stats_%s_%s.json", hostTag, clientID)),
		historyFile: filepath.Join(homeDir, ".bash_history"),
		stats:       make(map[string]int),
		stopChan:    make(chan struct{}),
	}

	// 加载现有统计
	cm.loadStats()

	// 获取初始历史文件大小
	if info, err := os.Stat(cm.historyFile); err == nil {
		cm.lastHistorySize = info.Size()
	}

	return cm
}

// Start 启动命令监控
func (cm *CommandMonitor) Start() {
	go cm.monitorHistory()
	log.Printf("命令监控已启动: %s", cm.historyFile)
}

// Stop 停止命令监控
func (cm *CommandMonitor) Stop() {
	close(cm.stopChan)
}

// 监控历史文件
func (cm *CommandMonitor) monitorHistory() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-cm.stopChan:
			return
		case <-ticker.C:
			cm.checkHistoryFile()
		}
	}
}

// 检查历史文件变化
func (cm *CommandMonitor) checkHistoryFile() {
	info, err := os.Stat(cm.historyFile)
	if err != nil {
		return
	}

	if info.Size() > cm.lastHistorySize {
		// 文件大小增加，说明有新命令
		cm.processNewCommands()
		cm.lastHistorySize = info.Size()
	}
}

// 处理新命令
func (cm *CommandMonitor) processNewCommands() {
	file, err := os.Open(cm.historyFile)
	if err != nil {
		return
	}
	defer file.Close()

	// 移动到上次读取的位置
	if _, err := file.Seek(cm.lastHistorySize, 0); err != nil {
		return
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		command := strings.TrimSpace(scanner.Text())
		if command != "" {
			cm.processCommand(command)
		}
	}
}

// 处理单个命令
func (cm *CommandMonitor) processCommand(rawCommand string) {
	// 清理命令
	command := cm.cleanCommand(rawCommand)
	if command == "" {
		return
	}

	// 检查是否是危险命令
	isDangerous := cm.isDangerousCommand(command)

	// 创建命令信息
	cmdInfo := CommandInfo{
		Timestamp:   time.Now(),
		HostTag:     cm.hostTag,
		ClientID:    cm.clientID,
		Command:     command,
		RawCommand:  rawCommand,
		User:        cm.user,
		WorkingDir:  cm.getWorkingDir(),
		IsDangerous: isDangerous,
	}

	// 记录命令
	cm.logCommand(cmdInfo)

	// 更新统计
	cm.updateStats(command, isDangerous)

	// 如果是危险命令，发出警告
	if isDangerous {
		cm.alertDangerousCommand(cmdInfo)
	}
}

// 清理命令
func (cm *CommandMonitor) cleanCommand(command string) string {
	// 移除注释
	if idx := strings.Index(command, "#"); idx != -1 {
		command = command[:idx]
	}

	// 移除前后空白
	command = strings.TrimSpace(command)

	// 跳过空命令
	if command == "" {
		return ""
	}

	// 跳过历史命令（以数字开头的）
	if matched, _ := regexp.MatchString(`^\d+\s+`, command); matched {
		return ""
	}

	return command
}

// 检查是否是危险命令
func (cm *CommandMonitor) isDangerousCommand(command string) bool {
	for _, pattern := range dangerousPatterns {
		if pattern.MatchString(command) {
			return true
		}
	}
	return false
}

// 获取工作目录
func (cm *CommandMonitor) getWorkingDir() string {
	if dir, err := os.Getwd(); err == nil {
		return dir
	}
	return "unknown"
}

// 记录命令
func (cm *CommandMonitor) logCommand(cmdInfo CommandInfo) {
	// 转换为JSON
	data, err := json.Marshal(cmdInfo)
	if err != nil {
		log.Printf("序列化命令信息失败: %v", err)
		return
	}

	// 追加到日志文件
	file, err := os.OpenFile(cm.logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("无法打开日志文件: %v", err)
		return
	}
	defer file.Close()

	// 写入JSON行
	if _, err := file.Write(append(data, '\n')); err != nil {
		log.Printf("写入日志文件失败: %v", err)
	}

	// 输出到控制台
	log.Printf("捕获命令: [%s] %s: %s", cmdInfo.Timestamp.Format("2006-01-02 15:04:05"), cm.hostTag, cmdInfo.Command)
}

// 更新统计
func (cm *CommandMonitor) updateStats(command string, isDangerous bool) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	// 提取命令名称
	parts := strings.Fields(command)
	if len(parts) > 0 {
		cmdName := parts[0]
		cm.stats[cmdName]++
	}

	if isDangerous {
		cm.stats["dangerous_commands"]++
	}

	cm.stats["total_commands"]++

	// 保存统计
	cm.saveStats()
}

// 保存统计
func (cm *CommandMonitor) saveStats() {
	data, err := json.Marshal(cm.stats)
	if err != nil {
		log.Printf("序列化统计信息失败: %v", err)
		return
	}

	if err := os.WriteFile(cm.statsFile, data, 0644); err != nil {
		log.Printf("保存统计信息失败: %v", err)
	}
}

// 加载统计
func (cm *CommandMonitor) loadStats() {
	data, err := os.ReadFile(cm.statsFile)
	if err != nil {
		return
	}

	if err := json.Unmarshal(data, &cm.stats); err != nil {
		log.Printf("加载统计信息失败: %v", err)
	}
}

// 警告危险命令
func (cm *CommandMonitor) alertDangerousCommand(cmdInfo CommandInfo) {
	alertMsg := fmt.Sprintf("⚠️  危险命令警告 ⚠️\n"+
		"时间: %s\n"+
		"主机: %s\n"+
		"用户: %s\n"+
		"命令: %s\n"+
		"工作目录: %s\n",
		cmdInfo.Timestamp.Format("2006-01-02 15:04:05"),
		cmdInfo.HostTag,
		cmdInfo.User,
		cmdInfo.Command,
		cmdInfo.WorkingDir)

	log.Printf("危险命令警告:\n%s", alertMsg)
}

// GetStats 获取统计信息
func (cm *CommandMonitor) GetStats() map[string]int {
	cm.mutex.RLock()
	defer cm.mutex.RUnlock()

	stats := make(map[string]int)
	for k, v := range cm.stats {
		stats[k] = v
	}

	return stats
}

// GetRecentCommands 获取最近的命令
func (cm *CommandMonitor) GetRecentCommands(limit int) []CommandInfo {
	file, err := os.Open(cm.logFile)
	if err != nil {
		return nil
	}
	defer file.Close()

	var commands []CommandInfo
	scanner := bufio.NewScanner(file)

	// 读取最后limit行
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > limit {
			lines = lines[1:]
		}
	}

	// 解析JSON
	for _, line := range lines {
		var cmdInfo CommandInfo
		if err := json.Unmarshal([]byte(line), &cmdInfo); err == nil {
			commands = append(commands, cmdInfo)
		}
	}

	return commands
}

// 设置shell历史记录
func (cm *CommandMonitor) setupShellHistory() error {
	// 确保bash历史记录正确配置
	historyConfig := `
# 设置历史记录大小
export HISTSIZE=10000
export HISTFILESIZE=20000

# 设置历史记录格式（包含时间戳）
export HISTTIMEFORMAT="%Y-%m-%d %T "

# 忽略重复命令
export HISTCONTROL=ignoredups

# 忽略特定命令
export HISTIGNORE="ls:ll:cd:pwd:clear:history"

# 立即写入历史记录
export PROMPT_COMMAND="history -a"
`

	// 写入到用户的bashrc
	bashrcFile := fmt.Sprintf("/home/%s/.bashrc", cm.user)
	if file, err := os.OpenFile(bashrcFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
		defer file.Close()
		file.WriteString(historyConfig)
		log.Printf("已配置bash历史记录: %s", bashrcFile)
	}

	return nil
}
