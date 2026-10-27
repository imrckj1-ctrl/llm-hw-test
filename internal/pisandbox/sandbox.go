// Package pisandbox pi 容器沙箱状态探测与自动构建(V2.23):
// Docker 已装但镜像缺失 → 自动后台执行 deploy/pi/build_image.sh;
// 用户唯一需要自行安装的是 Docker 本身。
package pisandbox

import (
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// Status 沙箱实时状态
type Status struct {
	Docker   bool `json:"docker"`   // docker version 通
	Image    bool `json:"image"`    // hwtest-pi:local 已构建
	Building bool `json:"building"` // 正在后台构建
}

var (
	mu        sync.Mutex
	building  bool
	lastBuild time.Time
)

// Detect 实时探测( docker version + docker image inspect )
func Detect() Status {
	s := Status{}
	if err := exec.Command("docker", "version").Run(); err != nil {
		return s
	}
	s.Docker = true
	s.Image = exec.Command("docker", "image", "inspect", "hwtest-pi:local").Run() == nil
	mu.Lock()
	s.Building = building
	mu.Unlock()
	return s
}

// EnsureBuiltAsync Docker 已装但镜像缺失时,自动后台构建(幂等,不重复起)
func EnsureBuiltAsync() {
	mu.Lock()
	defer mu.Unlock()
	if building || time.Since(lastBuild) < 10*time.Minute {
		return
	}
	if _, err := exec.LookPath("docker"); err != nil {
		return
	}
	if exec.Command("docker", "image", "inspect", "hwtest-pi:local").Run() == nil {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	script := filepath.Join(filepath.Dir(exe), "deploy", "pi", "build_image.sh")
	if _, err := os.Stat(script); err != nil {
		return
	}
	building = true
	go func() {
		defer func() {
			mu.Lock()
			building = false
			lastBuild = time.Now()
			mu.Unlock()
		}()
		out, _ := os.OpenFile(filepath.Join(os.TempDir(), "hwtest-pi-build.log"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if out != nil {
			defer out.Close()
		}
		cmd := exec.Command("bash", script)
		cmd.Stdout, cmd.Stderr = out, out
		_ = cmd.Run()
	}()
}
