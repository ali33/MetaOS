package pty

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

// sessionPIDs trả mọi pid có session id = sid, đọc từ /proc/<pid>/stat.
func sessionPIDs(sid int) []int {
	ents, _ := os.ReadDir("/proc")
	var out []int
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		b, err := os.ReadFile("/proc/" + e.Name() + "/stat")
		if err != nil {
			continue
		}
		// Trường 2 (comm) có thể chứa dấu cách và ')' — tách sau ')' cuối cùng.
		s := string(b)
		i := strings.LastIndexByte(s, ')')
		if i < 0 {
			continue
		}
		f := strings.Fields(s[i+1:]) // f[0]=state f[1]=ppid f[2]=pgrp f[3]=session
		if len(f) > 3 && f[3] == strconv.Itoa(sid) {
			out = append(out, pid)
		}
	}
	return out
}

func signalSession(sid int, sig syscall.Signal) {
	for _, pid := range sessionPIDs(sid) {
		_ = syscall.Kill(pid, sig)
	}
}

// ignoresHUP: tiến trình đã cố ý bỏ qua SIGHUP (nohup) — bit 0 của SigIgn.
func ignoresHUP(pid int) bool {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/status")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(b), "\n") {
		if v, ok := strings.CutPrefix(line, "SigIgn:"); ok {
			m, err := strconv.ParseUint(strings.TrimSpace(v), 16, 64)
			return err == nil && m&1 == 1
		}
	}
	return false
}

// killSession: SIGKILL mọi tiến trình còn trong session trừ những tiến trình
// cố ý bỏ qua SIGHUP (Q3: nohup được sống, giống SSH).
func killSession(sid int) {
	for _, pid := range sessionPIDs(sid) {
		if !ignoresHUP(pid) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}
}
