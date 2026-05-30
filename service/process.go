package service

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

type subProcess struct {
	cmd  *exec.Cmd
	done chan struct{}
}

var (
	runningSubServices   = make(map[string]subProcess)
	runningSubServicesMu sync.Mutex
)

func GetSubLogDir() string {
	return filepath.Join("/var/log/msg-service", "subservices")
}

func GetSubLogPath(name string) string {
	return filepath.Join(GetSubLogDir(), time.Now().Format("2006-01-02"), name+".log")
}

func findSubLogFile(name string) string {
	current := GetSubLogPath(name)
	if _, err := os.Stat(current); err == nil {
		return current
	}

	entries, err := os.ReadDir(GetSubLogDir())
	if err != nil {
		return current
	}

	var candidates []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		candidate := filepath.Join(GetSubLogDir(), entry.Name(), name+".log")
		if _, err := os.Stat(candidate); err == nil {
			candidates = append(candidates, candidate)
		}
	}

	sort.Sort(sort.Reverse(sort.StringSlice(candidates)))
	if len(candidates) > 0 {
		return candidates[0]
	}
	return current
}

func ReadSubLog(name string) (string, error) {
	data, err := os.ReadFile(findSubLogFile(name))
	if err != nil {
		return "", err
	}

	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) > 200 {
		lines = lines[len(lines)-200:]
	}
	return strings.Join(lines, "\n"), nil
}

// GetPIDByName 获取进程 PID（跳过僵尸进程）
func GetPIDByName(name string) int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}

	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}

		statPath := fmt.Sprintf("/proc/%d/stat", pid)
		statData, err := os.ReadFile(statPath)
		if err != nil {
			continue
		}

		idx := strings.LastIndex(string(statData), ")")
		if idx < 0 || idx+2 >= len(statData) {
			continue
		}
		fields := strings.Fields(string(statData)[idx+2:])
		if len(fields) >= 2 && fields[0] == "Z" {
			continue
		}

		cmdLinePath := fmt.Sprintf("/proc/%d/cmdline", pid)
		cmdLine, err := os.ReadFile(cmdLinePath)
		if err != nil {
			continue
		}
		if strings.Contains(strings.ReplaceAll(string(cmdLine), "\x00", " "), name) {
			return pid
		}
	}

	return 0
}

// GetPGIDByName 获取进程组 ID
func GetPGIDByName(name string) int {
	pid := GetPIDByName(name)
	if pid == 0 {
		return 0
	}
	// 读取 /proc/pid/stat 中的进程组ID（第5字段）
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0
	}
	fields := strings.Fields(string(data))
	if len(fields) >= 5 {
		pgid, _ := strconv.Atoi(fields[4])
		return pgid
	}
	return 0
}

type timestampWriter struct {
	w io.Writer
}

func (tw *timestampWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}

	text := strings.TrimRight(string(p), "\n")
	if text == "" {
		return len(p), nil
	}

	prefix := "[" + time.Now().Format("2006-01-02 15:04:05") + "] "
	_, err := tw.w.Write([]byte(prefix + strings.ReplaceAll(text, "\n", "\n"+prefix)))
	if err != nil {
		return 0, err
	}
	if strings.HasSuffix(string(p), "\n") {
		_, err = tw.w.Write([]byte("\n"))
	}
	return len(p), err
}

func getSubLogRetentionDays() int {
	data, err := os.ReadFile("/opt/msg-service/config.yaml")
	if err != nil {
		return 14
	}
	var cfg struct {
		Server struct {
			LogMaxDays int `yaml:"log_max_days"`
		} `yaml:"server"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil || cfg.Server.LogMaxDays <= 0 {
		return 14
	}
	return cfg.Server.LogMaxDays
}

func pruneOldSubLogs(dir string, keepDays int) {
	if keepDays <= 0 {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -keepDays).Format("2006-01-02")
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if entry.Name() >= cutoff {
			continue
		}
		_ = os.RemoveAll(filepath.Join(dir, entry.Name()))
	}
}

func StartSubService(name, binPath, workDir string) error {
	pruneOldSubLogs(GetSubLogDir(), getSubLogRetentionDays())

	runningSubServicesMu.Lock()
	if proc, ok := runningSubServices[name]; ok && proc.cmd != nil && proc.cmd.Process != nil {
		runningSubServicesMu.Unlock()
		return nil
	}
	runningSubServicesMu.Unlock()

	if err := os.MkdirAll(filepath.Dir(GetSubLogPath(name)), 0o755); err != nil {
		return err
	}

	logFile, err := os.OpenFile(GetSubLogPath(name), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}

	cmd := exec.Command(binPath)
	cmd.Dir = workDir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout = io.MultiWriter(os.Stdout, &timestampWriter{w: logFile})
	cmd.Stderr = io.MultiWriter(os.Stderr, &timestampWriter{w: logFile})

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return err
	}

	done := make(chan struct{})
	runningSubServicesMu.Lock()
	runningSubServices[name] = subProcess{cmd: cmd, done: done}
	runningSubServicesMu.Unlock()

	go func() {
		_ = cmd.Wait()
		_ = logFile.Close()
		close(done)

		runningSubServicesMu.Lock()
		if proc, ok := runningSubServices[name]; ok && proc.cmd == cmd {
			delete(runningSubServices, name)
		}
		runningSubServicesMu.Unlock()
	}()

	return nil
}

func StopSubService(name string) {
	runningSubServicesMu.Lock()
	proc, ok := runningSubServices[name]
	if ok {
		delete(runningSubServices, name)
	}
	runningSubServicesMu.Unlock()

	if ok && proc.cmd != nil && proc.cmd.Process != nil {
		_ = syscall.Kill(-proc.cmd.Process.Pid, syscall.SIGTERM)

		select {
		case <-proc.done:
			return
		case <-time.After(5 * time.Second):
			_ = syscall.Kill(-proc.cmd.Process.Pid, syscall.SIGKILL)
			select {
			case <-proc.done:
			case <-time.After(2 * time.Second):
			}
		}
		return
	}

	// 回退方案：按名称查找并停止
	pgid := GetPGIDByName(name)
	if pgid > 0 {
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
		for i := 0; i < 10; i++ {
			if GetPIDByName(name) == 0 {
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
		return
	}

	_ = exec.Command("pkill", "-f", name).Run()
}

func StopAll() {
	services := []string{
		"dingtalk_msg", "dingtalk_sync",
		"workwechat_msg", "workwechat_sync",
		"feishu_msg", "feishu_sync",
	}
	for _, svc := range services {
		StopSubService(svc)
	}
}
