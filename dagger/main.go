// X32 Personal Mix — Dagger CI/build module.
//
// Cross-compiles the server for common OS/arch combinations and runs tests.
package main

import (
	"context"
	"fmt"
	"path"
	"runtime"
	"strings"

	"dagger/x-32-mix/internal/dagger"
)

type X32PersonalMix struct{}

type buildTarget struct {
	GOOS   string
	GOARCH string
	GOARM  string // only for GOARCH=arm (e.g. "7")
}

// Common desktop/server targets: Linux / Windows / macOS × 32/64-bit × Intel/ARM
// where Go still supports the combination.
func defaultTargets() []buildTarget {
	return []buildTarget{
		{GOOS: "linux", GOARCH: "amd64"},
		{GOOS: "linux", GOARCH: "arm64"},
		{GOOS: "linux", GOARCH: "386"},
		{GOOS: "linux", GOARCH: "arm", GOARM: "7"},
		{GOOS: "windows", GOARCH: "amd64"},
		{GOOS: "windows", GOARCH: "arm64"},
		{GOOS: "windows", GOARCH: "386"},
		{GOOS: "darwin", GOARCH: "amd64"},
		{GOOS: "darwin", GOARCH: "arm64"},
		// note: darwin/386 and windows/arm (32-bit) are not supported by modern Go
	}
}

func (t buildTarget) binaryName() string {
	name := fmt.Sprintf("x32-personal-mix_%s_%s", t.GOOS, t.GOARCH)
	if t.GOARM != "" {
		name += "v" + t.GOARM
	}
	if t.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

func goImage(goVersion string) string {
	v := strings.TrimPrefix(strings.TrimSpace(goVersion), "go")
	if v == "" {
		v = "1.26"
	}
	return "golang:" + v + "-bookworm"
}

func (m *X32PersonalMix) base(source *dagger.Directory, goVersion string) *dagger.Container {
	src := source.
		WithoutDirectory(".git").
		WithoutDirectory("dist").
		WithoutDirectory("dagger").
		WithoutDirectory(".x32-personal-mix-tls").
		WithoutDirectory(".x32-mix-tls").
		WithoutDirectory(".idea")

	return dag.Container().
		From(goImage(goVersion)).
		WithEnvVariable("CGO_ENABLED", "0").
		WithEnvVariable("GOTOOLCHAIN", "local").
		WithMountedCache("/go/pkg/mod", dag.CacheVolume("x32-personal-mix-go-mod")).
		WithMountedCache("/root/.cache/go-build", dag.CacheVolume("x32-personal-mix-go-build")).
		WithDirectory("/src", src).
		WithWorkdir("/src")
}

// Test runs go test ./...
func (m *X32PersonalMix) Test(
	ctx context.Context,
	// Project root (contains go.mod, cmd/, web/, …)
	// +defaultPath="."
	source *dagger.Directory,
	// Go Docker tag, e.g. "1.26"
	// +optional
	// +default="1.26"
	goVersion string,
) (string, error) {
	return m.base(source, goVersion).
		WithExec([]string{"go", "test", "./..."}).
		Stdout(ctx)
}

// Build cross-compiles the server for Linux, Windows and macOS
// (amd64/arm64/386/armv7 where supported). Returns a directory of binaries.
func (m *X32PersonalMix) Build(
	// Project root (contains go.mod, cmd/, web/, …)
	// +defaultPath="."
	source *dagger.Directory,
	// Go Docker tag, e.g. "1.26"
	// +optional
	// +default="1.26"
	goVersion string,
) *dagger.Directory {
	golang := m.base(source, goVersion)
	outputs := dag.Directory()

	for _, t := range defaultTargets() {
		outName := t.binaryName()
		outPath := path.Join("/out", outName)

		ctr := golang.
			WithEnvVariable("GOOS", t.GOOS).
			WithEnvVariable("GOARCH", t.GOARCH).
			WithEnvVariable("CGO_ENABLED", "0")
		if t.GOARM != "" {
			ctr = ctr.WithEnvVariable("GOARM", t.GOARM)
		}

		built := ctr.WithExec([]string{
			"go", "build",
			"-trimpath",
			"-ldflags", "-s -w",
			"-o", outPath,
			"./cmd/server",
		})

		outputs = outputs.WithFile(outName, built.File(outPath))
	}

	// Small text index of what was built (handy in CI artifacts).
	index := "X32 Personal Mix — release binaries\n"
	index += fmt.Sprintf("built with Go %s (host %s/%s)\n\n", goVersion, runtime.GOOS, runtime.GOARCH)
	for _, t := range defaultTargets() {
		index += t.binaryName() + "\n"
	}
	outputs = outputs.WithNewFile("BINARIES.txt", index)

	return outputs
}

// Check runs tests, then builds all release binaries.
func (m *X32PersonalMix) Check(
	ctx context.Context,
	// Project root
	// +defaultPath="."
	source *dagger.Directory,
	// Go Docker tag
	// +optional
	// +default="1.26"
	goVersion string,
) (*dagger.Directory, error) {
	if _, err := m.Test(ctx, source, goVersion); err != nil {
		return nil, err
	}
	return m.Build(source, goVersion), nil
}

// Targets lists the GOOS/GOARCH combinations that Build produces.
func (m *X32PersonalMix) Targets() string {
	var b strings.Builder
	for _, t := range defaultTargets() {
		line := t.GOOS + "/" + t.GOARCH
		if t.GOARM != "" {
			line += " GOARM=" + t.GOARM
		}
		line += " → " + t.binaryName()
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}
