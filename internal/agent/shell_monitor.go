package agent

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ShellMonitor shell监控器
type ShellMonitor struct {
	hostTag     string
	clientID    string
	user        string
	logFile     string
	statsFile   string
	mutex       sync.RWMutex
	stats       map[string]int
	stopChan    chan struct{}
	lastCommand string
}

var historyTimestampPrefix = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}\s+\d{2}:\d{2}:\d{2}\s+`)

// NewShellMonitor 创建新的shell监控器
func NewShellMonitor(hostTag, clientID, user string) *ShellMonitor {
	// 确保日志目录存在
	logDir := "/tmp/t2t/commands"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Printf("无法创建日志目录: %v", err)
	}

	sm := &ShellMonitor{
		hostTag:   hostTag,
		clientID:  clientID,
		user:      user,
		logFile:   filepath.Join(logDir, fmt.Sprintf("commands_%s_%s.json", hostTag, clientID)),
		statsFile: filepath.Join(logDir, fmt.Sprintf("stats_%s_%s.json", hostTag, clientID)),
		stats:     make(map[string]int),
		stopChan:  make(chan struct{}),
	}

	// 加载现有统计
	sm.loadStats()

	return sm
}

// Start 启动shell监控
func (sm *ShellMonitor) Start() {
	// 设置shell环境来捕获命令
	sm.setupShellMonitoring()

	go sm.monitorShellOutput()
	log.Printf("Shell监控已启动")
}

// Stop 停止shell监控
func (sm *ShellMonitor) Stop() {
	close(sm.stopChan)
}

// 设置shell监控
func (sm *ShellMonitor) setupShellMonitoring() {
	// 创建监控脚本
	monitorScript := fmt.Sprintf(`#!/bin/bash
if [ -f ~/.bashrc ]; then
  source ~/.bashrc
fi

# 命令监控脚本
export HISTTIMEFORMAT="%%Y-%%m-%%d %%T "
export HISTSIZE=10000
export HISTFILESIZE=20000
export HISTCONTROL=ignoredups

# 自定义PROMPT_COMMAND来捕获命令
export PROMPT_COMMAND="history -a; echo 'T2T_CMD_MONITOR:'\$(history 1 | sed 's/^[ ]*[0-9]\+[ ]*//') >> /tmp/t2t_shell_monitor.log"

# 设置别名来捕获更多信息
alias t2t_monitor='echo "T2T_CMD_EXEC: $BASH_COMMAND" >> /tmp/t2t_shell_monitor.log'

# 设置trap来捕获命令执行
trap 'echo "T2T_CMD_EXIT: $? $BASH_COMMAND" >> /tmp/t2t_shell_monitor.log' DEBUG
`)

	// 写入监控脚本
	scriptFile := "/tmp/t2t_shell_monitor.sh"
	if err := os.WriteFile(scriptFile, []byte(monitorScript), 0755); err != nil {
		log.Printf("无法创建监控脚本: %v", err)
		return
	}

	promptCommand := `history -a; echo 'T2T_CMD_MONITOR:'$(history 1 | sed 's/^[ ]*[0-9]\+[ ]*//') >> /tmp/t2t_shell_monitor.log`

	// 关键：在启动 shell 前将监控变量注入环境，确保 shell 进程可见
	os.Setenv("HISTTIMEFORMAT", "%Y-%m-%d %T ")
	os.Setenv("HISTSIZE", "10000")
	os.Setenv("HISTFILESIZE", "20000")
	os.Setenv("HISTCONTROL", "ignoredups")
	os.Setenv("PROMPT_COMMAND", promptCommand)
	os.Setenv("BASH_ENV", scriptFile)
	os.Setenv("ENV", scriptFile)
	os.Setenv("T2T_MONITOR_SCRIPT", scriptFile)

	// 设置环境变量
	os.Setenv("T2T_MONITOR_ENABLED", "1")
	os.Setenv("T2T_HOST_TAG", sm.hostTag)
	os.Setenv("T2T_CLIENT_ID", sm.clientID)
	os.Setenv("T2T_USER", sm.user)
}

// 监控shell输出
func (sm *ShellMonitor) monitorShellOutput() {
	monitorFile := "/tmp/t2t_shell_monitor.log"

	// 创建监控文件
	file, err := os.Create(monitorFile)
	if err != nil {
		log.Printf("无法创建监控文件: %v", err)
		return
	}
	file.Close()

	// 监控文件变化
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var lastSize int64

	for {
		select {
		case <-sm.stopChan:
			return
		case <-ticker.C:
			sm.checkMonitorFile(monitorFile, &lastSize)
		}
	}
}

// 检查监控文件
func (sm *ShellMonitor) checkMonitorFile(filename string, lastSize *int64) {
	info, err := os.Stat(filename)
	if err != nil {
		return
	}

	if info.Size() > *lastSize {
		sm.processMonitorFile(filename, *lastSize)
		*lastSize = info.Size()
	}
}

// 处理监控文件
func (sm *ShellMonitor) processMonitorFile(filename string, offset int64) {
	file, err := os.Open(filename)
	if err != nil {
		return
	}
	defer file.Close()

	// 移动到上次读取的位置
	if _, err := file.Seek(offset, 0); err != nil {
		return
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		sm.processMonitorLine(line)
	}
}

// 处理监控行
func (sm *ShellMonitor) processMonitorLine(line string) {
	if strings.HasPrefix(line, "T2T_CMD_MONITOR:") {
		// 从PROMPT_COMMAND捕获的命令
		command := strings.TrimPrefix(line, "T2T_CMD_MONITOR:")
		command = strings.TrimSpace(command)
		if command != "" && command != sm.lastCommand {
			sm.lastCommand = command
			sm.processCommand(command)
		}
	} else if strings.HasPrefix(line, "T2T_CMD_EXEC:") {
		// 从trap捕获的命令
		command := strings.TrimPrefix(line, "T2T_CMD_EXEC:")
		command = strings.TrimSpace(command)
		if command != "" && command != sm.lastCommand {
			sm.lastCommand = command
			sm.processCommand(command)
		}
	}
}

// 处理命令
func (sm *ShellMonitor) processCommand(rawCommand string) {
	// 清理命令
	command := sm.cleanCommand(rawCommand)
	if command == "" {
		return
	}

	// 检查是否是危险命令
	isDangerous := sm.isDangerousCommand(command)

	// 创建命令信息
	cmdInfo := CommandInfo{
		Timestamp:   time.Now(),
		HostTag:     sm.hostTag,
		ClientID:    sm.clientID,
		Command:     command,
		RawCommand:  rawCommand,
		User:        sm.user,
		WorkingDir:  sm.getWorkingDir(),
		IsDangerous: isDangerous,
	}

	// 记录命令
	sm.logCommand(cmdInfo)

	// 更新统计
	sm.updateStats(command, isDangerous)

	// 如果是危险命令，发出警告
	if isDangerous {
		sm.alertDangerousCommand(cmdInfo)
	}
}

// 清理命令
func (sm *ShellMonitor) cleanCommand(command string) string {
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

	// 跳过监控相关的命令
	if strings.Contains(command, "T2T_") {
		return ""
	}

	// 去掉历史时间戳前缀，保留纯命令文本
	command = historyTimestampPrefix.ReplaceAllString(command, "")
	command = strings.TrimSpace(command)
	if command == "" {
		return ""
	}

	return command
}

// 检查是否是危险命令
func (sm *ShellMonitor) isDangerousCommand(command string) bool {
	for _, pattern := range dangerousPatterns {
		if pattern.MatchString(command) {
			return true
		}
	}
	return false
}

// 获取工作目录
func (sm *ShellMonitor) getWorkingDir() string {
	if dir, err := os.Getwd(); err == nil {
		return dir
	}
	return "unknown"
}

// 记录命令
func (sm *ShellMonitor) logCommand(cmdInfo CommandInfo) {
	// 转换为JSON
	data, err := json.Marshal(cmdInfo)
	if err != nil {
		log.Printf("序列化命令信息失败: %v", err)
		return
	}

	// 追加到日志文件
	file, err := os.OpenFile(sm.logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
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
	log.Printf("捕获命令: [%s] %s: %s", cmdInfo.Timestamp.Format("2006-01-02 15:04:05"), sm.hostTag, cmdInfo.Command)
}

// 更新统计
func (sm *ShellMonitor) updateStats(command string, isDangerous bool) {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	// 提取命令名称
	parts := strings.Fields(command)
	if len(parts) > 0 {
		cmdName := parts[0]
		sm.stats[cmdName]++
	}

	if isDangerous {
		sm.stats["dangerous_commands"]++
	}

	sm.stats["total_commands"]++

	// 保存统计
	sm.saveStats()
}

// 保存统计
func (sm *ShellMonitor) saveStats() {
	data, err := json.Marshal(sm.stats)
	if err != nil {
		log.Printf("序列化统计信息失败: %v", err)
		return
	}

	if err := os.WriteFile(sm.statsFile, data, 0644); err != nil {
		log.Printf("保存统计信息失败: %v", err)
	}
}

// 加载统计
func (sm *ShellMonitor) loadStats() {
	data, err := os.ReadFile(sm.statsFile)
	if err != nil {
		return
	}

	if err := json.Unmarshal(data, &sm.stats); err != nil {
		log.Printf("加载统计信息失败: %v", err)
	}
}

// 警告危险命令
func (sm *ShellMonitor) alertDangerousCommand(cmdInfo CommandInfo) {
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
func (sm *ShellMonitor) GetStats() map[string]int {
	sm.mutex.RLock()
	defer sm.mutex.RUnlock()

	stats := make(map[string]int)
	for k, v := range sm.stats {
		stats[k] = v
	}

	return stats
}

// GetRecentCommands 获取最近的命令
func (sm *ShellMonitor) GetRecentCommands(limit int) []CommandInfo {
	file, err := os.Open(sm.logFile)
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
