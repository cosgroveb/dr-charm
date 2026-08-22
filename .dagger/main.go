package main

import (
	"context"
	"dagger/dr-charm/internal/dagger"
	"fmt"
	"strings"
)

type DrCharm struct{}

const goImage = "golang:1.24.1-bookworm"

// Build the dr-charm binary
func (m *DrCharm) Build(
	ctx context.Context,
	source *dagger.Directory,
	// +optional
	// +default=["linux/amd64", "darwin/amd64", "darwin/arm64", "windows/amd64"]
	platforms []string,
) ([]*dagger.File, error) {
	var builds []*dagger.File

	for _, platform := range platforms {
		binary := dag.Container().
			From(goImage).
			WithMountedDirectory("/src", source).
			WithWorkdir("/src").
			WithEnvVariable("CGO_ENABLED", "0").
			WithEnvVariable("GOOS", strings.Split(platform, "/")[0]).
			WithEnvVariable("GOARCH", strings.Split(platform, "/")[1]).
			WithExec([]string{"go", "mod", "download"}).
			WithExec([]string{"go", "build", "-o", fmt.Sprintf("dr-charm-%s%s", strings.ReplaceAll(platform, "/", "-"), getExeSuffix(platform)), "./cmd/dr-charm"}).
			File(fmt.Sprintf("/src/dr-charm-%s%s", strings.ReplaceAll(platform, "/", "-"), getExeSuffix(platform)))

		builds = append(builds, binary)
	}

	return builds, nil
}

// Test runs the Go tests
func (m *DrCharm) Test(ctx context.Context, source *dagger.Directory) error {
	_, err := dag.Container().
		From(goImage).
		WithMountedDirectory("/src", source).
		WithWorkdir("/src").
		WithExec([]string{"go", "mod", "download"}).
		WithExec([]string{"go", "test", "-race", "-shuffle=on", "./..."}).
		Stdout(ctx)

	return err
}

// Lint runs Go linters
func (m *DrCharm) Lint(ctx context.Context, source *dagger.Directory) error {
	// Run go fmt check
	fmtOut, err := dag.Container().
		From(goImage).
		WithMountedDirectory("/src", source).
		WithWorkdir("/src").
		WithExec([]string{"sh", "-c", "gofmt -l ."}).
		Stdout(ctx)

	if err != nil {
		return err
	}

	if fmtOut != "" {
		return fmt.Errorf("go fmt found issues in files:\n%s", fmtOut)
	}

	// Run go vet
	_, err = dag.Container().
		From(goImage).
		WithMountedDirectory("/src", source).
		WithWorkdir("/src").
		WithExec([]string{"go", "mod", "download"}).
		WithExec([]string{"go", "vet", "./..."}).
		Stdout(ctx)

	return err
}

// Ci runs the full CI pipeline
func (m *DrCharm) Ci(ctx context.Context, source *dagger.Directory) error {
	// Run lint
	fmt.Println("Running lint...")
	if err := m.Lint(ctx, source); err != nil {
		return fmt.Errorf("lint failed: %w", err)
	}

	// Run tests
	fmt.Println("Running tests...")
	if err := m.Test(ctx, source); err != nil {
		return fmt.Errorf("tests failed: %w", err)
	}

	// Build binaries
	fmt.Println("Building binaries...")
	builds, err := m.Build(ctx, source, []string{"linux/amd64", "darwin/amd64", "darwin/arm64", "windows/amd64"})
	if err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	fmt.Printf("Successfully built %d binaries\n", len(builds))
	return nil
}

// getExeSuffix returns .exe for Windows platforms
func getExeSuffix(platform string) string {
	if strings.Contains(platform, "windows") {
		return ".exe"
	}
	return ""
}

// Release creates a GitHub release with binaries
func (m *DrCharm) Release(
	ctx context.Context,
	source *dagger.Directory,
	version string,
	githubToken *dagger.Secret,
) error {
	// Build all platforms
	builds, err := m.Build(ctx, source, []string{"linux/amd64", "darwin/amd64", "darwin/arm64", "windows/amd64"})
	if err != nil {
		return fmt.Errorf("build failed: %w", err)
	}

	// Create release using gh CLI
	container := dag.Container().
		From("alpine:latest").
		WithExec([]string{"apk", "add", "--no-cache", "github-cli"}).
		WithMountedDirectory("/src", source).
		WithWorkdir("/src").
		WithSecretVariable("GITHUB_TOKEN", githubToken)

	// Upload binaries
	platforms := []string{"linux-amd64", "darwin-amd64", "darwin-arm64", "windows-amd64"}
	for i, binary := range builds {
		fileName := fmt.Sprintf("/tmp/dr-charm-%s%s", platforms[i], getExeSuffix(platforms[i]))
		container = container.WithFile(fileName, binary)
	}

	// Create release
	_, err = container.
		WithExec([]string{"gh", "release", "create", version,
			"/tmp/dr-charm-linux-amd64",
			"/tmp/dr-charm-darwin-amd64",
			"/tmp/dr-charm-darwin-arm64",
			"/tmp/dr-charm-windows-amd64.exe",
			"--title", fmt.Sprintf("DragonRealms Charm %s", version),
			"--notes", "DragonRealms client with Charm UI",
		}).
		Stdout(ctx)

	return err
}
