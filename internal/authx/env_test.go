package authx

import (
	"slices"
	"testing"
)

func TestBridgeEnv(t *testing.T) {
	got := BridgeEnv(UserInfo{Name: "alice", UID: 1001, GID: 1001, Home: "/home/alice", Shell: "/bin/bash"})
	want := []string{
		"HOME=/home/alice", "USER=alice", "LOGNAME=alice", "SHELL=/bin/bash",
		"PATH=/usr/local/bin:/usr/bin:/bin", "LANG=C.UTF-8",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q", got)
	}
	if env := BridgeEnv(UserInfo{Name: "x", Home: "/", Shell: ""}); !slices.Contains(env, "SHELL=/bin/sh") {
		t.Fatalf("shell rỗng phải về /bin/sh: %q", env)
	}
}
