package browser

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExistingAccountContainer(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "runtime", "1")
	os.MkdirAll(dir, 0700)
	os.WriteFile(filepath.Join(dir, "worker-token"), []byte("private-token"), 0600)
	var info inspection
	info.State.Running = true
	info.Config.Labels = map[string]string{"order-processor.profile-layout": "container-v2", "order-processor.account": "1", "order-processor.data": root}
	json.Unmarshal([]byte(`{"Ports":{"18080/tcp":[{"HostIP":"127.0.0.1","HostPort":"31001"}],"6080/tcp":[{"HostIP":"127.0.0.1","HostPort":"31002"}]}}`), &info.NetworkSettings)
	d := Docker{DataDir: root, Prefix: "op-browser-", Run: func(_ context.Context, args ...string) ([]byte, error) {
		if strings.Join(args, " ") != "inspect op-browser-1" {
			t.Fatalf("unexpected command %v", args)
		}
		return json.Marshal([]inspection{info})
	}}
	target, e := d.Resolve(context.Background(), "1", false)
	if e != nil || target.Token != "private-token" || target.Desktop.String() != "http://127.0.0.1:31002" {
		t.Fatal(target.Desktop, e)
	}
	info.Config.Labels["order-processor.profile-layout"] = "container-v1"
	if _, e = d.Resolve(context.Background(), "1", false); e == nil || !strings.Contains(e.Error(), "重建") {
		t.Fatal("old layout accepted", e)
	}
	info.Config.Labels["order-processor.profile-layout"] = "container-v2"
	info.Config.Labels["order-processor.account"] = "2"
	if _, e = d.Resolve(context.Background(), "1", false); e == nil {
		t.Fatal("wrong account accepted")
	}
}
func TestMissingStatusNeverCreatesContainer(t *testing.T) {
	d := Docker{Run: func(_ context.Context, args ...string) ([]byte, error) {
		switch args[0] {
		case "inspect":
			return nil, errors.New("missing")
		case "ps":
			return nil, nil
		default:
			t.Fatalf("unexpected mutation %v", args)
			return nil, nil
		}
	}}
	if _, e := d.Resolve(context.Background(), "1", false); !errors.Is(e, ErrNotStarted) {
		t.Fatal(e)
	}
	if _, e := d.Resolve(context.Background(), "../1", true); e == nil {
		t.Fatal("invalid account accepted")
	}
}

func TestContainerProfilesDoNotReuseNativeChrome(t *testing.T) {
	d := Docker{DataDir: "/data/company"}
	args := strings.Join(d.mountArgs("12"), " ")
	if !strings.Contains(args, "src=/data/company/container-profiles/12,dst=/data/profiles/12") {
		t.Fatal(args)
	}
	if strings.Contains(args, "src=/data/company/profiles/") || strings.Contains(args, "orders.db") {
		t.Fatal("native profile or database exposed", args)
	}
	if !strings.Contains(args, "src=/data/company/snapshots/12,dst=/data/snapshots/12") {
		t.Fatal(args)
	}
	if !strings.Contains(args, "src=/data/company/raw/12,dst=/data/raw/12") {
		t.Fatal(args)
	}
}
func TestDockerStartupChecks(t *testing.T) {
	for _, failure := range []string{"info", "image", ""} {
		commands := []string{}
		d := Docker{Image: "test-browser", Run: func(_ context.Context, args ...string) ([]byte, error) {
			commands = append(commands, args[0])
			if args[0] == failure {
				return nil, errors.New("unavailable")
			}
			return nil, nil
		}}
		err := d.Check(context.Background())
		if (err != nil) != (failure != "") {
			t.Fatalf("%q: %v", failure, err)
		}
		if failure == "info" && len(commands) != 1 {
			t.Fatal("should stop on unavailable daemon")
		}
	}
}

func TestNewContainerCreatesSnapshotMount(t *testing.T) {
	root := t.TempDir()
	ran := false
	d := Docker{DataDir: root, Prefix: "op-browser-", Run: func(_ context.Context, args ...string) ([]byte, error) {
		switch args[0] {
		case "inspect":
			return nil, errors.New("missing")
		case "ps":
			return nil, nil
		case "run":
			ran = true
			joined := strings.Join(args, " ")
			if !strings.Contains(joined, "order-processor.profile-layout=container-v2") || !strings.Contains(joined, "src="+filepath.Join(root, "snapshots", "1")+",dst=/data/snapshots/1") {
				t.Fatal(args)
			}
			for _, part := range []string{"container-profiles", "raw", "runtime", "snapshots"} {
				if info, err := os.Stat(filepath.Join(root, part, "1")); err != nil || !info.IsDir() {
					t.Fatal(part, err)
				}
			}
			return nil, errors.New("stop before actual Docker")
		default:
			t.Fatalf("unexpected command %v", args)
			return nil, nil
		}
	}}
	if _, err := d.Resolve(context.Background(), "1", true); err == nil || !ran {
		t.Fatal(ran, err)
	}
}
