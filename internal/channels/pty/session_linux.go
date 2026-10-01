package pty

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

// sessionPIDs trả mọi pid còn sống (bỏ zombie) có session id = sid, đọc từ
// /proc/<pid>/stat.
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
		if len(f) > 3 && f[0] != "Z" && f[3] == strconv.Itoa(sid) {
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

// spared: tiến trình được tha khi đóng kênh — bỏ qua SIGHUP và KHÔNG phải chính
// shell (pid == sid). Shell đăng nhập không bao giờ là lệnh "cố ý tách riêng".
func spared(pid, sid int) bool { return pid != sid && ignoresHUP(pid) }

// killSession: SIGKILL mọi tiến trình còn trong session trừ những tiến trình
// được tha (Q3: nohup được sống, giống SSH).
func killSession(sid int) {
	killPasses(func() []int { return sessionPIDs(sid) }, func(pid int) bool { return spared(pid, sid) },
		func(pid int) { _ = syscall.Kill(pid, syscall.SIGKILL) })
}

// killPasses quét rồi giết, lặp tối đa 3 lượt tới khi một lượt quét không còn pid
// nào giết được: tiến trình fork giữa lúc quét và lúc giết thì con của nó chỉ
// hiện ở lượt sau.
func killPasses(scan func() []int, spare func(int) bool, kill func(int)) {
	for pass := 0; pass < 3; pass++ {
		n := 0
		for _, pid := range scan() {
			if !spare(pid) {
				kill(pid)
				n++
			}
		}
		if n == 0 {
			return
		}
	}
}

// sessionSettled: session không còn tiến trình nào mà killSession sẽ giết.
func sessionSettled(sid int) bool {
	for _, pid := range sessionPIDs(sid) {
		if !spared(pid, sid) {
			return false
		}
	}
	return true
}
