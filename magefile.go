//go:build mage

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var (
	binaryName = "digen"
	cmdPath    = "./cmd/digen"
)

// getVersionInfo 从 git 获取版本信息
// version: git describe --tags --dirty（包含 tag、commit 数量、dirty）
// commit:  git rev-parse HEAD（完整 commit hash）
func getVersionInfo() (version, commit, buildDate string) {
	version = "dev"
	commit = "none"
	buildDate = time.Now().UTC().Format(time.RFC3339)

	if out, err := exec.Command("git", "describe", "--tags", "--dirty").Output(); err == nil {
		v := strings.TrimSpace(string(out))
		if v != "" {
			version = v
		}
	}

	if out, err := exec.Command("git", "rev-parse", "HEAD").Output(); err == nil {
		c := strings.TrimSpace(string(out))
		if c != "" {
			commit = c
		}
	}

	return
}

// ldflags 构造 ldflags 参数
func ldflags() string {
	version, commit, buildDate := getVersionInfo()
	return fmt.Sprintf("-X 'main.Version=%s' -X 'main.Commit=%s' -X 'main.BuildDate=%s'",
		version, commit, buildDate)
}

// Build 构建二进制文件，自动注入 git tag 版本号
func Build() error {
	version, commit, buildDate := getVersionInfo()

	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	out := filepath.Join("bin", binaryName+ext)

	fmt.Printf("Building %s...\n", out)
	fmt.Printf("  Version:   %s\n", version)
	fmt.Printf("  Commit:    %s\n", commit)
	fmt.Printf("  BuildDate: %s\n\n", buildDate)

	if err := os.MkdirAll("bin", 0755); err != nil {
		return err
	}

	cmd := exec.Command("go", "build", "-ldflags", ldflags(), "-o", out, cmdPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Install 安装到 $GOPATH/bin，注入版本号
func Install() error {
	version, commit, buildDate := getVersionInfo()

	fmt.Printf("Installing %s...\n", binaryName)
	fmt.Printf("  Version:   %s\n", version)
	fmt.Printf("  Commit:    %s\n", commit)
	fmt.Printf("  BuildDate: %s\n\n", buildDate)

	cmd := exec.Command("go", "install", "-ldflags", ldflags(), cmdPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Test 运行所有测试
func Test() error {
	fmt.Println("Running tests...")
	cmd := exec.Command("go", "test", "-v", "-race", "./...")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Vet 运行 go vet 静态检查
func Vet() error {
	fmt.Println("Running go vet...")
	cmd := exec.Command("go", "vet", "./...")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Clean 清理构建产物
func Clean() error {
	fmt.Println("Cleaning...")
	if err := os.RemoveAll("bin"); err != nil {
		return err
	}
	// 清理生成文件
	return filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && strings.HasSuffix(path, "_gen.go") {
			if strings.Contains(path, "example") {
				fmt.Printf("  Removing %s\n", path)
				return os.Remove(path)
			}
		}
		return nil
	})
}

// Generate 生成示例代码（运行 digen 生成 dig_gen.go）
func Generate() error {
	fmt.Println("Generating example code...")
	cmd := exec.Command("go", "run", cmdPath, "./example/app")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Default 默认任务：Build
func Default() {
	if err := Build(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
