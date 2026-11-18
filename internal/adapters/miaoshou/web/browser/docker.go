package browser

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrNotStarted = errors.New("browser container not started")
var accountKey = regexp.MustCompile(`^(?:[a-f0-9]{32}|[1-9][0-9]*)$`)

type Docker struct {
	DataDir, Image, Prefix string
	mu                     sync.Mutex
	locks                  map[string]*sync.Mutex
	Run                    func(context.Context, ...string) ([]byte, error)
}

func (d *Docker) Remote() bool { return true }

// Fail at startup with actionable instructions rather than hiding Docker
// problems behind every account's polling request.
func (d *Docker) Check(ctx context.Context) error {
	if _, err := d.run(ctx, "info", "--format", "{{.ServerVersion}}"); err != nil {
		return fmt.Errorf("Docker 服务不可用：Mac 请先启动 Docker Desktop，并确认当前终端可执行 docker info")
	}
	if _, err := d.run(ctx, "image", "inspect", d.Image); err != nil {
		return fmt.Errorf("缺少浏览器镜像 %s，请先执行 npm run start:remote 构建并启动", d.Image)
	}
	return nil
}
func (d *Docker) mountArgs(key string) []string {
	args := []string{}
	for _, part := range []string{"profiles", "raw", "runtime", "snapshots"} {
		hostPart := part
		if part == "profiles" {
			hostPart = "container-profiles"
		}
		args = append(args, "--mount", "type=bind,src="+filepath.Join(d.DataDir, hostPart, key)+",dst=/data/"+part+"/"+key)
	}
	return args
}

func (d *Docker) lock(key string) *sync.Mutex {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.locks == nil {
		d.locks = map[string]*sync.Mutex{}
	}
	if d.locks[key] == nil {
		d.locks[key] = &sync.Mutex{}
	}
	return d.locks[key]
}
func (d *Docker) run(ctx context.Context, args ...string) ([]byte, error) {
	if d.Run != nil {
		return d.Run(ctx, args...)
	}
	return exec.CommandContext(ctx, "docker", args...).Output()
}

type inspection struct {
	ID              string
	State           struct{ Running bool }
	Config          struct{ Labels map[string]string }
	NetworkSettings struct {
		Ports map[string][]struct{ HostIP, HostPort string }
	}
}

func (d *Docker) inspect(ctx context.Context, name string) (inspection, error) {
	var result []inspection
	b, e := d.run(ctx, "inspect", name)
	if e != nil {
		return inspection{}, e
	}
	if e = json.Unmarshal(b, &result); e != nil || len(result) != 1 {
		return inspection{}, runtimeError("container_inspection_failed", "容器检查失败")
	}
	return result[0], nil
}
func localPort(v inspection, key string) (*url.URL, error) {
	ports := v.NetworkSettings.Ports[key]
	if len(ports) != 1 || ports[0].HostIP != "127.0.0.1" {
		return nil, runtimeError("container_port_invalid", "容器端口配置不正确")
	}
	p, e := strconv.Atoi(ports[0].HostPort)
	if e != nil || p < 1 || p > 65535 {
		return nil, runtimeError("container_port_invalid", "容器端口无效")
	}
	return url.Parse("http://127.0.0.1:" + ports[0].HostPort)
}
func (d *Docker) Resolve(ctx context.Context, key string, create bool) (Target, error) {
	return d.resolve(ctx, key, create, false)
}

func (d *Docker) Restart(ctx context.Context, key string) (Target, error) {
	return d.resolve(ctx, key, false, true)
}

func (d *Docker) resolve(ctx context.Context, key string, create, restart bool) (Target, error) {
	if !accountKey.MatchString(key) {
		return Target{}, runtimeError("account_invalid", "账户标识无效")
	}
	lock := d.lock(key)
	lock.Lock()
	defer lock.Unlock()
	name := d.Prefix + key
	info, e := d.inspect(ctx, name)
	if e != nil {
		// Distinguish an absent container from an unavailable Docker daemon.
		b, listErr := d.run(ctx, "ps", "-a", "--filter", "name=^/"+name+"$", "--format", "{{.Names}}")
		if listErr != nil {
			return Target{}, runtimeError("docker_unavailable", "Docker 不可用，请检查服务和运行权限")
		}
		if strings.TrimSpace(string(b)) != "" {
			return Target{}, runtimeError("container_inspection_failed", "无法读取已有浏览器容器")
		}
		if !create {
			return Target{}, ErrNotStarted
		}
	}
	dir := filepath.Join(d.DataDir, "runtime", key)
	tokenFile := filepath.Join(dir, "worker-token")
	var token []byte
	if e == nil {
		if info.Config.Labels["order-processor.account"] != key || info.Config.Labels["order-processor.data"] != d.DataDir {
			return Target{}, runtimeError("container_owner_mismatch", "同名容器不属于当前账户或数据目录")
		}
		if info.Config.Labels["order-processor.profile-layout"] != "container-v2" {
			return Target{}, runtimeError("container_rebuild_required", "旧浏览器容器需要重建以挂载订单完整范围凭证目录；请先停止同步，保留全部数据目录后重建该账户容器")
		}
		token, e = os.ReadFile(tokenFile)
		if e != nil {
			return Target{}, runtimeError("container_session_missing", "已有容器的会话凭据缺失")
		}
		if restart {
			if _, e = d.run(ctx, "restart", info.ID); e != nil {
				return Target{}, runtimeError("container_restart_failed", "重启账户浏览器容器失败，请查看 Docker 状态")
			}
		} else if !info.State.Running {
			if !create {
				return Target{}, ErrNotStarted
			}
			if _, e = d.run(ctx, "start", name); e != nil {
				return Target{}, runtimeError("container_start_failed", "启动浏览器容器失败")
			}
		}
	} else {
		for _, p := range []string{dir, filepath.Join(d.DataDir, "raw", key), filepath.Join(d.DataDir, "snapshots", key), filepath.Join(d.DataDir, "container-profiles", key)} {
			if e = os.MkdirAll(p, 0700); e != nil {
				return Target{}, e
			}
		}
		token, e = os.ReadFile(tokenFile)
		if os.IsNotExist(e) {
			secret := make([]byte, 32)
			if _, e = rand.Read(secret); e != nil {
				return Target{}, e
			}
			token = []byte(hex.EncodeToString(secret))
			if e = os.WriteFile(tokenFile, token, 0600); e != nil {
				return Target{}, e
			}
		} else if e != nil {
			return Target{}, e
		}
		args := []string{"run", "-d", "--name", name, "--init", "--restart", "unless-stopped", "--label", "order-processor.account=" + key, "--label", "order-processor.data=" + d.DataDir, "--label", "order-processor.profile-layout=container-v2", "--shm-size", "1g", "--memory", "3g", "--cpus", "2", "--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()), "-p", "127.0.0.1::18080", "-p", "127.0.0.1::6080", "--env", "ORDER_ACCOUNT_KEY=" + key, "--env", "BROWSER_TOKEN_FILE=/data/runtime/" + key + "/worker-token"}
		args = append(args, d.mountArgs(key)...)
		args = append(args, d.Image)
		if _, e = d.run(ctx, args...); e != nil {
			return Target{}, runtimeError("container_create_failed", "创建浏览器容器失败，请检查镜像、挂载目录和 Docker 日志")
		}
	}
	info, e = d.inspect(ctx, name)
	if e != nil {
		return Target{}, e
	}
	api, e := localPort(info, "18080/tcp")
	if e != nil {
		return Target{}, e
	}
	desktop, e := localPort(info, "6080/tcp")
	if e != nil {
		return Target{}, e
	}
	target := Target{API: api, Desktop: desktop, Token: strings.TrimSpace(string(token))}
	if create || restart {
		if e = waitReady(ctx, target, key); e != nil {
			return Target{}, e
		}
	}
	return target, nil
}
func waitReady(ctx context.Context, t Target, key string) error {
	timer := time.NewTimer(40 * time.Second)
	defer timer.Stop()
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	client := http.Client{Timeout: 2 * time.Second}
	for {
		req, _ := http.NewRequestWithContext(ctx, "GET", t.API.String()+"/accounts/"+key+"/status", nil)
		req.Header.Set("Authorization", "Bearer "+t.Token)
		r, e := client.Do(req)
		if e == nil {
			r.Body.Close()
			if r.StatusCode == 200 {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return runtimeError("worker_start_timeout", "浏览器容器启动超时，请查看容器日志")
		case <-tick.C:
		}
	}
}

// Stop the owned browser only; the profile and order archives remain on disk.
func (d *Docker) Stop(ctx context.Context, key string) error {
	if !accountKey.MatchString(key) {
		return runtimeError("account_invalid", "账户标识无效")
	}
	lock := d.lock(key)
	lock.Lock()
	defer lock.Unlock()
	info, err := d.inspect(ctx, d.Prefix+key)
	if err != nil {
		return err
	}
	if info.Config.Labels["order-processor.account"] != key || info.Config.Labels["order-processor.data"] != d.DataDir {
		return runtimeError("container_owner_mismatch", "同名容器不属于当前账户或数据目录")
	}
	if info.State.Running {
		if _, err = d.run(ctx, "stop", info.ID); err != nil {
			return runtimeError("container_stop_failed", "停止账户浏览器容器失败")
		}
	}
	return nil
}
